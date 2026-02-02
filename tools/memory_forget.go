package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"

	"github.com/ollama/ollama/api"
)

type MemoryForgetTool struct {
	manager *memory.Manager
}

func NewMemoryForgetTool(m *memory.Manager) *MemoryForgetTool {
	return &MemoryForgetTool{manager: m}
}

func (t *MemoryForgetTool) Name() string {
	return "memory_forget"
}

func (t *MemoryForgetTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_forget",
			Description: "用於刪除或「遺忘」記憶庫中的特定資訊。當使用者要求你忘記某事、修正錯誤資訊或刪除敏感數據時使用。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"content": {
						"type": "string",
						"description": "要刪除的記憶內容原文。必須儘可能精確匹配原始記憶。"
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

func (t *MemoryForgetTool) Run(argsJSON string) (string, error) {
	var args struct {
		Content string `json:"content"`
	}
	// 清洗 JSON
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	// 呼叫 Manager 執行刪除
	deleted, err := t.manager.DeleteByContent(args.Content)
	if err != nil {
		return "", fmt.Errorf("刪除過程發生錯誤: %w", err)
	}

	if !deleted {
		// 如果精確匹配失敗，這裡其實可以做進階處理（例如先 Search 再 Delete），
		// 但為了安全，我們先回報找不到。
		return fmt.Sprintf("找不到內容為「%s」的記憶，無法刪除。", args.Content), nil
	}

	return fmt.Sprintf("已成功將內容「%s」從記憶庫中移除。", args.Content), nil
}
