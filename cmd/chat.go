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

	// 樣式設定
	aiStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true)
	promptStr   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(">>> ")
	currentOpts = ollama.Options{Temperature: 0.7, TopP: 0.9}
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "開啟具備工具能力與 RAG 的互動對話",
	Run:   runChat,
}

func init() {
	cfg = config.LoadConfig()
	chatCmd.Flags().StringVarP(&modelName, "model", "m", cfg.Model, "指定模型")
	chatCmd.Flags().StringVarP(&systemPrompt, "system", "s", cfg.SystemPrompt, "系統提示詞")
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) {
	scanner := bufio.NewScanner(os.Stdin)
	renderer, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(100))

	// 1. 初始化工具註冊中心
	registry := tools.NewRegistry()
	registry.Register(&tools.ListFilesTool{})
	registry.Register(&tools.ShellExecTool{})       // 註冊執行工具
	registry.Register(&tools.KnowledgeSearchTool{}) // 註冊搜尋工具
	registry.Register(&tools.FetchURLTool{})        // 註冊爬蟲工具
	toolDefs := registry.GetDefinitions()

	// 2. 載入 Session (RAG 自動載入)
	sess := history.LoadLatestSession()
	if len(sess.Messages) == 0 {
		sess.Messages = append(sess.Messages, ollama.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("🚀 AI Agent 已就緒！(支援工具呼叫與自動歸納)"))

	for {
		fmt.Print(promptStr)
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		// 處理內建指令 (/e, /file, /set, exit 等)
		if handleCommands(input, sess) {
			continue
		}
		if input == "exit" {
			break
		}

		// 加入使用者訊息
		sess.Messages = append(sess.Messages, ollama.Message{Role: "user", Content: input})

		// 3. 進入 Tool-Calling 循環
		for {
			var fullResponse strings.Builder
			// fmt.Print(aiStyle.Render("AI: "))
			// 1. 顯示「思考中」提示
			fmt.Print(lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("AI 正在思考中..."))

			// 呼叫更新後的 ChatStream
			aiMsg, err := ollama.ChatStream(modelName, sess.Messages, toolDefs, currentOpts, func(content string) {
				// fmt.Print(content)
				fullResponse.WriteString(content)
			})

			// 清除「思考中」提示（使用 ANSI 序列回退）
			fmt.Print("\r\033[K")

			if err != nil {
				fmt.Printf("\n❌ 錯誤: %v\n", err)
				break
			}
			// 3. 處理 AI 的回覆內容
			if aiMsg.Content != "" {
				// 一次性渲染並顯示內容
				fmt.Println(aiStyle.Render("AI:"))
				out, _ := renderer.Render(fullResponse.String())
				fmt.Print(out)

				// 自動存入剪貼簿
				clipboard.WriteAll(fullResponse.String())
			}

			// 將 AI 的回應存入 Session
			sess.Messages = append(sess.Messages, aiMsg)

			// 檢查是否需要執行工具
			if len(aiMsg.ToolCalls) == 0 {
				// 沒有工具呼叫，顯示美化後的內容並結束本輪
				//renderFinal(fullResponse.String(), renderer)
				// clipboard.WriteAll(fullResponse.String())
				break
			}

			// 執行工具呼叫
			for _, tc := range aiMsg.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				fmt.Println(toolStyle.Render(fmt.Sprintf("🛠️ 執行工具 [%s] 參數: %s", tc.Function.Name, string(argsJSON))))

				// 執行並取得結果
				argsBytes, _ := json.Marshal(tc.Function.Arguments)
				result, toolErr := registry.CallTool(tc.Function.Name, string(argsBytes))
				if toolErr != nil {
					result = "工具執行錯誤: " + toolErr.Error()
				}

				// 將工具結果餵回 Session，角色定為 "tool"
				sess.Messages = append(sess.Messages, ollama.Message{
					Role:    "tool",
					Content: result,
				})
			}
			// 繼續循環，讓 AI 看到工具結果後重新生成回覆
		}

		// 每次對話完自動存檔
		history.SaveSession(sess)
	}
}

// 輔助：渲染最終 Markdown 並清理螢幕
func renderFinal(content string, r *glamour.TermRenderer) {
	fmt.Println("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", 50)))
	out, _ := r.Render(content)
	fmt.Print(out)
}

// 輔助：處理特殊指令
func handleCommands(input string, sess *history.Session) bool {
	// 這裡實作之前的 /e, /file, /set 等邏輯
	// ... (省略部分重複代碼，邏輯與之前一致)
	return false
}
