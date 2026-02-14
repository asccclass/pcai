package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/ollama/ollama/api"
)

// Agent 封裝了對話邏輯、工具呼叫與 Session 管理
type Agent struct {
	Session      *history.Session
	ModelName    string
	SystemPrompt string
	Registry     *core.Registry
	Options      ollama.Options
	Provider     llms.ChatStreamFunc
	Logger       *SystemLogger // [NEW] 系統日誌

	// Callbacks for UI interaction
	OnGenerateStart        func()
	OnModelMessageComplete func(content string)
	OnToolCall             func(name, args string)
	OnToolResult           func(result string)
	OnShortTermMemory      func(source, content string) // 短期記憶自動存入回調
}

// NewAgent 建立一個新的 Agent 實例
func NewAgent(modelName, systemPrompt string, session *history.Session, registry *core.Registry, logger *SystemLogger) *Agent {
	// 預設使用 Ollama
	defaultProvider, _ := llms.GetProviderFunc("ollama")

	return &Agent{
		Session:      session,
		ModelName:    modelName,
		SystemPrompt: systemPrompt,
		Registry:     registry,
		Options:      ollama.Options{Temperature: 0.7, TopP: 0.9},
		Provider:     defaultProvider,
		Logger:       logger,
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
	// [LOG] 記錄使用者輸入
	if a.Logger != nil {
		a.Logger.LogUserInput(input)
	}

	// [TOOL HINT] 根據關鍵字注入工具提示，引導 LLM 選擇正確工具
	userContent := input
	if hint := getToolHint(input); hint != "" {
		userContent = input + "\n\n" + hint
	}

	// 將使用者輸入加入對話歷史
	a.Session.Messages = append(a.Session.Messages, ollama.Message{Role: "user", Content: userContent})

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
			// [LOG] 記錄錯誤
			if a.Logger != nil {
				a.Logger.LogError("AI 思考錯誤", err)
			}
			return "", fmt.Errorf("AI 思考錯誤: %v", err)
		}

		// [FIX] 補救措施：如果 ToolCalls 為空，但 Content 看起來像是 JSON 工具呼叫
		if len(aiMsg.ToolCalls) == 0 {
			content := strings.TrimSpace(aiMsg.Content)
			if strings.HasPrefix(content, "{") && strings.Contains(content, "\"name\"") {
				// 嘗試解析這種非標準的 JSON 輸出
				// 例如: {"type": "function", "name": "fs_append_to_file", "parameters": {...}}
				var rawCall struct {
					Name       string                         `json:"name"`
					Parameters *api.ToolCallFunctionArguments `json:"parameters"` // 改變為指標以允許 nil 檢查
					Arguments  *api.ToolCallFunctionArguments `json:"arguments"`
				}

				// 嘗試抓取 JSON 區塊 (以防前後有文字)
				start := strings.Index(content, "{")
				end := strings.LastIndex(content, "}")
				if start != -1 && end != -1 && end > start {
					jsonStr := content[start : end+1]
					if err := json.Unmarshal([]byte(jsonStr), &rawCall); err == nil && rawCall.Name != "" {
						fmt.Printf("🔍 [Agent] 偵測到原始 JSON 工具呼叫: %s\n", rawCall.Name)

						// 參數相容性處理: 有些模型會用 parameters 代替 arguments
						var finalArgs api.ToolCallFunctionArguments

						if rawCall.Arguments != nil {
							finalArgs = *rawCall.Arguments
						} else if rawCall.Parameters != nil {
							finalArgs = *rawCall.Parameters
						} else {
							// 若皆無，保持 zero value (假設 api.ToolCallFunctionArguments 是一個 struct，zero value 可用)
							finalArgs = api.ToolCallFunctionArguments{}
						}

						// 建構標準 ToolCall
						aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
							Function: api.ToolCallFunction{
								Name:      rawCall.Name,
								Arguments: finalArgs,
							},
						})

						// 清空 Content 以免重複顯示 JSON 給使用者
						// 但如果只有 JSON，我們將其清空；如果有其他解釋文字，可能要保留？
						// 這裡選擇清空，因為我們已經轉成執行動作了
						aiMsg.Content = ""
						finalResponse = "" // 清除已累積的 Content，避免被 OnModelMessageComplete 印出
					}
				}
			}
		}

		// 累積最終回應 (移動到這裡，確保 fallback 處理完後再決定是否觸發回調)
		if aiMsg.Content != "" {
			// 如果 fallback 成功，這裡 Content 會變空，就不會觸發回調
			finalResponse = aiMsg.Content
			// 觸發訊息完成回調 (供 UI 渲染 Markdown)
			if a.OnModelMessageComplete != nil {
				a.OnModelMessageComplete(finalResponse)
			}
			// [LOG] 記錄 AI 回應
			if a.Logger != nil {
				a.Logger.LogAIResponse(finalResponse)
			}
		}

		// 將 AI 回應加入歷史 (移到處理完 Content 之後)
		a.Session.Messages = append(a.Session.Messages, aiMsg)

		// 檢查是否呼叫工具
		if len(aiMsg.ToolCalls) == 0 {
			break // 最終回答完畢，跳出循環
		}

		// 執行工具
		for _, tc := range aiMsg.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			argsStr := string(argsJSON)

			// [LOG] 記錄工具呼叫
			if a.Logger != nil {
				a.Logger.LogToolCall(tc.Function.Name, argsStr)
			}

			// 觸發工具呼叫回調 (供 UI 顯示 "Executing..." 提示)
			if a.OnToolCall != nil {
				a.OnToolCall(tc.Function.Name, argsStr)
			}

			result, toolErr := a.Registry.CallTool(tc.Function.Name, argsStr)

			// [LOG] 記錄工具結果
			if a.Logger != nil {
				a.Logger.LogToolResult(tc.Function.Name, result, toolErr)
			}

			// [SHORT-TERM MEMORY] 將工具回應自動存入短期記憶
			if toolErr == nil && result != "" && a.OnShortTermMemory != nil {
				// 根據工具名稱決定來源分類
				source := toolNameToMemorySource(tc.Function.Name)
				if source != "" {
					a.OnShortTermMemory(source, result)
				}
			}

			// --- 強化背景執行的反饋 ---
			var toolFeedback string
			if toolErr != nil {
				toolFeedback = fmt.Sprintf("【執行失敗】：%v", toolErr)
				// [NEW] 攔截幻覺 (Hallucination) 並記錄
				if strings.Contains(toolErr.Error(), "找不到工具") {
					// 為了避免 circular dependency，這裡我們不直接 import tools，
					// 但因為 ReportMissingTool 在 tools package，而 tools import agent，
					// 所以 agent 不能 import tools。這是一個架構問題。
					// 解法：
					// 1. 將 LogMissingToolEvent 移到 internal/agent 或 internal/core (最乾淨)
					// 2. 定義一個 Callback 讓 InitRegistry 注入 (最快)

					// 由於時間限制，我們採用 "定義 Callback" 的方式。
					// 參見 Agent struct 的 OnToolResult 或新增一個 OnHallucination?
					// 為了簡單，我們直接在 result string 提示使用者系統無此工具。
					// 並依賴 `ReportMissingTool` 讓 LLM *主動* 回報。
					// 但使用者說 "不要亂猜"，"若需要的功能系統沒有...記錄至 botmemory/notools.log"。

					// 我們可以將 LogMissingToolEvent 的邏輯複製一份在這裡 (或移至 internal/utils?)
					// 為了符合 "Clean Architecture"，我們不該讓 agent 依賴 tools。
					// 讓我們把 LogMissingToolEvent 移到 internal/core/definition.go 或 internal/agent/logger.go?
					//
					// 其實 agent 已經有 Logger 了 (*SystemLogger)。我們可以加一個 LogHallucination 方法。
					if a.Logger != nil {
						a.Logger.LogHallucination(input, tc.Function.Name) // 需實作
					}
				}
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

			// 觸發結果回調
			if a.OnToolResult != nil {
				msgToPrint := result
				if toolFeedback != "" {
					msgToPrint = toolFeedback
				}
				a.OnToolResult(msgToPrint)
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

// toolNameToMemorySource 將工具名稱對應到短期記憶的來源分類
// 返回空字串表示不需要儲存
func toolNameToMemorySource(toolName string) string {
	sourceMap := map[string]string{
		"get_taiwan_weather": "weather",
		"read_calendars":     "calendar",
		"read_email":         "email",
		"web_search":         "search",
		"knowledge_search":   "knowledge_query",
	}
	if source, ok := sourceMap[toolName]; ok {
		return source
	}
	return ""
}
