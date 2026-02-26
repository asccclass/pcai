package cmd

import (
	"fmt"

	"github.com/asccclass/pcai/llms/copilot"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(copilotLoginCmd)
}

var copilotLoginCmd = &cobra.Command{
	Use:   "copilot-login",
	Short: "透過 GitHub OAuth Device Flow 登入 GitHub Copilot",
	Long:  "使用與 OpenClaw 相同的 Device Flow 認證方式登入 GitHub Copilot。登入成功後，Token 會儲存在 copilot_token.json 中。",
	Run: func(cmd *cobra.Command, args []string) {
		token, err := copilot.Login()
		if err != nil {
			fmt.Printf("❌ 登入失敗: %v\n", err)
			return
		}
		_ = token
	},
}
