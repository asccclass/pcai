package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ollama/ollama/api"
)

// BotInteractSkill 包裝 Bot 通訊邏輯
type BotInteractSkill struct{}

func NewBotInteractSkill() *BotInteractSkill {
	return &BotInteractSkill{}
}

// BotInteractTool 是暴露給 AI 使用的工具
type BotInteractTool struct {
	skill *BotInteractSkill
}

func (s *BotInteractSkill) CreateTool() *BotInteractTool {
	return &BotInteractTool{skill: s}
}

func (t *BotInteractTool) Name() string {
	return "bot_interact"
}

func (t *BotInteractTool) IsSkill() bool {
	return false
}

func (t *BotInteractTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "bot_interact",
			Description: "透過 HTTP API 與另一個 PCAI Bot 進行直接通訊。當你需要將任務交辦給遠端的 Bot，或需要與其他 Bot 協作時請使用此技能。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"bot_url": {
						"type": "string",
						"description": "目標 Bot 的 API 端點網址，例如 http://localhost:8081/api/chat"
					},
					"message": {
						"type": "string",
						"description": "要發送給目標 Bot 的具體訊息內容"
					},
					"sender_id": {
						"type": "string",
						"description": "發送者的識別名稱，留空預設為 anonymous_bot。可用來讓對方區分不同的發送者。"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"bot_url", "message"},
				}
			}(),
		},
	}
}

func (t *BotInteractTool) Run(argsJSON string) (string, error) {
	var args struct {
		BotURL   string `json:"bot_url"`
		Message  string `json:"message"`
		SenderID string `json:"sender_id"`
	}

	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	if args.BotURL == "" || args.Message == "" {
		return "", fmt.Errorf("bot_url and message are required")
	}

	senderID := "pcai_client"
	if args.SenderID != "" {
		senderID = args.SenderID
	}

	payload := map[string]string{
		"sender_id": senderID,
		"message":   args.Message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", args.BotURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to bot: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("對方伺服器回傳錯誤 (HTTP %d): %s", resp.StatusCode, string(body)), nil
	}

	var result struct {
		Success bool   `json:"success"`
		Reply   string `json:"reply"`
		Error   string `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Sprintf("寄送成功，但無法解析對方回傳格式: %s", string(body)), nil
	}

	if !result.Success {
		return fmt.Sprintf("對方處理失敗: %s", result.Error), nil
	}

	return fmt.Sprintf("✅ 訊息已發送給 %s。對方 Bot 回覆：\n%s", args.BotURL, result.Reply), nil
}
