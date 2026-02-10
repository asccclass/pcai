package gmail

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/asccclass/pcai/internal/googleauth"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// FilterConfig 定義郵件過濾規則
type FilterConfig struct {
	AllowedSenders []string // 允許的寄件者關鍵字 (例如: "google.com")
	KeyPhrases     []string // 主旨必須包含的關鍵字
	MaxResults     int64
}

// MarkAsRead 將指定的郵件 ID 清單標記為已讀
func MarkAsRead(srv *gmail.Service, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}

	user := "me"
	req := &gmail.BatchModifyMessagesRequest{
		Ids:            messageIDs,
		RemoveLabelIds: []string{"UNREAD"}, // 移除「未讀」標籤
	}

	err := srv.Users.Messages.BatchModify(user, req).Do()
	if err != nil {
		return fmt.Errorf("無法標記郵件為已讀: %v", err)
	}

	log.Printf("✅ 已成功將 %d 封郵件標記為已讀", len(messageIDs))
	return nil
}

// FetchLatestEmails 是對外的整合進入點
func FetchLatestEmails(cfg FilterConfig) (string, error) {
	ctx := context.Background()

	// 1. 讀取憑證檔案
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return "", fmt.Errorf("無法讀取 credentials.json: %v", err)
	}

	// 2. 配置 OAuth2 .GmailModifyScope
	config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope)
	if err != nil {
		return "", fmt.Errorf("解析憑證失敗: %v", err)
	}

	// 3. 取得授權的 HTTP Client
	client := googleauth.GetClient(config)

	// 4. 初始化 Gmail 服務
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("無法啟動 Gmail 服務: %v", err)
	}

	// 5. 執行過濾讀取
	return fetchAndFilter(srv, cfg)
}

// fetchAndFilter 執行實際的 API 呼叫與邏輯過濾
func fetchAndFilter(srv *gmail.Service, cfg FilterConfig) (string, error) {
	user := "me"

	// 建立基礎搜尋條件：未讀
	query := "is:unread"
	if len(cfg.AllowedSenders) > 0 {
		senderPart := fmt.Sprintf("(from:%s)", strings.Join(cfg.AllowedSenders, " OR from:"))
		query += " " + senderPart
	}

	// 呼叫 API 取得列表
	r, err := srv.Users.Messages.List(user).Q(query).MaxResults(cfg.MaxResults).Do()
	if err != nil {
		return "", fmt.Errorf("API 請求失敗: %v", err)
	}

	if len(r.Messages) == 0 {
		return "目前沒有符合條件的新郵件。", nil
	}

	var result strings.Builder
	var matchedIDs []string // 收集符合條件的郵件 ID

	for _, m := range r.Messages {
		// 取得郵件詳細內容 (Metadata 模式較省流量)
		msg, err := srv.Users.Messages.Get(user, m.Id).Format("metadata").Do()
		if err != nil {
			continue
		}

		subject := ""
		from := ""
		for _, h := range msg.Payload.Headers {
			if h.Name == "Subject" {
				subject = h.Value
			}
			if h.Name == "From" {
				from = h.Value
			}
		}

		// 主旨關鍵字過濾 (Client-side)
		match := len(cfg.KeyPhrases) == 0
		for _, phrase := range cfg.KeyPhrases {
			if strings.Contains(strings.ToLower(subject), strings.ToLower(phrase)) {
				match = true
				break
			}
		}

		if match {
			matchedIDs = append(matchedIDs, m.Id) // 加入待標記清單

			result.WriteString(fmt.Sprintf("【寄件者】: %s\n", from))
			result.WriteString(fmt.Sprintf("【主旨】: %s\n", subject))
			result.WriteString(fmt.Sprintf("【內容摘要】: %s\n", msg.Snippet))
			result.WriteString("------------------------------------------\n")
		}
	}

	// 標記已處理的郵件為已讀
	if len(matchedIDs) > 0 {
		if err := MarkAsRead(srv, matchedIDs); err != nil {
			log.Printf("⚠️ 標記已讀失敗: %v", err)
		}
	}

	return result.String(), nil
}

// --- OAuth2 輔助函式 ---
// 已重構至 internal/googleauth

// SearchEmails 根據關鍵字搜尋郵件
func SearchEmails(query string, maxResults int64) (string, error) {
	ctx := context.Background()

	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return "", fmt.Errorf("無法讀取 credentials.json: %v", err)
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailModifyScope)
	if err != nil {
		return "", fmt.Errorf("解析憑證失敗: %v", err)
	}

	client := googleauth.GetClient(config)

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("無法啟動 Gmail 服務: %v", err)
	}

	user := "me"
	// 如果 query 為空，預設為未讀
	if query == "" {
		query = "is:unread"
	}

	r, err := srv.Users.Messages.List(user).Q(query).MaxResults(maxResults).Do()
	if err != nil {
		return "", fmt.Errorf("API 請求失敗: %v", err)
	}

	if len(r.Messages) == 0 {
		return "", nil // 回傳空字串表示無結果
	}

	var result strings.Builder
	for _, m := range r.Messages {
		msg, err := srv.Users.Messages.Get(user, m.Id).Format("metadata").Do()
		if err != nil {
			continue
		}

		subject := ""
		from := ""
		for _, h := range msg.Payload.Headers {
			if h.Name == "Subject" {
				subject = h.Value
			}
			if h.Name == "From" {
				from = h.Value
			}
		}

		result.WriteString(fmt.Sprintf("【寄件者】: %s\n", from))
		result.WriteString(fmt.Sprintf("【主旨】: %s\n", subject))
		result.WriteString(fmt.Sprintf("【內容摘要】: %s\n", msg.Snippet))
		result.WriteString("------------------------------------------\n")
	}

	return result.String(), nil
}
