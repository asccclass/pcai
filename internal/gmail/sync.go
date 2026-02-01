package gmail

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	signal "github.com/asccclass/pcai/internal/singal"
	"github.com/ollama/ollama/api"
)

func saveToKnowledge(summary string) {
	timestamp := time.Now().Format("2006-01-02 15:04")
	content := fmt.Sprintf("\n\n## 📝 自動郵件歸納: %s\n%s\n", timestamp, summary)

	home, _ := os.Getwd()
	path := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(content)
}

func SyncGmailToKnowledge(client *api.Client, modelName string, cfg FilterConfig) {
	emailContent, err := FetchLatestEmails(cfg)
	if err != nil {
		log.Printf("[Sync Error] 抓取郵件失敗: %v", err)
		return
	}
	// 如果沒有符合條件的新郵件，就直接結束
	if emailContent == "" || emailContent == "目前沒有符合條件的新郵件。" {
		log.Printf("[Sync] 無新郵件需要處理")
		return
	}

	ctx := context.Background()
	prompt := fmt.Sprintf(`你是一個智慧秘書。請閱讀以下郵件並完成兩個任務：
1. 摘要郵件重點。
2. 如果郵件內容涉及「緊急」、「立即處理」、「限期回覆」或「重要資安警報」，請在第一行加上 [URGENT] 標籤。
3. 請忽略行銷廣告性質的『緊急』字眼（如：限時優惠、最後一天），只針對與個人、工作、或資安相關的真正緊急事件進行標註。

郵件內容：
%s`, emailContent)

	// 2. 呼叫 Ollama 生成摘要
	req := &api.GenerateRequest{
		Model:  modelName,
		Prompt: prompt,
		Stream: new(bool), // 設為 false 關閉串流，直接取得完整回覆
	}

	var summary string
	err = client.Generate(ctx, req, func(resp api.GenerateResponse) error {
		summary = resp.Response
		return nil
	})

	if err != nil {
		log.Printf("Ollama 摘要失敗: %v", err)
		return
	}

	// 檢查是否包含緊急標籤
	if strings.Contains(summary, "[URGENT]") {
		log.Println("🚨 偵測到緊急郵件，準備發送 Signal 通知...")

		// 呼叫你的 Signal API 工具
		// 假設你的 Signal 工具接受接收者與訊息內容
		alertMsg := fmt.Sprintf("⚠️ PCAI 緊急郵件通知：\n%s", summary)
		err := signal.SendNotification("+886921609364", alertMsg) // 換成你的號碼
		if err != nil {
			log.Printf("Signal 發送失敗: %v", err)
		} else {
			log.Println("✅ Signal 通知已送出")
		}
	}

	saveToKnowledge(summary) // 寫入檔案
}
