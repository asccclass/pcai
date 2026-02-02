// Ollama (AI 連接層)
// 定義介面與 Ollama 實作。這樣設計的好處是，未來你想換成 OpenAI，只需要新增一個 struct 實作 Embedder 介面即可，
// 不用改 Manager 的程式碼

package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Embedder 定義了將文字轉換為向量的通用介面
type Embedder interface {
	GetEmbedding(text string) ([]float32, error)
}

// OllamaEmbedder 實作 Embedder 介面
type OllamaEmbedder struct {
	BaseURL string
	Model   string
}

// NewOllamaEmbedder 建構函式
func NewOllamaEmbedder(baseUrl, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		BaseURL: baseUrl,
		Model:   model,
	}
}

// 內部使用的請求結構
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// 內部使用的回應結構
type ollamaResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

func (o *OllamaEmbedder) GetEmbedding(text string) ([]float32, error) {
	reqBody := ollamaRequest{
		Model:  o.Model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // 嵌入可能較慢，給多點時間
	defer cancel()

	url := fmt.Sprintf("%s/api/embeddings", o.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("連線 Ollama 失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API Error [%d]: %s", resp.StatusCode, string(body))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Error != "" {
		return nil, fmt.Errorf("Ollama 錯誤: %s", result.Error)
	}

	return result.Embedding, nil
}
