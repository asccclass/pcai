package llms

import (
	"errors"
	"os"
	"strings"

	"github.com/asccclass/pcai/llms/copilot"
	"github.com/asccclass/pcai/llms/ollama"
)

// GetProvider 回傳指定名稱的 Provider 函式
// 目前支援: "ollama" (預設), "copilot" (GitHub Copilot)
func GetProviderFunc(providerName string) (ChatStreamFunc, error) {
	switch strings.ToLower(providerName) {
	case "ollama", "": // 預設為 Ollama
		return ollama.ChatStream, nil
	case "copilot":
		return copilot.ChatStream, nil
	default:
		return nil, errors.New("unsupported provider: " + providerName)
	}
}

// GetDefaultChatStream 回傳根據 PCAI_PROVIDER 環境變數設定的 ChatStreamFunc
// 供背景服務(排程、歸納、Heartbeat)使用，不再寫死 Ollama
func GetDefaultChatStream() ChatStreamFunc {
	provider := os.Getenv("PCAI_PROVIDER")
	fn, err := GetProviderFunc(provider)
	if err != nil {
		// Fallback: Ollama
		fn, _ = GetProviderFunc("ollama")
	}
	return fn
}
