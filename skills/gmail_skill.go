package skills

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/gmail"
	signal "github.com/asccclass/pcai/internal/singal"
	"github.com/ollama/ollama/api"
)

// GmailSkill 負責協調 Gmail 讀取、AI 摘要與 Signal 通知
type GmailSkill struct {
	ollamaClient *api.Client
	modelName    string
}

func NewGmailSkill(client *api.Client, modelName string) *GmailSkill {
	return &GmailSkill{
		ollamaClient: client,
		modelName:    modelName,
	}
}

// Execute 執行讀取與通知流程
func (s *GmailSkill) Execute(cfg gmail.FilterConfig) {
	fmt.Println("[GmailSkill] 開始執行郵件同步任務...")

	// 1. 呼叫 Worker 取得郵件內容
	emailContent, err := gmail.FetchLatestEmails(cfg)
	if err != nil {
		log.Printf("[GmailSkill Error] 抓取郵件失敗: %v", err)
		return
	}

	// 如果沒有符合條件的新郵件，就直接結束
	if emailContent == "" || emailContent == "目前沒有符合條件的新郵件。" {
		log.Printf("[GmailSkill] 無新郵件需要處理")
		return
	}

	// 2. 構建 Prompt (Adapter 層職責)
	ctx := context.Background()
	prompt := fmt.Sprintf(`你是一個智慧秘書。請閱讀以下郵件並完成兩個任務：
1. 摘要郵件重點。
2. 如果郵件內容涉及「緊急」、「立即處理」、「限期回覆」或「重要資安警報」，請在第一行加上 [URGENT] 標籤。
3. 請忽略行銷廣告性質的『緊急』字眼（如：限時優惠、最後一天），只針對與個人、工作、或資安相關的真正緊急事件進行標註。

郵件內容：
%s`, emailContent)

	// 3. 呼叫 Ollama 生成摘要
	req := &api.GenerateRequest{
		Model:  s.modelName,
		Prompt: prompt,
		Stream: new(bool), // 設為 false 關閉串流，直接取得完整回覆
	}

	var summary string
	err = s.ollamaClient.Generate(ctx, req, func(resp api.GenerateResponse) error {
		summary = resp.Response
		return nil
	})

	if err != nil {
		log.Printf("[GmailSkill Error] Ollama 摘要失敗: %v", err)
		return
	}

	// 4. 判斷是否緊急並發送 Signal (業務邏輯)
	if strings.Contains(summary, "[URGENT]") {
		log.Println("🚨 [GmailSkill] 偵測到緊急郵件，準備發送 Signal 通知...")

		alertMsg := fmt.Sprintf("⚠️ PCAI 緊急郵件通知：\n%s", summary)
		// 注意：這裡假設 Signal 接收者號碼是寫死的，或者是注入的。
		// 在重購時，保留原有的 Hardcoded 號碼，或建議之後改成設定檔讀取
		err := signal.SendNotification("+886921609364", alertMsg)
		if err != nil {
			log.Printf("[GmailSkill Error] Signal 發送失敗: %v", err)
		} else {
			log.Println("✅ [GmailSkill] Signal 通知已送出")
		}
	}

	// 5. 寫入長期記憶
	s.saveToKnowledge(summary)
}

func (s *GmailSkill) saveToKnowledge(summary string) {
	home, _ := os.Getwd()
	path := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")

	// 1. 檢查是否重複 (讀取最後 4096 bytes)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("[GmailSkill Error] 無法讀取知識庫: %v", err)
		return
	}
	defer f.Close()

	stat, _ := f.Stat()
	fileSize := stat.Size()

	// 只讀取最後 5KB 進行比對
	readSize := int64(5120)
	if fileSize < readSize {
		readSize = fileSize
	}

	if readSize > 0 {
		buf := make([]byte, readSize)
		f.Seek(-readSize, 2)
		f.Read(buf)

		existingContent := string(buf)
		// 簡單比對: 如果新摘要的內容 (去除 timestamp 前綴後的核心內容) 已經存在，就跳過
		// 這裡假設 summary 內容足夠獨特。
		// 為了避免因時間戳記不同而誤判，我們可以比對 summary 的後段或核心句子。
		// 這裡做一個簡單的 substring check。
		if strings.Contains(existingContent, summary) {
			log.Println("⚠️ [GmailSkill] 摘要已存在，跳過寫入 (Deduplicated)")
			return
		}
	}

	// 2. 寫入
	timestamp := time.Now().Format("2006-01-02 15:04")
	content := fmt.Sprintf("\n\n## 📝 自動郵件歸納: %s\n%s\n", timestamp, summary)

	// 移動到檔尾準備寫入
	f.Seek(0, 2)
	f.WriteString(content)
	log.Println("✅ [GmailSkill] 摘要已存入 Knowledge")
}
