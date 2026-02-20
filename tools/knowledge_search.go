package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// KnowledgeSearchTool 搜尋長期記憶知識庫（使用混合搜尋）
type KnowledgeSearchTool struct {
	toolkit *memory.ToolKit
}

func NewKnowledgeSearchTool(tk *memory.ToolKit) *KnowledgeSearchTool {
	return &KnowledgeSearchTool{toolkit: tk}
}

func (t *KnowledgeSearchTool) Name() string { return "knowledge_search" }

func (t *KnowledgeSearchTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "knowledge_search",
			Description: "搜尋長期記憶知識庫中的內容。使用混合搜尋（BM25 + 向量）。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "要搜尋的關鍵字"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"query"},
				}
			}(),
		},
	}
}

func (t *KnowledgeSearchTool) Run(argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}

	if args.Query == "" {
		return "錯誤: 搜尋關鍵字不能為空。", nil
	}

	ctx := context.Background()
	resp, err := t.toolkit.MemorySearch(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("搜尋失敗: %w", err)
	}

	if len(resp.Results) == 0 {
		return "目前沒有找到相關的長期記憶。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 個相關知識區塊：\n", len(resp.Results)))
	for i, res := range resp.Results {
		sb.WriteString(fmt.Sprintf("\n--- 區塊 %d (分數: %.2f) ---\n", i+1, res.FinalScore))
		sb.WriteString(res.Chunk.Content)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
