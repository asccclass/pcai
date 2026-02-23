package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/channel"
	"github.com/ollama/ollama/api"
)

// WhatsAppSendTool 專門用於主動發送 WhatsApp 訊息的工具
type WhatsAppSendTool struct {
	Channel *channel.WhatsAppChannel
}

// Name 回傳工具名稱
func (t *WhatsAppSendTool) Name() string {
	return "send_whatsapp"
}

func (t *WhatsAppSendTool) IsSkill() bool {
	return false
}

// Definition 回傳工具定義給 LLM
func (t *WhatsAppSendTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "send_whatsapp",
			"description": "主動發送 WhatsApp 訊息給指定號碼。請注意：如果對方未曾與此帳號互動過，可能會被視為 spam，建議先由對方發起對話。",
			"parameters": {
				"type": "object",
				"properties": {
					"phone_number": {
						"type": "string",
						"description": "接收者的電話號碼 (格式: 8869xxxxxxxx 或 +8869xxxxxxxx)，需包含國碼"
					},
					"message": {
						"type": "string",
						"description": "要發送的訊息內容"
					}
				},
				"required": ["phone_number", "message"]
			}
		}
	}`
	if err := json.Unmarshal([]byte(jsonStr), &tool); err != nil {
		fmt.Printf("⚠️ [WhatsAppSendTool] Definition JSON error: %v\n", err)
	}
	return tool
}

// Run 執行工具
func (t *WhatsAppSendTool) Run(argsJSON string) (string, error) {
	if t.Channel == nil {
		return "", fmt.Errorf("WhatsApp 頻道未啟用或未連接")
	}

	var args struct {
		PhoneNumber string `json:"phone_number"`
		Message     string `json:"message"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 處理電話號碼格式
	phone := strings.TrimSpace(args.PhoneNumber)
	phone = strings.TrimPrefix(phone, "+")

	// 簡單驗證 (這只是基本檢查，實際由 Channel 的 ParseJID 處理)
	if phone == "" {
		return "", fmt.Errorf("電話號碼不能為空")
	}

	// 呼叫 Channel 發送
	// 注意：SendWhatsApp 需要 JID 格式，通常是 "國碼手機號@s.whatsapp.net"
	// Channel 的 SendMessage 已經有簡單的處理邏輯 (如果是純數字會嘗試 append server)
	// 但為了保險，我們這裡不需自己組字串，讓 Channel 做

	// 使用 Channel 的 SendMessage
	// 這裡我們假設 Channel.SendMessage 接受的 ID 可以是純號碼
	// 根據之前的 code: internal/channel/whatsapp.go:128 -> types.NewJID(chatID, types.DefaultUserServer)
	// 所以只要傳入號碼即可 (例如 886912345678)

	err := t.Channel.SendMessage(phone, args.Message)
	if err != nil {
		return "", fmt.Errorf("發送失敗: %v", err)
	}

	return fmt.Sprintf("✅ 訊息已發送至 %s", phone), nil
}
