package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
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

	// 2. 配置 OAuth2
	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope)
	if err != nil {
		return "", fmt.Errorf("解析憑證失敗: %v", err)
	}

	// 3. 取得授權的 HTTP Client
	client := getClient(config)

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
			result.WriteString(fmt.Sprintf("【寄件者】: %s\n", from))
			result.WriteString(fmt.Sprintf("【主旨】: %s\n", subject))
			result.WriteString(fmt.Sprintf("【內容摘要】: %s\n", msg.Snippet))
			result.WriteString("------------------------------------------\n")
		}
	}

	return result.String(), nil
}

// --- OAuth2 輔助函式 ---

func getClient(config *oauth2.Config) *http.Client {
	tokenFile := "token.json"
	tok, err := tokenFromFile(tokenFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokenFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("⚠️  請在瀏覽器開啟以下連結授權 PCAI 存取 Gmail:\n\n%v\n\n請輸入驗證碼: ", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("無法讀取驗證碼: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("換取 Token 失敗: %v", err)
	}
	return tok
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("儲存 Token 失敗: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
