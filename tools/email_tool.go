package tools

import (
	"encoding/json"
	"fmt"

	"github.com/asccclass/pcai/internal/gmail"
	"github.com/ollama/ollama/api"
)

// EmailTool 讓 LLM 可以主動查詢一般郵件
type EmailTool struct{}

type EmailToolArgs struct {
	Query      string `json:"query,omitempty"`
	MaxResults int64  `json:"max_results,omitempty"`
}

func (t *EmailTool) Name() string { return "read_email" }

func (t *EmailTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "read_email",
			Description: "讀取使用者的 Gmail 郵件。當使用者詢問「有沒有新信」、「查看最近的 Email」時使用。可選填 query 參數進行關鍵字搜尋。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "搜尋關鍵字 (例如: 'from:boss', 'subject:meeting', 'is:unread')"
					},
					"max_results": {
						"type": "integer",
						"description": "要讀取的最大郵件數量 (預設 5)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{},
				}
			}(),
		},
	}
}

func (t *EmailTool) Run(args string) (string, error) {
	var a EmailToolArgs
	if args != "" {
		_ = json.Unmarshal([]byte(args), &a)
	}
	if a.MaxResults <= 0 {
		a.MaxResults = 5
	}

	// 轉換參數為 FilterConfig
	// 這裡我們需要修改 gmail package 讓它支援更靈活的 query
	// 目前 gmail.FetchLatestEmails 比較針對「背景監控」設計 (只抓未讀)
	// 我們可能需要一個新的函式 FetchEmailsByQuery

	// 暫時使用 FetchLatestEmails 的邏輯並稍作修改，或者在 gmail package 新增功能
	// 為了快速實作，我們假設 gmail.Worker 已經準備好被重構
	// 這裡直接呼叫我們即將新增的 gmail.FetchEmails(query, max)
	// 但因為不能直接改 internal/gmail/worker.go 的簽章 (怕壞掉)，我們新增一個。

	res, err := gmail.SearchEmails(a.Query, a.MaxResults)
	if err != nil {
		return "", fmt.Errorf("讀取郵件失敗: %v", err)
	}

	if res == "" {
		return "📭 找不到符合條件的郵件。", nil
	}

	return fmt.Sprintf("📧 **搜尋結果**:\n%s", res), nil
}
