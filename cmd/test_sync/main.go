package main

import (
	"log"

	"github.com/asccclass/pcai/internal/gmail" // 請確認你的 package 路徑
	"github.com/ollama/ollama/api"
)

func main() {
	log.Println("🚀 開始手動測試 Gmail 知識同步任務...")

	// 1. 初始化 Ollama Client
	// api.ClientFromEnvironment 會自動讀取 OLLAMA_HOST 環境變數
	client, err := api.ClientFromEnvironment()
	if err != nil {
		log.Fatalf("無法初始化 Ollama 客戶端: %v", err)
	}

	// 2. 設定測試用的過濾規則
	// 建議先放寬條件（例如只放 gmail.com）確保能抓到東西
	cfg := gmail.FilterConfig{
		AllowedSenders: []string{"@gmail.com"},
		KeyPhrases:     []string{}, // 留空代表不限主旨關鍵字
		MaxResults:     3,
	}

	// 3. 手動觸發同步任務
	// 注意：第一次執行時，終端機可能會出現 OAuth 授權網址，請依照指示操作
	log.Println("正在讀取郵件並進行摘要 (這可能需要一點時間)...")

	// 假設你本地使用的模型是 llama3
	gmail.SyncGmailToKnowledge(client, "llama3.3", cfg)

	log.Println("✅ 測試完成！請檢查你的 knowledge.md 檔案是否有新增內容。")
}
