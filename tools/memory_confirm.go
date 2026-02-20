package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// MemoryGetTool 讀取記憶檔案內容（取代舊的 MemoryConfirmTool）
type MemoryGetTool struct {
	toolkit *memory.ToolKit
}

// NewMemoryGetTool 建立記憶讀取工具
func NewMemoryGetTool(tk *memory.ToolKit) *MemoryGetTool {
	return &MemoryGetTool{toolkit: tk}
}

func (t *MemoryGetTool) Name() string {
	return "memory_get"
}

func (t *MemoryGetTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_get",
			Description: "讀取記憶檔案的指定內容。可以讀取長期記憶 (MEMORY.md) 或每日日誌 (memory/YYYY-MM-DD.md)。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"path": {
						"type": "string",
						"description": "要讀取的檔案相對路徑，例如 'MEMORY.md' 或 'memory/2026-02-18.md'。不填則讀取長期記憶。"
					},
					"start_line": {
						"type": "integer",
						"description": "起始行號 (1-indexed)，預設為 1"
					},
					"num_lines": {
						"type": "integer",
						"description": "讀取行數，預設為全部"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{},
				}
			}(),
		},
	}
}

func (t *MemoryGetTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		NumLines  int    `json:"num_lines"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	if args.Path == "" {
		args.Path = "MEMORY.md"
	}

	content, err := t.toolkit.MemoryGet(args.Path, args.StartLine, args.NumLines)
	if err != nil {
		return fmt.Sprintf("讀取失敗: %v", err), nil
	}

	if content == "" {
		return "檔案為空或不存在。", nil
	}

	return fmt.Sprintf("📄 %s 內容:\n%s", args.Path, content), nil
}
