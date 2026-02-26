package gateway

import (
	"log"
	"os"
	"strings"

	"github.com/asccclass/pcai/llms"
)

// RouteResult 包含了路由決策的結果
type RouteResult struct {
	ModelName string
	Provider  llms.ChatStreamFunc
}

// Router 負責根據使用者輸入決定使用哪個模型與供應商
type Router struct {
	DefaultModel string
}

// NewRouter 建立新的路由
func NewRouter(defaultModel string) *Router {
	return &Router{
		DefaultModel: defaultModel,
	}
}

// Route 執行路由邏輯
func (r *Router) Route(input string) (*RouteResult, error) {
	// 簡單的規則範例：
	// 如果開頭是 /code -> 使用 Claude (需實作 Claude Provider)
	// 如果開頭是 /gpt -> 使用 OpenAI
	// 否則 -> 使用預設 (Ollama)

	var targetModel string
	var providerName string

	switch {
	case strings.HasPrefix(input, "/copilot"):
		copilotModel := os.Getenv("PCAI_MODEL")
		if copilotModel == "" {
			copilotModel = "gpt-4o"
		}
		targetModel = copilotModel
		providerName = "copilot"
	case strings.HasPrefix(input, "/code"):
		targetModel = "claude-3-5-sonnet-latest"
		providerName = "claude"
	case strings.HasPrefix(input, "/gpt"):
		targetModel = "gpt-4o"
		providerName = "openai"
	default:
		defaultProvider := os.Getenv("PCAI_PROVIDER")
		if defaultProvider == "" {
			defaultProvider = "ollama"
		}
		targetModel = r.DefaultModel
		providerName = defaultProvider
	}

	provider, err := llms.GetProviderFunc(providerName)
	if err != nil {
		// 如果找不到該 Provider (例如 claude 未實作)，降級回預設
		log.Printf("⚠️ 路由失敗 (%s): %v。降級回預設模型。", providerName, err)
		targetModel = r.DefaultModel
		providerName = "ollama"
		provider, _ = llms.GetProviderFunc("ollama")
	}

	return &RouteResult{
		ModelName: targetModel,
		Provider:  provider,
	}, nil
}
