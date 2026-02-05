package llms

import (
	"errors"
	"strings"

	"github.com/asccclass/pcai/llms/ollama"
)

// GetProvider 回傳指定名稱的 Provider 函式
// 目前支援: "ollama" (預設), "claude" (待實作), "openai" (待實作)
func GetProviderFunc(providerName string) (ChatStreamFunc, error) {
	switch strings.ToLower(providerName) {
	case "ollama", "": // 預設為 Ollama
		return ollama.ChatStream, nil
	// 未來擴充:
	// case "claude":
	// 	   return claude.ChatStream, nil
	default:
		return nil, errors.New("unsupported provider: " + providerName)
	}
}
