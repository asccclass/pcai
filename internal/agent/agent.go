package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	ActiveBuffer *history.ActiveBuffer
	DailyLogger  *history.DailyLogger

	// Callbacks for UI interaction
	OnGenerateStart        func()
	OnModelMessageComplete func(content string)
	OnToolCall             func(name, args string)
	OnToolResult           func(result string)
	OnShortTermMemory      func(source, content string) // 短期記憶自動存入回調
	OnMemorySearch         func(query string) string    // 記憶預搜尋回調
	OnCheckPendingPlan     func() string                // 未完成任務檢查回調
	OnAcquireTaskLock      func() bool                  // 獲取任務鎖
	OnReleaseTaskLock      func()                       // 釋放任務鎖
	OnIsTaskLocked         func() bool                  // 檢查任務鎖
}

// NewAgent 建立一個新的 Agent 實例
func NewAgent(modelName, systemPrompt string, session *history.Session, registry *core.Registry, logger *SystemLogger) *Agent {
	// 預設使用 Ollama
	defaultProvider, _ := llms.GetProviderFunc("ollama")

	// 初始化每日日誌與 Active Buffer
	home, _ := os.Getwd()
	kbDir := filepath.Join(home, "botmemory")
	dailyLogger := history.NewDailyLogger(kbDir)
	activeBuffer := history.NewActiveBuffer(4000, dailyLogger)

	// 自動恢復今日會話
	entries, _ := dailyLogger.LoadToday()
	for _, e := range entries {
		activeBuffer.Add(ollama.Message{Role: e.Role, Content: e.Content})
	}

	return &Agent{
		Session:      session,
		ModelName:    modelName,
		SystemPrompt: systemPrompt,
		Registry:     registry,
		Options:      ollama.Options{Temperature: 0.7, TopP: 0.9},
		Provider:     defaultProvider,
		Logger:       logger,
		ActiveBuffer: activeBuffer,
		DailyLogger:  dailyLogger,
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
	var lastPendingID string
	if a.Session != nil {
		for i := len(a.Session.Messages) - 1; i >= 0; i-- {
			msg := a.Session.Messages[i]
			if strings.Contains(msg.Content, "pending_") {
				re := regexp.MustCompile(`pending_\d+`)
				matches := re.FindStringSubmatch(msg.Content)
				if len(matches) > 0 {
					lastPendingID = matches[0]
					break
				}
			}
		}
	}

	// [MULTI-STEP] 偵測多步驟意圖，若偵測到則注入計畫編排 Prompt
	multiStepDetected := false
	if multiStepHint := detectMultiStepIntent(input); multiStepHint != "" {
		// 檢查任務鎖：若已有任務在執行，不允許建立新計畫
		if a.OnIsTaskLocked != nil && a.OnIsTaskLocked() {
			fmt.Println("⚠️ [Agent] 已有任務在執行中，無法建立新計畫")
		} else {
			userContent = input + "\n\n" + multiStepHint
			multiStepDetected = true
			// 獲取任務鎖
			if a.OnAcquireTaskLock != nil {
				a.OnAcquireTaskLock()
			}
			fmt.Println("🧩 [Agent] 偵測到多步驟意圖，啟用計畫編排模式")
		}
	}

	// [TASK RECOVERY] 若非新計畫模式，檢查是否有未完成的計畫需要恢復
	if !multiStepDetected && a.OnCheckPendingPlan != nil {
		if resumeHint := a.OnCheckPendingPlan(); resumeHint != "" {
			// 檢查任務鎖
			if a.OnIsTaskLocked != nil && a.OnIsTaskLocked() {
				fmt.Println("⚠️ [Agent] 已有任務在執行中，跳過恢復")
			} else {
				userContent = input + "\n\n" + resumeHint
				// 獲取任務鎖
				if a.OnAcquireTaskLock != nil {
					a.OnAcquireTaskLock()
				}
				fmt.Println("🔄 [Agent] 偵測到未完成任務，注入恢復指令")
			}
		}
	}

	if hint := getToolHint(input, lastPendingID); hint != "" {
		userContent = userContent + "\n\n" + hint
	}

	// [MEMORY-FIRST] 搜尋記憶，注入相關上下文
	if a.OnMemorySearch != nil {
		if memCtx := a.OnMemorySearch(input); memCtx != "" {
			// 把記憶放在問題之前，讓 LLM 的注意力聚焦在最後的問題上
			userContent = memCtx + "\n\n【使用者問題】\n" + userContent
			fmt.Println("💾 [Memory] 記憶命中，已注入上下文")
		}
	}

	// [ACTIVE-BUFFER] 注入當前日誌上下文
	if a.ActiveBuffer != nil && len(a.ActiveBuffer.GetMessages()) > 0 {
		var activeCtx strings.Builder
		activeCtx.WriteString("【今日對話上下文】\n")
		for _, m := range a.ActiveBuffer.GetMessages() {
			activeCtx.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
		}
		userContent = activeCtx.String() + "\n\n" + userContent
	}

	// 將使用者輸入加入對話歷史
	msg := ollama.Message{Role: "user", Content: userContent}
	a.Session.Messages = append(a.Session.Messages, msg)

	// 記錄到 Active Buffer 和每日日誌
	if a.ActiveBuffer != nil {
		a.ActiveBuffer.Add(ollama.Message{Role: "user", Content: input}) // 記錄原始輸入
	}
	if a.DailyLogger != nil {
		_ = a.DailyLogger.Record(ollama.Message{Role: "user", Content: input})
	}

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
		// 有些情況下，LLM 甚至會一次輸出多個獨立的 JSON block
		if len(aiMsg.ToolCalls) == 0 {
			content := strings.TrimSpace(aiMsg.Content)

			// 尋找所有的 JSON blocks (替換為大括號計數邏輯，支援巢狀 JSON)
			matches := extractJSONBlocks(content)

			parsedCount := 0
			for _, jsonStr := range matches {
				// 嘗試解析這種非標準的 JSON 輸出
				// 例如: {"type": "function", "name": "fs_append_to_file", "parameters": {...}}
				var rawCall struct {
					Name       string                         `json:"name"`
					Action     string                         `json:"action"`     // Support "action" instead of "name"
					Parameters *api.ToolCallFunctionArguments `json:"parameters"` // 改變為指標以允許 nil 檢查
					Arguments  *api.ToolCallFunctionArguments `json:"arguments"`
				}

				if err := json.Unmarshal([]byte(jsonStr), &rawCall); err == nil {
					funcName := rawCall.Name
					if funcName == "" {
						funcName = rawCall.Action
					}

					// [FIX] 嘗試從參數特徵推斷 (如果 AI 漏寫 action/name)
					if funcName == "" {
						var inferMap map[string]interface{}
						if json.Unmarshal([]byte(jsonStr), &inferMap) == nil {
							// 若包含 content 和 category，高機率是 memory_save 的參數體
							if _, hasContent := inferMap["content"]; hasContent {
								if _, hasCategory := inferMap["category"]; hasCategory {
									funcName = "memory_save"
								}
							}
						}
					}

					if funcName != "" {
						fmt.Printf("🔍 [Agent] 偵測到原始 JSON 工具呼叫: %s\n", funcName)

						// 參數相容性處理: 有些模型會用 parameters 代替 arguments
						var finalArgs api.ToolCallFunctionArguments

						if rawCall.Arguments != nil {
							finalArgs = *rawCall.Arguments
						} else if rawCall.Parameters != nil {
							finalArgs = *rawCall.Parameters
						} else {
							// 嘗試將整個 JSON 視為 Arguments
							var fullArgs map[string]interface{}
							if err := json.Unmarshal([]byte(jsonStr), &fullArgs); err == nil {
								delete(fullArgs, "name")
								delete(fullArgs, "action")

								// Convert map to api.ToolCallFunctionArguments
								argsBytes, _ := json.Marshal(fullArgs)
								var convertedArgs api.ToolCallFunctionArguments
								_ = json.Unmarshal(argsBytes, &convertedArgs)
								finalArgs = convertedArgs
							}
						}

						// 建構標準 ToolCall
						aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
							Function: api.ToolCallFunction{
								Name:      funcName,
								Arguments: finalArgs,
							},
						})
						parsedCount++
					}
				}
			}

			if parsedCount > 0 {
				aiMsg.Content = ""
				finalResponse = ""
			}
		}

		// [FIX] 補救措施 2：處理 Python 風格的工具呼叫
		// Llama 有時會輸出 <|python_tag|>get_weather(city="苗栗") 格式
		// 或在文字中嵌入 function_name(key="value") 格式
		if len(aiMsg.ToolCalls) == 0 {
			content := strings.TrimSpace(aiMsg.Content)
			// 移除 <|python_tag|> 前綴
			cleaned := content
			if idx := strings.Index(cleaned, "<|python_tag|>"); idx != -1 {
				cleaned = strings.TrimSpace(cleaned[idx+len("<|python_tag|>"):])
			}

			// 匹配 function_name(key=value, key2=value2) 格式 (不限定行首行尾)
			// 例如: get_weather(city="苗栗") 或嵌入在自然語言文字中
			pyCallRe := regexp.MustCompile(`(\w+)\((\w+\s*=\s*(?:"[^"]*"|'[^']*'|\S+)(?:\s*,\s*\w+\s*=\s*(?:"[^"]*"|'[^']*'|\S+))*)\)`)
			if m := pyCallRe.FindStringSubmatch(cleaned); m != nil {
				funcName := m[1]
				argsStr := m[2]

				fmt.Printf("🔍 [Agent] 偵測到 Python 風格工具呼叫: %s(%s)\n", funcName, argsStr)

				// 解析 key=value 或 key="value" 參數
				argsMap := make(map[string]interface{})
				// 匹配 key="value" 或 key='value' 或 key=value
				argRe := regexp.MustCompile(`(\w+)\s*=\s*(?:"([^"]*)"|'([^']*)'|(\S+))`)
				for _, am := range argRe.FindAllStringSubmatch(argsStr, -1) {
					key := am[1]
					val := am[2] // double-quoted
					if val == "" {
						val = am[3] // single-quoted
					}
					if val == "" {
						val = am[4] // unquoted
					}
					argsMap[key] = val
				}

				// 轉換為 api.ToolCallFunctionArguments
				argsBytes, _ := json.Marshal(argsMap)
				var finalArgs api.ToolCallFunctionArguments
				_ = json.Unmarshal(argsBytes, &finalArgs)

				aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
					Function: api.ToolCallFunction{
						Name:      funcName,
						Arguments: finalArgs,
					},
				})

				aiMsg.Content = ""
				finalResponse = ""
			}

		}

		// [FIX] 補救措施 2.5：處理方括號風格的工具呼叫
		// Llama 有時會輸出 [get_taiwan_weather location="苗栗縣"] 格式
		if len(aiMsg.ToolCalls) == 0 {
			content := strings.TrimSpace(aiMsg.Content)
			// 匹配 [tool_name key="value" key2="value2"] 格式
			bracketRe := regexp.MustCompile(`\[(\w+)\s+((?:\w+\s*=\s*(?:"[^"]*"|'[^']*'|\S+)\s*)+)\]`)
			if m := bracketRe.FindStringSubmatch(content); m != nil {
				funcName := m[1]
				argsStr := m[2]

				fmt.Printf("🔍 [Agent] 偵測到方括號風格工具呼叫: [%s %s]\n", funcName, argsStr)

				// 解析 key=value 或 key="value" 參數
				argsMap := make(map[string]interface{})
				argRe := regexp.MustCompile(`(\w+)\s*=\s*(?:"([^"]*)"|'([^']*)'|(\S+))`)
				for _, am := range argRe.FindAllStringSubmatch(argsStr, -1) {
					key := am[1]
					val := am[2] // double-quoted
					if val == "" {
						val = am[3] // single-quoted
					}
					if val == "" {
						val = am[4] // unquoted
					}
					argsMap[key] = val
				}

				// 轉換為 api.ToolCallFunctionArguments
				argsBytes, _ := json.Marshal(argsMap)
				var finalArgs api.ToolCallFunctionArguments
				_ = json.Unmarshal(argsBytes, &finalArgs)

				aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
					Function: api.ToolCallFunction{
						Name:      funcName,
						Arguments: finalArgs,
					},
				})

				aiMsg.Content = ""
				finalResponse = ""
			}
		}

		// [FIX] 補救措施 2.6：處理裸工具名稱 + key="value" 的常見格式
		// Llama 有時會輸出 browser_open url="https://..." 格式
		if len(aiMsg.ToolCalls) == 0 {
			content := strings.TrimSpace(aiMsg.Content)
			// 匹配 tool_name key="value" key2="value2" 格式 (確保不在引號或括號內)
			nakedRe := regexp.MustCompile(`^([\w_]+)\s+((?:\w+\s*=\s*(?:"[^"]*"|'[^']*'|\S+)\s*)+)$`)

			// 為了處理可能前面有一些空行或提示詞，先切出最後一行來檢查
			lines := strings.Split(content, "\n")
			for i := len(lines) - 1; i >= 0; i-- {
				line := strings.TrimSpace(lines[i])
				if m := nakedRe.FindStringSubmatch(line); len(m) == 3 {
					funcName := m[1]
					argsStr := m[2]

					// 確保是真的工具名稱
					isValidTool := false
					for _, d := range toolDefs {
						if d.Function.Name == funcName {
							isValidTool = true
							break
						}
					}

					if isValidTool {
						fmt.Printf("🔍 [Agent] 偵測到裸參數風格工具呼叫: %s %s\n", funcName, argsStr)

						argsMap := make(map[string]interface{})
						argRe := regexp.MustCompile(`(\w+)\s*=\s*(?:"([^"]*)"|'([^']*)'|(\S+))`)
						for _, am := range argRe.FindAllStringSubmatch(argsStr, -1) {
							key := am[1]
							val := am[2] // double-quoted
							if val == "" {
								val = am[3] // single-quoted
							}
							if val == "" {
								val = am[4] // unquoted
							}
							argsMap[key] = val
						}

						argsBytes, _ := json.Marshal(argsMap)
						var finalArgs api.ToolCallFunctionArguments
						_ = json.Unmarshal(argsBytes, &finalArgs)

						aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
							Function: api.ToolCallFunction{
								Name:      funcName,
								Arguments: finalArgs,
							},
						})

						// 移除工具呼叫那一行，保留前面的說話內容
						lines[i] = ""
						aiMsg.Content = strings.Join(lines, "\n")
						finalResponse = aiMsg.Content
						break
					}
				}
			}
		}

		// [FIX] 補救措施 3：處理自然語言描述 + 非標準參數的模式
		// 支援的格式:
		//   (a) 裸 JSON 參數: "我會呼叫 get_taiwan_weather... { "location": "苗栗縣" }"
		//   (b) URL query string: "get_taiwan_weather?location=苗栗縣"
		if len(aiMsg.ToolCalls) == 0 {
			content := strings.TrimSpace(aiMsg.Content)

			// 在文字中搜尋已知工具名稱
			detectedTool := ""
			for _, tDef := range toolDefs {
				if strings.Contains(content, tDef.Function.Name) {
					detectedTool = tDef.Function.Name
					break
				}
			}

			if detectedTool != "" {
				var parsed bool

				// (a) 嘗試裸 JSON 參數
				start := strings.Index(content, "{")
				end := strings.LastIndex(content, "}")
				if start != -1 && end > start {
					jsonStr := content[start : end+1]
					var argsMap map[string]interface{}
					if err := json.Unmarshal([]byte(jsonStr), &argsMap); err == nil {
						if _, hasName := argsMap["name"]; !hasName {
							fmt.Printf("🔍 [Agent] 偵測到自然語言工具呼叫: %s + %s\n", detectedTool, jsonStr)
							argsBytes, _ := json.Marshal(argsMap)
							var finalArgs api.ToolCallFunctionArguments
							_ = json.Unmarshal(argsBytes, &finalArgs)
							aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
								Function: api.ToolCallFunction{
									Name:      detectedTool,
									Arguments: finalArgs,
								},
							})
							aiMsg.Content = ""
							finalResponse = ""
							parsed = true
						}
					}
				}

				// (b) 嘗試 URL query string: tool_name?key=value&key2=value2
				if !parsed {
					qsRe := regexp.MustCompile(regexp.QuoteMeta(detectedTool) + `\?([^\s]+)`)
					if m := qsRe.FindStringSubmatch(content); m != nil {
						queryStr := m[1]
						argsMap := make(map[string]interface{})
						for _, pair := range strings.Split(queryStr, "&") {
							kv := strings.SplitN(pair, "=", 2)
							if len(kv) == 2 {
								argsMap[kv[0]] = kv[1]
							}
						}
						if len(argsMap) > 0 {
							fmt.Printf("🔍 [Agent] 偵測到 URL query string 工具呼叫: %s?%s\n", detectedTool, queryStr)
							argsBytes, _ := json.Marshal(argsMap)
							var finalArgs api.ToolCallFunctionArguments
							_ = json.Unmarshal(argsBytes, &finalArgs)
							aiMsg.ToolCalls = append(aiMsg.ToolCalls, api.ToolCall{
								Function: api.ToolCallFunction{
									Name:      detectedTool,
									Arguments: finalArgs,
								},
							})
							aiMsg.Content = ""
							finalResponse = ""
						}
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
			if finalResponse != "" {
				if a.Logger != nil {
					a.Logger.LogAIResponse(finalResponse)
				}
				// 記錄到 Active Buffer 和每日日誌
				if a.ActiveBuffer != nil {
					a.ActiveBuffer.Add(ollama.Message{Role: "assistant", Content: finalResponse})
					// 檢查是否需要歸納
					if a.ActiveBuffer.ShouldSummarize() {
						fmt.Println("🧠 [Memory] 偵測到上下文過長，觸發自動歸納...")
						summarizeFunc := func(model string, prompt string) (string, error) {
							var res strings.Builder
							_, err := ollama.ChatStream(model, []ollama.Message{
								{Role: "system", Content: "你是一個對話摘要專家。請幫我精煉對話。"},
								{Role: "user", Content: prompt},
							}, nil, a.Options, func(c string) { res.WriteString(c) })
							return res.String(), err
						}
						_ = a.ActiveBuffer.TriggerSummarization(a.ModelName, summarizeFunc)
					}
				}
				if a.DailyLogger != nil {
					_ = a.DailyLogger.Record(ollama.Message{Role: "assistant", Content: finalResponse})
				}
			}
		}

		// 將 AI 回應加入歷史 (移到處理完 Content 之後)
		a.Session.Messages = append(a.Session.Messages, aiMsg)

		// 檢查是否呼叫工具
		if len(aiMsg.ToolCalls) == 0 {
			break // 最終回答完畢，跳出循環
		}

		// 執行工具
		forceBreakState := false
		var forcedAssistReply string

		for _, tc := range aiMsg.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Function.Arguments)
			argsStr := string(argsJSON)

			// [FIX] memory_save 防亂碼：將 LLM 生成的 content 替換為使用者原始輸入
			// 因為 LLM 的中文 tokenizer 會產生亂碼，但使用者原始輸入一定是正確的
			if tc.Function.Name == "memory_save" {
				var kaArgs map[string]interface{}
				if err := json.Unmarshal(argsJSON, &kaArgs); err == nil {
					// 替換 content 為使用者原始輸入
					kaArgs["content"] = input
					fmt.Printf("🔄 [Agent] memory_save 防亂碼：使用原始輸入替換 content\n")

					// 修正 category（LLM 可能產生亂碼分類名）
					if cat, ok := kaArgs["category"].(string); ok {
						validCategories := []string{"個人資訊", "工作紀錄", "偏好設定", "生活雜記", "技術開發"}
						// 嘗試匹配最接近的分類
						matched := false
						for _, vc := range validCategories {
							if strings.Contains(cat, vc) || strings.Contains(vc, cat) {
								kaArgs["category"] = vc
								matched = true
								break
							}
						}
						if !matched {
							// 根據使用者輸入的內容語義自動判斷分類
							lowerInput := strings.ToLower(input)
							if strings.Contains(lowerInput, "叫") || strings.Contains(lowerInput, "名") ||
								strings.Contains(lowerInput, "生日") || strings.Contains(lowerInput, "住") ||
								strings.Contains(lowerInput, "電話") || strings.Contains(lowerInput, "稱呼") {
								kaArgs["category"] = "個人資訊"
							} else if strings.Contains(lowerInput, "工作") || strings.Contains(lowerInput, "專案") ||
								strings.Contains(lowerInput, "會議") || strings.Contains(lowerInput, "任職") {
								kaArgs["category"] = "工作紀錄"
							} else if strings.Contains(lowerInput, "喜歡") || strings.Contains(lowerInput, "偏好") ||
								strings.Contains(lowerInput, "習慣") {
								kaArgs["category"] = "偏好設定"
							} else {
								kaArgs["category"] = "個人資訊" // 預設分類
							}
							fmt.Printf("🔄 [Agent] memory_save 分類校正: '%s' → '%s'\n", cat, kaArgs["category"])
						}
					}

					fixedArgs, _ := json.Marshal(kaArgs)
					argsStr = string(fixedArgs)
				}
			}

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
					// 根據環境變數決定是否啟用自動創建技能的提示
					enableAutoSkill := os.Getenv("ENABLE_AUTO_SKILL_CREATION")
					if enableAutoSkill == "false" {
						toolFeedback = fmt.Sprintf("【系統回饋】：您嘗試呼叫的工具 '%s' 不存在或無權限。系統管理員已停用自動建立技能的功能，請停止嘗試使用不存在的工具。", tc.Function.Name)
					} else {
						toolFeedback = fmt.Sprintf("【系統回饋】：您嘗試呼叫的工具 '%s' 不存在或無權限。\n\n"+
							"⚠️ **觸發「自我演化協議」(Self-Evolution Protocol)** ⚠️\n"+
							"系統偵測到您試圖使用尚未實作的能力。請按照以下步驟自主創建此工具：\n"+
							"1. **分析需求**: 判斷此功能是否可透過 OS 指令 (如 bash, powershell) 或簡單腳本達成。\n"+
							"2. **測試解決方案**: 使用 `shell_exec` 嘗試執行相關指令，確認輸出符合預期。\n"+
							"3. **創建技能骨架**: 使用 `skill_scaffold` 建立新技能目錄 (例如: `skill_scaffold(name=\"%s\", ...)` )。\n"+
							"4. **實作與寫入**: 使用 `fs_write_file` 將測試成功的指令或腳本寫入 `SKILL.md` 或對應檔案。\n"+
							"5. **註冊技能**: 使用 `reload_skills` 載入新技能。\n"+
							"6. **最終執行**: 再次呼叫新創建的工具 `%s` 來完成原始任務。\n\n"+
							"或者，你可以直接呼叫 `generate_skill(goal='...')` 讓我為你自動生成此技能！", tc.Function.Name, tc.Function.Name, tc.Function.Name)
					}

					// 為了符合 Clean Architecture，這裡只呼叫 Logger 的介面
					// 若 Logger 有實作 LogHallucination 則會被呼叫
					if a.Logger != nil {
						// a.Logger.LogHallucination(input, tc.Function.Name) // 暫時註解，等待 Logger 實作
						a.Logger.LogError(fmt.Sprintf("Hallucination detected: %s", tc.Function.Name), toolErr)
					}
				}
			} else {
				// 如果結果包含 "背景啟動" 或需要詢問使用者確認，則給予強大的確認標記並中斷連續呼叫
				if strings.Contains(result, "背景啟動") || strings.Contains(result, "請務必詢問使用者") {
					toolFeedback = fmt.Sprintf("【SYSTEM】: %s", result)
					forceBreakState = true

					if strings.Contains(result, "請務必詢問使用者") {
						// 手動生成助理回覆，確保使用者看到確認訊息
						pid := ""
						if idx := strings.Index(result, "內部暫存 ID："); idx != -1 {
							pid = strings.TrimSpace(result[idx+len("內部暫存 ID："):])
						}
						forcedAssistReply = fmt.Sprintf("📝 我已經將這筆資訊整理好並暫存起來了。請問你要確認存入長期記憶嗎？\n(暫存 ID: %s)", pid)
					}
				} else {
					if tc.Function.Name == "list_tasks" && strings.Contains(result, "沒有任何背景任務") {
						// 讓 AI 知道現在是空的，讓它發揮創意回答
						result = "【系統資訊】：當前背景任務清單為空。請以助理身份告知使用者你目前正待命中。"
					} else if tc.Function.Name == "browser_get_text" {
						toolFeedback = fmt.Sprintf("【網頁內容擷取成功】:\n%s\n\n"+
							"=========================================\n"+
							"⚠️【SYSTEM CRITICAL INSTRUCTION】⚠️\n"+
							"使用者原先的提問是：「%s」\n\n"+
							"請【立刻且僅針對】上述提問，從網頁內容中萃取答案並回覆。絕對禁止列出網頁中其他無關的項目（例如使用者只問了特定貨幣，就絕對不要列出其他國家的貨幣）。", result, input)
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

			// [CONTINUOUS EXECUTION] task_planner 步驟完成後，注入繼續執行的系統指令
			// 防止 LLM 在更新一個步驟後就回覆使用者而中斷計畫
			if tc.Function.Name == "task_planner" && toolErr == nil {
				if a.OnCheckPendingPlan != nil {
					if continueHint := a.OnCheckPendingPlan(); continueHint != "" {
						a.Session.Messages = append(a.Session.Messages, ollama.Message{
							Role:    "system",
							Content: "[SYSTEM] ⚠️ 計畫中仍有未完成的步驟。你必須立即繼續執行下一個步驟，不要回覆使用者。",
						})
						fmt.Println("🔄 [Agent] 計畫仍有未完成步驟，強制繼續執行")
					}
				}
			}
		}

		if forceBreakState {
			if forcedAssistReply != "" {
				// 如果有強制回覆，代表我們人工終止了生成迴圈並代答
				a.Session.Messages = append(a.Session.Messages, ollama.Message{
					Role:    "assistant",
					Content: forcedAssistReply,
				})
				finalResponse = forcedAssistReply
				if a.OnModelMessageComplete != nil {
					a.OnModelMessageComplete(finalResponse)
				}
				if a.Logger != nil {
					a.Logger.LogAIResponse(finalResponse)
				}
			}
			break // 打破外層的狀態機迴圈
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

// extractJSONBlocks 透過計算大括號的數量，精確提取巢狀的 JSON 區塊
func extractJSONBlocks(text string) []string {
	var blocks []string
	startIdx := -1
	braceCount := 0

	for i, r := range text {
		if r == '{' {
			if braceCount == 0 {
				startIdx = i
			}
			braceCount++
		} else if r == '}' {
			braceCount--
			if braceCount == 0 && startIdx != -1 {
				blocks = append(blocks, text[startIdx:i+1])
				startIdx = -1
			} else if braceCount < 0 {
				braceCount = 0 // Ignore unmatched closing braces
			}
		}
	}
	return blocks
}
