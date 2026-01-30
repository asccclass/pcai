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
	renderer, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(100))

	// 初始化工具
	registry := tools.NewRegistry()
	registry.Register(&tools.ListFilesTool{})
	registry.Register(&tools.ShellExecTool{})
	registry.Register(&tools.KnowledgeSearchTool{})
	registry.Register(&tools.FetchURLTool{})
	toolDefs := registry.GetDefinitions()

	// 載入 Session 與 RAG 增強
	sess := history.LoadLatestSession()
	if len(sess.Messages) == 0 {
		ragPrompt := history.GetRAGEnhancedPrompt()
		sess.Messages = append(sess.Messages, ollama.Message{Role: "system", Content: systemPrompt + ragPrompt})
	}

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("🚀 PCAI Agent 已啟動 (ARM64 Optimized)"))

	for {
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
				fmt.Println(aiStyle.Render("AI:"))
				out, _ := renderer.Render(fullResponse.String())
				fmt.Print(out)
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
				fmt.Println(toolStyle.Render(fmt.Sprintf("🛠️  執行工具 [%s] 參數: %s", tc.Function.Name, string(argsJSON))))

				result, toolErr := registry.CallTool(tc.Function.Name, string(argsJSON))
				if toolErr != nil {
					result = "Error: " + toolErr.Error()
				}

				sess.Messages = append(sess.Messages, ollama.Message{
					Role:    "tool",
					Content: result,
				})
			}
			// 執行完工具，會回到循環頂端再次顯示「思考中...」並請 AI 總結工具結果
		}

		// 持久化 Session
		history.SaveSession(sess)
		// 檢查是否需要歸納 (RAG)
		history.CheckAndSummarize(modelName, systemPrompt)
	}
}
