package llms

import "github.com/asccclass/pcai/llms/ollama"

// Provider 定義了所有 LLM 供應商必須實作的方法
type Provider interface {
	Chat(messages []ollama.Message) (ollama.Message, error)
	Name() string
}
