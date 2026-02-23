// 主動學習 (新增記憶工具) — 直接寫入 Markdown 檔案
package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

type MemorySaveTool struct {
	toolkit *memory.ToolKit
	pending *memory.PendingStore
}

func NewMemorySaveTool(tk *memory.ToolKit, ps *memory.PendingStore) *MemorySaveTool {
	return &MemorySaveTool{toolkit: tk, pending: ps}
}

func (t *MemorySaveTool) Name() string {
	return "memory_save"
}

func (t *MemorySaveTool) IsSkill() bool {
	return false
}

func (t *MemorySaveTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_save",
			Description: "用於儲存重要資訊。當使用者要求你「記住」某事，或提供了新的個人資訊、專案細節時，使用此工具。可選擇寫入今日日誌 (daily) 或長期記憶 (long_term)。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"content": {
						"type": "string",
						"description": "要儲存的詳細內容，請將其總結為清晰的陳述句。"
					},
					"mode": {
						"type": "string",
						"description": "儲存模式：'daily' (今日日誌，適合短期事件) 或 'long_term' (長期記憶，適合持久事實)。預設為 'long_term'。",
						"enum": ["daily", "long_term"]
					},
					"category": {
						"type": "string",
						"description": "記憶分類 (僅 long_term 模式使用)，例如 'preference', 'project', 'person', 'fact'"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"content"},
				}
			}(),
		},
	}
}

func (t *MemorySaveTool) Run(argsJSON string) (string, error) {
	var args struct {
		Content  string `json:"content"`
		Mode     string `json:"mode"`
		Category string `json:"category"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	if args.Content == "" {
		return "內容不能為空", nil
	}

	if args.Mode == "" {
		args.Mode = "long_term"
	}

	// 寫入 PendingStore
	pendingID := t.pending.Add(args.Content, args.Category, args.Mode)

	// 回傳對話提示告訴 AI
	return fmt.Sprintf("記憶已暫存。請務必詢問使用者：「我準備記住這筆資訊，要確認存入嗎？」\n內部暫存 ID：%s", pendingID), nil
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
