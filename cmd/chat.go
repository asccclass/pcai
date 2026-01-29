package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/asccclass/pcai/llms/ollama"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// 定義包等級變數，讓所有函數都能存取
var (
	modelName    string
	systemPrompt string

	// 定義樣式
	aiStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	promptStr = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(">>> ")

	// 根指令
	rootCmd = &cobra.Command{
		Use:   "pcai",
		Short: "Personal CLI AI Tool",
	}

	// 聊天指令
	chatCmd = &cobra.Command{
		Use:   "chat",
		Short: "開啟互動式對話模式",
		Run:   runChat, // 指向下方定義的函數
	}
)

// init 函數會在包被載入時自動執行，適合用來設定指令關係
func init() {
	chatCmd.Flags().StringVarP(&modelName, "model", "m", "llama3.3", "指定使用的模型")
	chatCmd.Flags().StringVarP(&systemPrompt, "system", "s", "你是一個專業的助手", "設定 System Prompt")
	rootCmd.AddCommand(chatCmd)
}

// 將邏輯封裝在函數中，避免 Top-level 語法錯誤
func runChat(cmd *cobra.Command, args []string) {
	scanner := bufio.NewScanner(os.Stdin)
	var currentContext []int

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)

	fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("🚀 AI 聊天室已就緒！輸入 'exit' 結束。"))

	for {
		fmt.Print(promptStr)
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		if input == "exit" || input == "quit" {
			fmt.Println("再見！")
			break
		}
		if input == "" {
			continue
		}

		fmt.Print(aiStyle.Render("AI: "))

		var fullResponse strings.Builder
		lineCount := 0

		// 串流顯示
		newCtx, err := ollama.ChatStream(modelName, systemPrompt, input, currentContext, func(content string) {
			fmt.Print(content)
			fullResponse.WriteString(content)
			// 簡單計算換行數
			lineCount += strings.Count(content, "\n")
		})

		if err != nil {
			fmt.Printf("\n❌ 錯誤: %v\n", err)
			continue
		}
		currentContext = newCtx

		// --- ANSI 覆蓋邏輯 ---
		// 1. 回到行首
		fmt.Print("\r")
		// 2. 根據輸出的行數向上移動並清除
		for i := 0; i < lineCount; i++ {
			fmt.Print("\033[F\033[K")
		}
		fmt.Print("\033[K") // 清除 "AI: " 這一行

		// 3. 輸出渲染後的 Markdown
		rendered, _ := renderer.Render(fullResponse.String())
		fmt.Println(aiStyle.Render("AI: "))
		fmt.Print(rendered)
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(strings.Repeat("─", 50)))
	}
}

// Execute 提供給 main.go 呼叫
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
