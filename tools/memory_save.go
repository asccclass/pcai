// 主動學習 (新增記憶工具)
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

type MemorySaveTool struct {
	manager      *memory.Manager
	markdownPath string // 原始檔案路徑，用於附加寫入
}

func NewMemorySaveTool(m *memory.Manager, mdPath string) *MemorySaveTool {
	return &MemorySaveTool{
		manager:      m,
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
			Description: "用於儲存重要資訊。當使用者要求你「記住」某事，或提供了新的個人資訊、專案細節時，使用此工具將其永久保存。",
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

	// 1. 更新執行中的記憶庫 (Vector Store)
	// 這會即時生效，AI 下一句話就能檢索到
	if err := t.manager.Add(args.Content, []string{"user_created"}); err != nil {
		return "", fmt.Errorf("寫入記憶庫失敗: %w", err)
	}

	// 2. (選用) 同步寫入 Markdown 檔案 (File-First)
	// 這樣下次重啟程式時，資料還會在
	if t.markdownPath != "" {
		f, err := os.OpenFile(t.markdownPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			// 寫入格式：空行 + 內容
			if _, err := f.WriteString("\n\n" + args.Content); err != nil {
				fmt.Printf("警告: 無法寫入 Markdown 檔案: %v\n", err)
			}
		}
	}

	return fmt.Sprintf("已成功記住: \"%s\"", args.Content), nil
}
