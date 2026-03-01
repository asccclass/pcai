package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/config"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/asccclass/pcai/tools"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	modelName    string
	systemPrompt string
	cfg          *config.Config

	aiStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	// toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true)
	notifyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // 亮黃色
	promptStr   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(">>> ")
	currentOpts = ollama.Options{Temperature: 0.7, TopP: 0.9}
)

func init() {
	cfg = config.LoadConfig()
	chatCmd.Flags().StringVarP(&modelName, "model", "m", cfg.Model, "指定使用的模型")
	chatCmd.Flags().StringVarP(&systemPrompt, "system", "s", cfg.SystemPrompt, "設定 System Prompt")
	rootCmd.AddCommand(chatCmd)
}

// 輔助函式：用來處理 Glamour 需要的 uint 指標
func uintPtr(i uint) *uint { return &i }

func runChat(cmd *cobra.Command, args []string) {
	scanner := bufio.NewScanner(os.Stdin)
	// --- 緊湊型 Glamour 樣式設定 ---
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(0), // 自動適配終端寬度，不強制切斷
	)

	// 初始化背景執行管理器(Background Manager)
	bgMgr := tools.NewBackgroundManager()
	GlobalBgMgr = bgMgr // 將實例交給全域指標，讓 health 指令讀得到

	// 初始化 System Logger (在工具註冊前初始化，以便傳入 Adapter)
	logger, err := agent.NewSystemLogger("botmemory")
	if err != nil {
		fmt.Printf("⚠️ [System] Failed to initialize system logger: %v\n", err)
	} else {
		defer logger.Close()
	}

	// 初始化工具
	registry, cleanup := tools.InitRegistry(bgMgr, cfg, logger, func() {
		// 當非同步任務(如Telegram)完成且有輸出時，補印提示符
		fmt.Print("\n" + promptStr)
	})
	defer cleanup() // 程式結束時執行清理 (停止 Telegram)

	// 載入 Session 與 RAG 增強
	sess := history.LoadLatestSession()

	// [FIX] 啟動時檢查是否需要歸納 (處理「上次關閉後過很久才重開」的情況)
	history.CheckAndSummarize(sess, modelName, systemPrompt)

	// 若歸納後被清空 (Start New Session)，這裡 sess 內容已變，需重新對齊
	// 但因為 CurrentSession 是指標，上面的 CheckAndSummarize 內修改的就是同一個物件
	// 只是若 Messages 被清空，這裡需要確保補回 System Prompt
	if len(sess.Messages) == 0 {
		ragPrompt := history.GetRAGEnhancedPrompt()
		sess.Messages = append(sess.Messages, ollama.Message{Role: "system", Content: systemPrompt + ragPrompt})
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("🚀 PCAI Agent 已啟動 ( I'm the assistant your terminal demanded, not the one your sleep schedule requested.)"))

	// -------------------------------------------------------------
	// 5. 初始化 Agent
	// -------------------------------------------------------------
	myAgent := agent.NewAgent(modelName, systemPrompt, sess, registry, logger)

	// [BOOT] 系統啟動時，優先詢問 LLM 的姓名並寫入全域變數
	fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("AI 正在設定專屬稱呼..."))
	bootMessages := []ollama.Message{
		{Role: "system", Content: "你是一個專業的助理。"},
		{Role: "user", Content: "請為你自己取一個簡短的名字（只要回覆名字即可，絕對不要回答其他對話或標點符號）。"},
	}
	// 繞過 agent.Chat 直接呼叫 Provider 確保這段對話不會被記錄進歷史
	if nameMsg, err := myAgent.Provider(modelName, bootMessages, nil, currentOpts, nil); err == nil && nameMsg.Content != "" {
		config.GlobalName = strings.TrimSpace(nameMsg.Content)
		// 動態更新終端 Prompt 顯示名字
		promptStr = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf(">>> [%s]: ", config.GlobalName))
		fmt.Printf("\r\033[K%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(fmt.Sprintf("✅ 系統已為 AI 命名為：%s", config.GlobalName)))
	} else {
		// 失敗或逾時的 Fallback
		fmt.Print("\r\033[K")
		config.GlobalName = "Assistant"
	}

	// [MEMORY-FIRST] 設定記憶預搜尋回調
	if cfg.MemoryEnabled && (tools.GlobalDB != nil || tools.GlobalMemoryToolKit != nil) {
		myAgent.OnMemorySearch = agent.BuildMemorySearchFunc(tools.GlobalDB, tools.GlobalMemoryToolKit)
	}

	// [TASK RECOVERY] 設定未完成任務檢查回調
	myAgent.OnCheckPendingPlan = tools.CheckPendingPlan
	myAgent.OnAcquireTaskLock = tools.AcquireTaskLock
	myAgent.OnReleaseTaskLock = tools.ReleaseTaskLock
	myAgent.OnIsTaskLocked = tools.IsTaskLocked

	// 設定 UI 回調 (Bridging Agent Events -> CLI Glamour UI)
	myAgent.OnGenerateStart = func() {
		// 恢復 "AI 正在思考中..." 的暫時性提示
		fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("AI 正在思考中..."))
	}

	myAgent.OnModelMessageComplete = func(content string) {
		// 清除行 (如果是思考中...)
		fmt.Print("\r\033[K")

		if content != "" {
			// 檢查內容是否包含 <thought> 標籤或是純文字
			// 為了符合使用者需求 ">> Agent 思考: ..."
			// 我們假設 Agent 的 Response 如果不包含 Tool Call，就是思考或回答。
			// 但這裡接收到的是最終回答。
			// 如果要印出 "Agent 思考"，通常是在 Tool Call 之前。
			// 讓我們調整策略：在 onStream 中捕捉思考過程?
			// 或是在 Agent 內部區分 "Thought" 和 "Content"。
			// 目前架構 Agent.Chat 會回傳 finalResponse。

			// 簡單實作：直接印出回答作為結果，或者視為思考的一部分 (如果後面還有 Tool Call)
			// 但 OnModelMessageComplete 是在 Tool Loop 裡面的每一輪都會觸發嗎？
			// 看 agent.go:99 -> 是的，每次 Provider 回傳都會觸發。

			// 判斷是否為「引導 Tool Call 的思考」還是「最終回答」比較困難，
			// 但通常如果是 CoT 模型，它會先輸出思考。

			// 為了格式統一，我們這裡印出 ">> Agent 思考: " 加上內容?
			// 但使用者範例是:
			// >> Agent 思考: 識別出這是一個「排程」需求。
			// >> 工具決策: ...

			// 這裡的 content 就是 Agent 的輸出。
			// 如果 Agent 決定呼叫工具，它的 Content 通常會是空的 (OpenAI) 或包含思考 (Ollama/CoT)。
			// 我們即使是 Final Answer 也可以用類似格式。

			// 為了美觀，我們先印出 Header
			header := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render(">> Agent 思考: ")
			fmt.Println(header)

			// 內容渲染
			out, _ := renderer.Render(content)
			fmt.Println(strings.TrimSpace(out))
			clipboard.WriteAll(content)
		}
	}

	myAgent.OnToolCall = func(name, args string) {
		// 工具決策輸出
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true).Render(">> 工具決策: ")
		fmt.Printf("%s呼叫 %s\n", header, name)

		// 參數輸出 (縮排)
		paramStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		fmt.Printf("       %s\n", paramStyle.Render(fmt.Sprintf("參數: %s", args)))
	}

	myAgent.OnToolResult = func(result string) {
		// 結果輸出
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render(">> 結果: ")

		// 處理結果字串，讓它好看一點 (例如去除 【SYSTEM】 前綴)
		cleanResult := strings.Replace(result, "【SYSTEM】: ", "", 1)
		cleanResult = strings.Replace(cleanResult, "【SYSTEM】", "", 1)

		// 加上 ✅ 如果成功 (或是讓 Agent 回傳時就帶有)
		// 這裡簡單判斷：如果沒有 "失敗" 或 "Error" 字眼
		icon := "✅"
		lowerResult := strings.ToLower(cleanResult)
		if strings.Contains(lowerResult, "error") || strings.Contains(lowerResult, "failed") || strings.Contains(cleanResult, "失敗") {
			icon = "❌"
		}

		fmt.Printf("%s %s %s\n", header, icon, strings.TrimSpace(cleanResult))
	}

	for {
		// --- 背景任務完成通知推播 ---
		select {
		case msg := <-bgMgr.NotifyChan:
			fmt.Println("\n" + notifyStyle.Render(msg))
		default:
			// 無通知則跳過
		}

		fmt.Print(promptStr)
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		// 顯示使用者輸入 (模擬 Log 格式，雖然使用者已經打在螢幕上了，但為了符合需求格式，我們再印一次？)
		// 使用者需求: ">>> 使用者輸入: 「...」"
		// 由於 scanner 讀取時使用者已經輸入了 ">>> [input]" (promptStr 是 ">>> ")
		// 我們可以不重複印，或者為了嚴格符合格式要求再印一次。
		// 考慮到體驗，重複印會很怪。使用者輸入的那行就是 ">>> [input]"。
		// 我們只要確保 promptStr 是 ">>> " 即可。目前 code line 29 就是。
		// 但使用者想要 ">>> 使用者輸入: "，我們可以修改 Prompt?
		// 或是保留 ">>> "，但在 Log 裡補上標籤?
		// ">>> 使用者輸入: 「...」" 看起來像是回顧 Log。
		// 如果是即時互動， Prompt 就是 Prompt。
		// 讓我們修改 Prompt 顯示方式，或者在 Agent 處理前印出一個確認行。

		if input != "" && input != "exit" && input != "quit" {
			fmt.Printf("\033[1A\033[K") // 清除上一行 (使用者的原始輸入) - 選擇性，看終端支援度
			// 重新格式化輸出
			userHeader := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true).Render(">>> 使用者輸入: ")
			fmt.Printf("%s「%s」\n", userHeader, input)
		}

		if input == "exit" || input == "quit" {
			break
		}
		if input == "" {
			continue
		}

		// 這裡可以加入處理 /file, /set 等自定義指令的邏輯

		// 交給 Agent 處理
		_, err := myAgent.Chat(input, nil) // CLI 暫不使用 Realtime stream raw text，而是依賴 Callbacks 渲染 Markdown
		if err != nil {
			fmt.Printf("❌ 錯誤: %v\n", err)
		}

		// 自動儲存與 RAG 歸納檢查 (Session 由 Agent 內部維護，直接儲存即可)
		history.SaveSession(sess)
		history.CheckAndSummarize(sess, modelName, systemPrompt)
	}
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "開啟具備 AI Agent 能力的對話",
	Run:   runChat,
}
