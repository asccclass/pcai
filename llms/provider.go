package llms

import (
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/ollama/ollama/api"
)

// Provider 定義了所有 LLM 供應商必須實作的方法
type Provider interface {
	Chat(messages []ollama.Message) (ollama.Message, error)
	Name() string
}

// ChatStreamFunc 定義了通用的 LLM 聊天函式簽名
type ChatStreamFunc func(modelName string, messages []ollama.Message, tools []api.Tool, opts ollama.Options, callback func(string)) (ollama.Message, error)
