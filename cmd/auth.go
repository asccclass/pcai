package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/asccclass/pcai/internal/googleauth"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "管理外部服務授權",
	Long:  `用於重新認證或取得外部服務（如 Gmail, Google Calendar）的 OAuth Token。`,
}

var googleAuthCmd = &cobra.Command{
	Use:   "google",
	Short: "重新認證 Google 服務 (Gmail + Calendar)",
	Long:  `此指令會啟動 OAuth 流程，請求 Gmail 與 Calendar 權限。請刪除舊的 token.json 後執行此指令。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🚀 正在啟動 Google 服務 (Gmail + Calendar) 認證流程...")
		fmt.Println("⚠️  注意：請確保您已刪除 'token.json'，否則可能會直接使用舊憑證。")

		// 讀取 credentials.json
		b, err := os.ReadFile("credentials.json")
		if err != nil {
			log.Fatalf("無法讀取 credentials.json: %v", err)
		}

		// 設定需要的 Scope: Gmail 修改權限 + Calendar 唯讀權限
		config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope, calendar.CalendarReadonlyScope)
		if err != nil {
			log.Fatalf("解析憑證失敗: %v", err)
		}

		// 觸發 Auth 流程
		client := googleauth.GetClient(config)
		if client != nil {
			fmt.Println("✅ 認證流程完成，token.json 應已更新 (包含 Gmail 與 Calendar 權限)。")
		}
	},
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(googleAuthCmd)
}
