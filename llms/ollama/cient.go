package ollama

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// Options 定義模型參數，用於調整 AI 的行為風格
type Options struct {
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
}

// Message 代表對話中的一則訊息（符合 Ollama /api/chat 標準）
type Message struct {
	Role      string         `json:"role"`                 // system, user, assistant, tool
	Content   string         `json:"content"`              // 訊息內容
	Images    []string       `json:"images,omitempty"`     // 支援視覺模型 (Base64 陣列)
	ToolCalls []api.ToolCall `json:"tool_calls,omitempty"` // AI 請求的工具呼叫
}

// ChatRequest 定義發送至 /api/chat 的資料結構
type ChatRequest struct {
	Model    string     `json:"model"`
	Messages []Message  `json:"messages"`
	Tools    []api.Tool `json:"tools,omitempty"`   // 工具定義清單
	Stream   bool       `json:"stream"`            // 是否啟用串流回傳
	Options  Options    `json:"options,omitempty"` // 模型參數
}

// ChatResponseChunk 是串流回傳時每一小塊資料的格式
type ChatResponseChunk struct {
	Model     string  `json:"model"`
	CreatedAt string  `json:"created_at"`
	Message   Message `json:"message"`
	Done      bool    `json:"done"`
}

// ChatStream 負責發送請求並處理串流回傳
// 回傳完整的 Assistant Message，方便上層更新 Session 歷史
func ChatStream(modelName string, messages []Message, tools []api.Tool, opts Options, callback func(string)) (Message, error) {
	reqBody := ChatRequest{
		Model:    modelName,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
		Options:  opts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, fmt.Errorf("JSON 解析失敗: %v", err)
	}

	// 預設 Ollama 位址，可透過設定檔或環境變數擴充
	ollamaURL := os.Getenv("OLLAMA_HOST")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	// 處理 URL 結尾
	ollamaURL = strings.TrimSuffix(ollamaURL, "/")

	var resp *http.Response
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		// 每次重試都需要一個新的 Reader，因為 http.Post 會讀取它
		resp, err = http.Post(ollamaURL+"/api/chat", "application/json", bytes.NewBuffer(jsonData))
		if err == nil {
			break
		}

		// 若發生連線錯誤 (例如 connectex)，等待後重試
		if i < maxRetries-1 {
			fmt.Printf("⚠️ 連線至 Ollama 失敗 (嘗試 %d/%d): %v\n⏳ 3秒後重試...\n", i+1, maxRetries, err)
			time.Sleep(3 * time.Second)
		}
	}

	if err != nil {
		return Message{}, fmt.Errorf("連線至 Ollama 失敗 (已重試 %d 次): %v", maxRetries, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("Ollama 回傳錯誤碼: %d", resp.StatusCode)
	}

	var fullAssistantMsg Message
	fullAssistantMsg.Role = "assistant"

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk ChatResponseChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}

		// 處理 AI 生成的文字內容
		if chunk.Message.Content != "" {
			fullAssistantMsg.Content += chunk.Message.Content
			callback(chunk.Message.Content) // 即時回傳片段給 UI 顯示
		}

		// 處理 AI 請求的工具呼叫
		if len(chunk.Message.ToolCalls) > 0 {
			// 如果 fullAssistantMsg 還沒有這組工具呼叫，才加入
			// 或者簡單地說：在串流模式下，通常最後一個包含 ToolCalls 的封包才是完整的
			// 這裡我們用覆蓋而非 append，或是比對 ID
			fullAssistantMsg.ToolCalls = chunk.Message.ToolCalls
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return Message{}, fmt.Errorf("串流讀取錯誤: %v", err)
	}

	return fullAssistantMsg, nil
}

// EncodeImageToBase64 將圖片檔案路徑轉換為 Base64 字串，供視覺模型使用
func EncodeImageToBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("讀取圖片失敗: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
