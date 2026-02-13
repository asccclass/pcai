// 主動學習 (新增記憶工具) — 需要使用者確認後才寫入
package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

type MemorySaveTool struct {
	manager      *memory.Manager
	pending      *memory.PendingStore
	markdownPath string // 原始檔案路徑，用於附加寫入
}

func NewMemorySaveTool(m *memory.Manager, ps *memory.PendingStore, mdPath string) *MemorySaveTool {
	return &MemorySaveTool{
		manager:      m,
		pending:      ps,
		markdownPath: mdPath,
	}
}

func (t *MemorySaveTool) Name() string {
	return "memory_save"
}

func (t *MemorySaveTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_save",
			Description: "用於儲存重要資訊。當使用者要求你「記住」某事，或提供了新的個人資訊、專案細節時，使用此工具將其暫存。注意：記憶不會立即寫入，需要等使用者確認後才會永久保存。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"content": {
						"type": "string",
						"description": "要儲存的詳細內容，請將其總結為清晰的陳述句。"
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
		Content string `json:"content"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	if args.Content == "" {
		return "內容不能為空", nil
	}

	// 暫存到 PendingStore，等待使用者確認
	pendingID := t.pending.Add(args.Content, []string{"user_created"})

	// 回傳提示訊息，讓 AI 告知使用者需要確認
	preview := args.Content
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}

	return fmt.Sprintf(
		"📝 記憶已暫存，等待確認 (ID: %s)\n內容預覽: \"%s\"\n\n請詢問使用者是否確認儲存。使用者確認後，請呼叫 memory_confirm 工具執行 confirm 操作。",
		pendingID, preview,
	), nil
}
