package llms

import "github.com/asccclass/pcai/llms/ollama"

type OpenAIProvider struct {
	APIKey string
	Model  string
	URL    string
}

func (p *OpenAIProvider) Chat(messages []ollama.Message) (ollama.Message, error) {
	// 將 PCAI 內部的 Message 格式轉換為 OpenAI 格式
	// 發送 HTTP 請求至 p.URL (例如 https://api.openai.com/v1/chat/completions)
	// 回傳結果...
	return ollama.Message{Role: "assistant", Content: "來自外部 LLM 的回覆"}, nil
}
