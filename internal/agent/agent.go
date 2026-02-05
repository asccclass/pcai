package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms"
	"github.com/asccclass/pcai/llms/ollama"
)

// Agent 封裝了對話邏輯、工具呼叫與 Session 管理
type Agent struct {
	Session      *history.Session
	ModelName    string
	SystemPrompt string
	Registry     *core.Registry
	Options      ollama.Options
	Provider     llms.ChatStreamFunc // [NEW] 抽象化的 Provider

	// Callbacks for UI interaction
	OnGenerateStart        func()
	OnModelMessageComplete func(content string)
	OnToolCall             func(name, args string)
}

// NewAgent 建立一個新的 Agent 實例
func NewAgent(modelName, systemPrompt string, session *history.Session, registry *core.Registry) *Agent {
	// 預設使用 Ollama
	defaultProvider, _ := llms.GetProviderFunc("ollama")

	return &Agent{
		Session:      session,
		ModelName:    modelName,
		SystemPrompt: systemPrompt,
		Registry:     registry,
		Options:      ollama.Options{Temperature: 0.7, TopP: 0.9},
		Provider:     defaultProvider,
	}
}

// SetModelConfig update the model and provider dynamically
func (a *Agent) SetModelConfig(modelName string, provider llms.ChatStreamFunc) {
	if modelName != "" {
		a.ModelName = modelName
	}
	if provider != nil {
		a.Provider = provider
	}
}

// Chat 處理使用者輸入，執行思考與工具呼叫迴圈
// onStream 是即時輸出 AI 回應的回調函式
func (a *Agent) Chat(input string, onStream func(string)) (string, error) {
	// 將使用者輸入加入對話歷史
	a.Session.Messages = append(a.Session.Messages, ollama.Message{Role: "user", Content: input})

	var finalResponse string

	// Tool-Calling 狀態機循環
	for {
		var currentResponse strings.Builder
		toolDefs := a.Registry.GetDefinitions()

		// 觸發生成開始回調 (供 UI 顯示 "Thinking..." 提示)
		if a.OnGenerateStart != nil {
			a.OnGenerateStart()
		}

		// 呼叫 Provider 進行對話串流 (不再寫死 ollama.ChatStream)
		if a.Provider == nil {
			return "", fmt.Errorf("Agent Provider 未設定")
		}

		aiMsg, err := a.Provider(
			a.ModelName,
			a.Session.Messages,
			toolDefs,
			a.Options,
			func(content string) {
				currentResponse.WriteString(content)
				if onStream != nil {
					onStream(content)
				}
			},
		)

		if err != nil {
			return "", fmt.Errorf("AI 思考錯誤: %v", err)
		}

		// 累積最終回應
		if aiMsg.Content != "" {
			finalResponse = aiMsg.Content
			// 觸發訊息完成回調 (供 UI 渲染 Markdown)
			if a.OnModelMessageComplete != nil {
				a.OnModelMessageComplete(finalResponse)
			}
		}

		// 將 AI 回應加入歷史
		a.Session.Messages = append(a.Session.Messages, aiMsg)

		// 檢查是否呼叫工具
		if len(aiMsg.ToolCalls) == 0 {
			break // 最終回答完畢，跳出循環
		}

		// 執行工具
		for _, tc := range aiMsg.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			argsStr := string(argsJSON)

			// 觸發工具呼叫回調 (供 UI 顯示 "Executing..." 提示)
			if a.OnToolCall != nil {
				a.OnToolCall(tc.Function.Name, argsStr)
			}

			result, toolErr := a.Registry.CallTool(tc.Function.Name, argsStr)

			// --- 強化背景執行的反饋 ---
			var toolFeedback string
			if toolErr != nil {
				toolFeedback = fmt.Sprintf("【執行失敗】：%v", toolErr)
			} else {
				// 如果結果包含 "背景啟動"，則給予強大的確認標記
				if strings.Contains(result, "背景啟動") {
					aiMsg.ToolCalls = nil // 💡 強制清除，防止 AI 腦袋卡住
				} else {
					if tc.Function.Name == "list_tasks" && strings.Contains(result, "沒有任何背景任務") {
						// 讓 AI 知道現在是空的，讓它發揮創意回答
						result = "【系統資訊】：當前背景任務清單為空。請以助理身份告知使用者你目前正待命中。"
					} else {
						toolFeedback = fmt.Sprintf("【SYSTEM】: %s", result)
					}
				}
			}

			// 將工具執行結果加入歷史
			a.Session.Messages = append(a.Session.Messages, ollama.Message{
				Role:    "tool",
				Content: toolFeedback,
			})
		}
	}

	return finalResponse, nil
}
