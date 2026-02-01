package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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

	aiStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true)
	notifyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // 亮黃色
	promptStr   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(">>> ")
	currentOpts = ollama.Options{Temperature: 0.7, TopP: 0.9}
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "開啟具備 AI Agent 能力的對話",
	Run:   runChat,
}

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
		glamour.WithWordWrap(100),
	)

	// 初始化管理器
	bgMgr := tools.NewBackgroundManager()
	GlobalBgMgr = bgMgr // 將實例交給全域指標，讓 health 指令讀得到
	// 初始化工具
	registry := tools.InitRegistry(bgMgr)
	toolDefs := registry.GetDefinitions()

	// 載入 Session 與 RAG 增強
	sess := history.LoadLatestSession()

	// [FIX] 初始化全域 CurrentSession，否則 CheckAndSummarize 會抓不到
	history.CurrentSession = sess
	// [FIX] 啟動時檢查是否需要歸納 (處理「上次關閉後過很久才重開」的情況)
	history.CheckAndSummarize(modelName, systemPrompt)

	// 若歸納後被清空 (Start New Session)，這裡 sess 內容已變，需重新對齊
	// 但因為 CurrentSession 是指標，上面的 CheckAndSummarize 內修改的就是同一個物件
	// 只是若 Messages 被清空，這裡需要確保補回 System Prompt
	if len(sess.Messages) == 0 {
		ragPrompt := history.GetRAGEnhancedPrompt()
		sess.Messages = append(sess.Messages, ollama.Message{Role: "system", Content: systemPrompt + ragPrompt})
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("🚀 PCAI Agent 已啟動 ( I'm the assistant your terminal demanded, not the one your sleep schedule requested.)"))

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

		sess.Messages = append(sess.Messages, ollama.Message{Role: "user", Content: input})

		// Tool-Calling 狀態機循環
		for {
			var fullResponse strings.Builder
			thinkingMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("AI 正在思考中...")
			fmt.Print(thinkingMsg)

			aiMsg, err := ollama.ChatStream(modelName, sess.Messages, toolDefs, currentOpts, func(content string) {
				fullResponse.WriteString(content)
			})

			// 清除「思考中...」提示
			fmt.Print("\r\033[K")

			if err != nil {
				fmt.Printf("❌ 錯誤: %v\n", err)
				break
			}

			// 顯示 AI 回覆內容 (一次性渲染)
			if aiMsg.Content != "" {
				// 印出「AI: 」標籤 (不換行)
				fmt.Print(aiStyle.Render("AI: "))
				out, _ := renderer.Render(fullResponse.String())
				// 去除 Glamour 和 AI 內容前後的空白與換行
				cleanOut := strings.TrimSpace(out)
				fmt.Print(cleanOut)
				// 結尾手動補兩個換行，保持與下個提示符的距離
				fmt.Print("\n\n")
				clipboard.WriteAll(fullResponse.String())
			}

			sess.Messages = append(sess.Messages, aiMsg)

			// 檢查是否呼叫工具
			if len(aiMsg.ToolCalls) == 0 {
				break // 最終回答完畢，跳出循環
			}

			// 執行工具
			for _, tc := range aiMsg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				// 改用灰色且稍微縮進的樣式
				toolHint := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
					fmt.Sprintf("  ↳ 🛠️ Executing %s(%s)...", tc.Function.Name, string(argsJSON)),
				)
				fmt.Println(toolHint)

				result, toolErr := registry.CallTool(tc.Function.Name, string(argsJSON))
				// --- 強化背景執行的反饋 ---
				var toolFeedback string
				if toolErr != nil {
					toolFeedback = fmt.Sprintf("【執行失敗】：%v", toolErr)
				} else {
					// 如果結果包含 "背景啟動"，則給予強大的確認標記
					if strings.Contains(result, "背景啟動") {
						aiMsg.ToolCalls = nil // 💡 強制清除，防止 AI 腦袋卡住
						// toolFeedback = fmt.Sprintf("【SYSTEM】: %s。任務已交給作業系統，請立即停止呼叫工具，並用一句話回報使用者任務已啟動。", result)
					} else {
						toolFeedback = fmt.Sprintf("【SYSTEM】: %s", result)
					}
				}

				sess.Messages = append(sess.Messages, ollama.Message{
					Role:    "tool",
					Content: toolFeedback,
				})
			}
			// 執行完工具，會回到循環頂端再次顯示「思考中...」並請 AI 總結工具結果
		}

		// 自動儲存與 RAG 歸納檢查
		history.SaveSession(sess)
		history.CheckAndSummarize(modelName, systemPrompt)
	}
}
