package copilot

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/asccclass/pcai/llms/ollama"
	"github.com/ollama/ollama/api"
)

// ──────────────────────────────────────────────────────
// 常數定義
// ──────────────────────────────────────────────────────

const (
	// GitHub Copilot 官方 VS Code OAuth App 的 Client ID
	copilotClientID = "Iv1.b507a08c87ecfe98"

	// GitHub OAuth 端點
	deviceCodeURL   = "https://github.com/login/device/code"
	accessTokenURL  = "https://github.com/login/oauth/access_token"
	verificationURL = "https://github.com/login/device" // 使用者在瀏覽器中填入 code 的 URL

	// Copilot Token 端點
	copilotTokenURL = "https://api.github.com/copilot_internal/v2/token"

	// Copilot Chat 端點
	copilotChatURL = "https://api.githubcopilot.com/chat/completions"

	// Token 存檔路徑 (相對於 CWD)
	tokenFileName = "copilot_token.json"
)

// ──────────────────────────────────────────────────────
// Token 資料結構
// ──────────────────────────────────────────────────────

// savedToken 持久化到檔案的 GitHub OAuth Token
type savedToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// copilotSession 從 Copilot API 取得的短期 Session Token
type copilotSession struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

var (
	cachedSession *copilotSession
	sessionMutex  sync.Mutex
)

// ──────────────────────────────────────────────────────
// GitHub OAuth Device Flow
// ──────────────────────────────────────────────────────

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
}

// Login 執行 GitHub OAuth Device Flow 登入
// 回傳 GitHub Access Token，並持久化到 copilot_token.json
func Login() (string, error) {
	// Step 1: 申請 Device Code
	body := fmt.Sprintf("client_id=%s&scope=copilot", copilotClientID)
	req, _ := http.NewRequest("POST", deviceCodeURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("device code 請求失敗: %v", err)
	}
	defer resp.Body.Close()

	var dcResp deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcResp); err != nil {
		return "", fmt.Errorf("device code 解析失敗: %v", err)
	}

	// Step 2: 提示使用者
	fmt.Println("╔═══════════════════════════════════════════════════╗")
	fmt.Println("║       GitHub Copilot 登入                         ║")
	fmt.Println("╠═══════════════════════════════════════════════════╣")
	fmt.Printf("║  請在瀏覽器開啟: %-33s║\n", dcResp.VerificationURI)
	fmt.Printf("║  輸入驗證碼:     %-33s║\n", dcResp.UserCode)
	fmt.Println("╚═══════════════════════════════════════════════════╝")
	fmt.Println("⏳ 等待授權中...")

	// Step 3: 輪詢 Access Token
	interval := dcResp.Interval
	if interval < 5 {
		interval = 5
	}

	deadline := time.Now().Add(time.Duration(dcResp.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Duration(interval) * time.Second)

		tokenBody := fmt.Sprintf("client_id=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code",
			copilotClientID, dcResp.DeviceCode)
		tokenReq, _ := http.NewRequest("POST", accessTokenURL, strings.NewReader(tokenBody))
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		tokenReq.Header.Set("Accept", "application/json")

		tokenResp, err := http.DefaultClient.Do(tokenReq)
		if err != nil {
			continue
		}

		var atResp accessTokenResponse
		json.NewDecoder(tokenResp.Body).Decode(&atResp)
		tokenResp.Body.Close()

		switch atResp.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5
			continue
		case "":
			// 成功！
			if atResp.AccessToken != "" {
				// 持久化
				if err := saveToken(&savedToken{
					AccessToken: atResp.AccessToken,
					TokenType:   atResp.TokenType,
				}); err != nil {
					fmt.Printf("⚠️ Token 存檔失敗: %v\n", err)
				}
				fmt.Println("✅ GitHub Copilot 登入成功！Token 已儲存。")
				return atResp.AccessToken, nil
			}
		default:
			return "", fmt.Errorf("OAuth 錯誤: %s", atResp.Error)
		}
	}

	return "", fmt.Errorf("登入逾時，請重試")
}

// ──────────────────────────────────────────────────────
// Token 持久化
// ──────────────────────────────────────────────────────

func tokenFilePath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, tokenFileName)
}

func saveToken(t *savedToken) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenFilePath(), data, 0600)
}

func loadToken() (*savedToken, error) {
	data, err := os.ReadFile(tokenFilePath())
	if err != nil {
		return nil, err
	}
	var t savedToken
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetGitHubToken 取得 GitHub Token（優先從檔案讀取，其次從環境變數）
func GetGitHubToken() (string, error) {
	// 1. 從持久化檔案讀取
	if t, err := loadToken(); err == nil && t.AccessToken != "" {
		return t.AccessToken, nil
	}

	// 2. 從環境變數讀取
	for _, envKey := range []string{"GITHUB_TOKEN", "GH_TOKEN", "COPILOT_GITHUB_TOKEN"} {
		if token := os.Getenv(envKey); token != "" {
			return token, nil
		}
	}

	return "", fmt.Errorf("未找到 GitHub Token。請先執行 `pcai copilot-login` 或設定 GITHUB_TOKEN 環境變數")
}

// ──────────────────────────────────────────────────────
// Copilot Session Token (短期)
// ──────────────────────────────────────────────────────

func getCopilotSessionToken(githubToken string) (string, error) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	// 檢查快取是否仍然有效（提前 60 秒過期）
	if cachedSession != nil && time.Now().Unix() < cachedSession.ExpiresAt-60 {
		return cachedSession.Token, nil
	}

	req, _ := http.NewRequest("GET", copilotTokenURL, nil)
	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	// 必須模擬 VS Code Copilot 擴充的 User-Agent，否則會被拒絕 (HTTP 403)
	req.Header.Set("User-Agent", "GitHubCopilotChat/0.24.0")
	req.Header.Set("Editor-Version", "vscode/1.96.0")
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.24.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Copilot token 請求失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Copilot token 回傳錯誤 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var session copilotSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", fmt.Errorf("Copilot token 解析失敗: %v", err)
	}

	cachedSession = &session
	return session.Token, nil
}

// ──────────────────────────────────────────────────────
// OpenAI-Compatible 訊息格式
// ──────────────────────────────────────────────────────

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

// SSE 回應格式
type sseChunk struct {
	Choices []struct {
		Delta struct {
			Role      string           `json:"role,omitempty"`
			Content   string           `json:"content,omitempty"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// ──────────────────────────────────────────────────────
// 格式轉換（Ollama ⇄ OpenAI）
// ──────────────────────────────────────────────────────

func convertMessages(msgs []ollama.Message) []openAIMessage {
	var out []openAIMessage

	// 追蹤 assistant 產生的 tool_call IDs，以便後續 tool 回應能對應
	var pendingToolCallIDs []string

	for _, m := range msgs {
		oaiMsg := openAIMessage{
			Role:    m.Role,
			Content: m.Content,
		}

		// 轉換 assistant 的 tool calls（並記錄 ID）
		if len(m.ToolCalls) > 0 {
			pendingToolCallIDs = nil // 重置
			for i, tc := range m.ToolCalls {
				callID := fmt.Sprintf("call_%d", i)
				pendingToolCallIDs = append(pendingToolCallIDs, callID)
				argsBytes, _ := json.Marshal(tc.Function.Arguments)
				oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openAIToolCall{
					ID:   callID,
					Type: "function",
					Function: openAIFunctionCall{
						Name:      tc.Function.Name,
						Arguments: string(argsBytes),
					},
				})
			}
			// assistant message with tool_calls 不應有 content (OpenAI 規定)
			oaiMsg.Content = ""
		}

		// 為 tool 回應訊息補上 tool_call_id
		if m.Role == "tool" {
			if len(pendingToolCallIDs) > 0 {
				oaiMsg.ToolCallID = pendingToolCallIDs[0]
				pendingToolCallIDs = pendingToolCallIDs[1:]
			} else {
				// Fallback: 沒有對應的 ID 時產生一個
				oaiMsg.ToolCallID = fmt.Sprintf("call_fallback_%d", len(out))
			}
		}

		out = append(out, oaiMsg)
	}
	return out
}

func convertTools(tools []api.Tool) []openAITool {
	var out []openAITool
	for _, t := range tools {
		// OpenAI/Copilot API 要求 parameters 必須是完整的 JSON Schema object
		// Ollama 工具定義中某些欄位可能為 nil，需要清理
		params := sanitizeParams(t.Function.Parameters)

		out = append(out, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

// sanitizeParams 將參數 schema 序列化為 JSON 後清理所有 null/空值
// 確保符合 OpenAI/Copilot API 的 JSON Schema 要求
func sanitizeParams(params interface{}) interface{} {
	// 先序列化為 JSON
	data, err := json.Marshal(params)
	if err != nil || string(data) == "null" || string(data) == "{}" {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	// 反序列化為泛用 map 以便清理
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	// 確保頂層必要欄位
	typeVal, _ := m["type"].(string)
	if typeVal == "" {
		m["type"] = "object"
	}
	if m["properties"] == nil {
		m["properties"] = map[string]interface{}{}
	}

	// 遞迴清理每個 property 內部的 schema
	if props, ok := m["properties"].(map[string]interface{}); ok {
		for k, v := range props {
			if propMap, ok := v.(map[string]interface{}); ok {
				// 確保每個 property 有 type
				if pt, _ := propMap["type"].(string); pt == "" {
					propMap["type"] = "string"
				}
				props[k] = propMap
			}
		}
		m["properties"] = props
	}

	return m
}

// ──────────────────────────────────────────────────────
// ChatStream — 核心串流聊天函式
// ──────────────────────────────────────────────────────

// ChatStream 實作 llms.ChatStreamFunc 介面
// 透過 GitHub Copilot API 進行串流聊天
func ChatStream(modelName string, messages []ollama.Message, tools []api.Tool, opts ollama.Options, callback func(string)) (ollama.Message, error) {
	// 1. 取得 GitHub Token
	githubToken, err := GetGitHubToken()
	if err != nil {
		return ollama.Message{}, err
	}

	// 2. 兌換 Copilot Session Token
	sessionToken, err := getCopilotSessionToken(githubToken)
	if err != nil {
		return ollama.Message{}, fmt.Errorf("Copilot session token 取得失敗: %v", err)
	}

	// 3. 建構請求
	chatReq := chatRequest{
		Model:    modelName,
		Messages: convertMessages(messages),
		Stream:   true,
	}

	if len(tools) > 0 {
		chatReq.Tools = convertTools(tools)
	}

	jsonData, err := json.Marshal(chatReq)
	if err != nil {
		return ollama.Message{}, fmt.Errorf("JSON 序列化失敗: %v", err)
	}

	// 4. 發送請求
	req, _ := http.NewRequest("POST", copilotChatURL, bytes.NewBuffer(jsonData))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "GitHubCopilotChat/0.24.0")
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")
	req.Header.Set("Editor-Version", "vscode/1.96.0")
	req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.24.0")
	req.Header.Set("Openai-Intent", "conversation-panel")

	client := &http.Client{Timeout: 120 * time.Second}

	var resp *http.Response
	maxRetries := 2
	for i := 0; i <= maxRetries; i++ {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		if i < maxRetries {
			fmt.Printf("⚠️ Copilot 連線失敗 (嘗試 %d/%d): %v\n", i+1, maxRetries+1, err)
			time.Sleep(2 * time.Second)
			// 重新建構 request body (因為 Body 已被讀取)
			req, _ = http.NewRequest("POST", copilotChatURL, bytes.NewBuffer(jsonData))
			req.Header.Set("Authorization", "Bearer "+sessionToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Copilot-Integration-Id", "vscode-chat")
			req.Header.Set("Editor-Version", "vscode/1.96.0")
			req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.24.0")
			req.Header.Set("Openai-Intent", "conversation-panel")
		}
	}
	if err != nil {
		return ollama.Message{}, fmt.Errorf("Copilot API 連線失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		// Token 過期，清除快取
		sessionMutex.Lock()
		cachedSession = nil
		sessionMutex.Unlock()
		return ollama.Message{}, fmt.Errorf("Copilot 認證失敗 (HTTP 401)。請執行 `pcai copilot-login` 重新登入")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return ollama.Message{}, fmt.Errorf("Copilot API 錯誤 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// 5. 解析 SSE 串流
	var fullMsg ollama.Message
	fullMsg.Role = "assistant"

	// 用來累積分段傳來的 tool_calls
	toolCallMap := make(map[int]*openAIToolCall)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE 格式: "data: {json}" 或 "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// 處理文字內容
		if delta.Content != "" {
			fullMsg.Content += delta.Content
			if callback != nil {
				callback(delta.Content)
			}
		}

		// 處理工具呼叫（可能分多個 chunk 傳來）
		for _, tc := range delta.ToolCalls {
			idx := 0 // 預設 index
			if tc.ID != "" {
				// 新的 tool call 開始
				if existing, ok := toolCallMap[idx]; ok {
					// 如果已經存在，找一個新的 index
					for i := 0; ; i++ {
						if _, ok := toolCallMap[i]; !ok {
							idx = i
							break
						}
						if toolCallMap[i].ID == tc.ID {
							idx = i
							existing = toolCallMap[i]
							_ = existing
							break
						}
					}
				}
				if _, ok := toolCallMap[idx]; !ok {
					toolCallMap[idx] = &openAIToolCall{
						ID:   tc.ID,
						Type: "function",
					}
				}
			}

			entry := toolCallMap[idx]
			if entry == nil {
				entry = &openAIToolCall{Type: "function"}
				toolCallMap[idx] = entry
			}

			if tc.Function.Name != "" {
				entry.Function.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				entry.Function.Arguments += tc.Function.Arguments
			}
		}
	}

	// 6. 將 OpenAI 格式的 tool_calls 轉回 Ollama 格式
	for i := 0; i < len(toolCallMap); i++ {
		tc, ok := toolCallMap[i]
		if !ok {
			continue
		}

		var args api.ToolCallFunctionArguments
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

		fullMsg.ToolCalls = append(fullMsg.ToolCalls, api.ToolCall{
			Function: api.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}

	return fullMsg, nil
}
