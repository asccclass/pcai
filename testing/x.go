package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Agent 結構定義
type ReActAgent struct {
	ModelName      string
	SystemPrompt   string
	History        []string
	MaxHistory     int
	ErrorCount     int
	MaxError       int
	OllamaEndpoint string
	HttpClient     *http.Client
}

// NewAgent 初始化物件並讀取環境變數
func NewAgent(systemPrompt string) *ReActAgent {
	// 從環境變數讀取配置，若無則使用預設值
	model := os.Getenv("MODEL_NAME")
	if model == "" {
		model = "llama3.3" // 預設模型
	}

	endpoint := os.Getenv("OLLAMA_URL")
	if endpoint == "" {
		endpoint = "http://172.18.124.210:11434/api/generate"
	}

	return &ReActAgent{
		ModelName:      model,
		SystemPrompt:   systemPrompt,
		History:        []string{},
		MaxHistory:     3,
		MaxError:       3,
		OllamaEndpoint: endpoint,
		HttpClient:     &http.Client{Timeout: 60 * time.Second},
	}
}

// CallLLM 負責與 Ollama 通訊 (含重試邏輯)
func (a *ReActAgent) CallLLM(prompt string) (string, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		payload := map[string]interface{}{
			"model":  a.ModelName,
			"prompt": prompt,
			"stream": false,
		}
		jsonData, _ := json.Marshal(payload)

		resp, err := a.HttpClient.Post(a.OllamaEndpoint, "application/json", bytes.NewBuffer(jsonData))
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var res struct {
				Response string `json:"response"`
			}
			json.Unmarshal(body, &res)
			return res.Response, nil
		}
		lastErr = err
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	return "", lastErr
}

// Run 啟動 ReAct 循環
func (a *ReActAgent) Run(userQuery string) string {
	a.History = append(a.History, "User: "+userQuery)
	a.ErrorCount = 0 // 新任務重置錯誤計數

	for round := 1; round <= 10; round++ {
		// 1. 錯誤防禦
		if a.ErrorCount >= a.MaxError {
			return "抱歉，我嘗試多次修正工具參數但仍然失敗，請檢查系統後台。"
		}

		// 2. 準備 Prompt (滑動窗口)
		start := 0
		if len(a.History) > a.MaxHistory*2 {
			start = len(a.History) - (a.MaxHistory * 2)
		}
		fullPrompt := a.SystemPrompt + "\n" + strings.Join(a.History[start:], "\n")

		// 3. LLM 推理
		output, err := a.CallLLM(fullPrompt)
		if err != nil {
			return fmt.Sprintf("系統錯誤: 無法呼叫模型 %s", a.ModelName)
		}
		fmt.Printf("[%s] Round %d 推理中...\n", a.ModelName, round)
		a.History = append(a.History, output)

		if strings.Contains(output, "Final Answer:") {
			return output
		}

		// 4. 解析 Action 並並行執行
		re := regexp.MustCompile(`Action: (\w+)\[(.*?)\]`)
		matches := re.FindAllStringSubmatch(output, -1)

		if len(matches) > 0 {
			obs := a.executeParallel(matches)
			a.History = append(a.History, obs)
			fmt.Println(obs)
		} else {
			a.ErrorCount++
			a.History = append(a.History, "Observation: [ERROR] 請按格式輸出 Action 或 Final Answer。")
		}
	}
	return "達到最大推理輪次，任務未完成。"
}

// 內部方法：並行執行工具
func (a *ReActAgent) executeParallel(matches [][]string) string {
	var wg sync.WaitGroup
	results := make(chan string, len(matches))
	hasErr := false

	for _, m := range matches {
		wg.Add(1)
		go func(name, param string) {
			defer wg.Done()
			// 這裡之後可以串接真正的工具 Map
			if name == "GetStatus" {
				results <- "Observation (GetStatus): 系統運作正常"
			} else {
				hasErr = true
				results <- fmt.Sprintf("Observation (%s): [ERROR] 未知工具", name)
			}
		}(m[1], m[2])
	}

	wg.Wait()
	close(results)

	if hasErr {
		a.ErrorCount++
	} else {
		a.ErrorCount = 0
	}

	var sb strings.Builder
	for r := range results {
		sb.WriteString("\n" + r)
	}
	return sb.String()
}

func main() {
	// 使用方式
	sysPrompt := "你是一個系統管理 Agent。格式：Thought, Action, Observation, Final Answer。可用工具：GetStatus[]"

	agent := NewAgent(sysPrompt)

	fmt.Printf("--- 正在使用模型: %s ---\n", agent.ModelName)
	result := agent.Run("幫我檢查系統狀態。")
	fmt.Println("\n結果：", result)
}
