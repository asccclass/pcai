package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// MemoryForgetTool 永久刪除記憶
type MemoryForgetTool struct {
	toolkit *memory.ToolKit
}

func NewMemoryForgetTool(tk *memory.ToolKit) *MemoryForgetTool {
	return &MemoryForgetTool{toolkit: tk}
}

func (t *MemoryForgetTool) Name() string {
	return "memory_forget"
}

func (t *MemoryForgetTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_forget",
			Description: "用於永久刪除記憶。當使用者要求「忘記」、「刪除」某事時使用。會從 MEMORY.md 中移除匹配的段落。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"content": {
						"type": "string",
						"description": "要刪除的記憶內容關鍵字。會搜尋並移除包含此關鍵字的整個段落。"
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
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	if args.Content == "" {
		return "錯誤: 刪除關鍵字不能為空", nil
	}

	// 從 MEMORY.md 中搜尋並刪除包含關鍵字的段落
	mgr := t.toolkit.Manager()
	memoryPath := filepath.Join(mgr.Config().WorkspaceDir, "MEMORY.md")

	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return "記憶檔案不存在或無法讀取", nil
	}

	original := string(data)
	sections := strings.Split(original, "\n---\n")
	var kept []string
	removed := 0

	for _, section := range sections {
		if strings.Contains(strings.ToLower(section), strings.ToLower(args.Content)) {
			removed++
		} else {
			kept = append(kept, section)
		}
	}

	if removed == 0 {
		return fmt.Sprintf("未找到包含 \"%s\" 的記憶段落。", args.Content), nil
	}

	// 寫回
	newContent := strings.Join(kept, "\n---\n")
	if err := os.WriteFile(memoryPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("寫入失敗: %w", err)
	}

	return fmt.Sprintf("🗑️ 已刪除 %d 個包含 \"%s\" 的記憶段落。", removed, args.Content), nil
}
