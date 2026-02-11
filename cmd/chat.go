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
	// 初始化工具
	// 初始化工具
	registry, cleanup := tools.InitRegistry(bgMgr, cfg, func() {
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
	// 4. 初始化 Agent
	// -------------------------------------------------------------
	myAgent := agent.NewAgent(modelName, systemPrompt, sess, registry)

	// 設定 UI 回調 (Bridging Agent Events -> CLI Glamour UI)
	myAgent.OnGenerateStart = func() {
		thinkingMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("AI 正在思考中...")
		fmt.Print(thinkingMsg)
	}

	myAgent.OnModelMessageComplete = func(content string) {
		// 清除「思考中...」提示
		fmt.Print("\r\033[K")

		if content != "" {
			// 印出「AI: 」標籤 (不換行)
			fmt.Print(aiStyle.Render("AI: "))
			out, _ := renderer.Render(content)
			// 去除 Glamour 和 AI 內容前後的空白與換行
			cleanOut := strings.TrimSpace(out)
			fmt.Print(cleanOut)
			// 結尾手動補兩個換行，保持與下個提示符的距離
			fmt.Print("\n\n")
			clipboard.WriteAll(content)
		}
	}

	myAgent.OnToolCall = func(name, args string) {
		// 這裡確保清除思考提示，因為 Agent 流程中是: GenerateStart -> ChatStream -> MessageComplete(Clear) -> ToolCall
		// 但如果是連續 ToolCall，可能需要再次清除
		fmt.Print("\r\033[K")

		toolHint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
			fmt.Sprintf("  ↳ 🛠️ Executing %s(%s)...", name, args),
		)
		fmt.Println(toolHint)
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
