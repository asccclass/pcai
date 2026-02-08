package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd 代表基礎指令，當不帶任何子指令執行時觸發
var rootCmd = &cobra.Command{
	Use:   "pcai",
	Short: "Personalized Contextual AI - 你的個人 AI 助手",
	Long:  `一個支援多輪對話、工具呼叫、RAG 長期記憶的強大 CLI 工具。`,
}

// Execute 將所有子指令註冊到根指令並執行
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
