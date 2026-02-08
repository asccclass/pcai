package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ollama/ollama/api"
)

// FetchURLTool Wrapper for the robust FetchURL function
type FetchURLTool struct{}

func (t *FetchURLTool) Name() string { return "fetch_url" }

func (t *FetchURLTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "fetch_url",
			Description: "獲取指定網址的純文字內容。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"url": {
						"type": "string",
						"description": "Target URL to fetch"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"url"},
				}
			}(),
		},
	}
}

func (t *FetchURLTool) Run(argsJSON string) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	// Use the robust FetchURL function
	return FetchURL(args.URL)
}

// --- Robust FetchURL Implementation (merged from httptools.go) ---

// FetchConfig 存放抓取的設定
type FetchConfig struct {
	Timeout  time.Duration
	Headers  map[string]string
	SavePath string // 如果為空字串，則不寫入檔案
}

// Option 定義修改設定的函數類型
type Option func(*FetchConfig)

// defaultFetchConfig 回傳預設設定
func defaultFetchConfig() *FetchConfig {
	return &FetchConfig{
		Timeout: 60 * time.Second, // 預設 60 秒超時
		Headers: map[string]string{
			"User-Agent": "Mozilla/5.0 (compatible; Go-Fetcher/1.0)",
		},
		SavePath: "", // 預設不存檔
	}
}

// WithTimeout 設定超時時間
func WithTimeout(d time.Duration) Option {
	return func(c *FetchConfig) {
		c.Timeout = d
	}
}

// WithHeader 加入或覆蓋 HTTP Header
func WithHeader(key, value string) Option {
	return func(c *FetchConfig) {
		c.Headers[key] = value
	}
}

// WithUserAgent 快速設定 User-Agent
func WithUserAgent(ua string) Option {
	return func(c *FetchConfig) {
		c.Headers["User-Agent"] = ua
	}
}

// WithSaveToFile 啟用寫入檔案功能
func WithSaveToFile(path string) Option {
	return func(c *FetchConfig) {
		c.SavePath = path
	}
}

// FetchURL 執行 HTTP GET 請求
// 輸入: url (string), opts (不定長度選項)
// 輸出: 內容 ([]byte), 錯誤 (error)
func FetchURL(url string, opts ...Option) (string, error) {
	// 1. 套用設定
	config := defaultFetchConfig()
	for _, opt := range opts {
		opt(config)
	}

	// 2. 建立 Context 以控制超時
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	// 3. 建立 Request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("建立請求失敗: %w", err)
	}

	// 4. 設定 Headers
	for k, v := range config.Headers {
		req.Header.Set(k, v)
	}

	// 5. 執行請求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("請求發送失敗: %w", err)
	}
	defer resp.Body.Close()

	// 6. 檢查狀態碼
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP 錯誤狀態碼: %d", resp.StatusCode)
	}

	// 7. 讀取內容
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("讀取內容失敗: %w", err)
	}

	// 8. (選配) 寫入檔案
	if config.SavePath != "" {
		err := os.WriteFile(config.SavePath, bodyBytes, 0644)
		if err != nil {
			return string(bodyBytes), fmt.Errorf("寫入檔案失敗 (%s): %w", config.SavePath, err)
		}
		fmt.Printf("[Info] 內容已儲存至: %s\n", config.SavePath)
	}

	// Limit content length for AI consumption
	content := string(bodyBytes)
	if len(content) > 10000 {
		content = content[:10000] + "...(truncated)"
	}
	return content, nil
}
