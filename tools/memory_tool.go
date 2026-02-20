package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// MemoryTool 記憶搜尋工具（使用 ToolKit 混合搜尋）
type MemoryTool struct {
	toolkit *memory.ToolKit
}

// NewMemoryTool 建立搜尋工具
func NewMemoryTool(tk *memory.ToolKit) *MemoryTool {
	return &MemoryTool{toolkit: tk}
}

func (t *MemoryTool) Name() string {
	return "memory_search"
}

func (t *MemoryTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "memory_search",
			Description: "用於檢索過去的對話記錄、專案知識或使用者偏好。當你不確定問題答案，或覺得以前曾經討論過時，請使用此工具。使用混合搜尋（BM25 + 向量）提供更精準的結果。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "搜尋關鍵字或問題，例如 '專案的 API Key 是多少' 或 '上次會議的結論'"
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

func (t *MemoryTool) Run(argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("解析工具參數失敗: %w (原始輸入: %s)", err, argsJSON)
	}
	if args.Query == "" {
		return "錯誤: 搜尋查詢 (query) 不能為空。", nil
	}

	ctx := context.Background()
	resp, err := t.toolkit.MemorySearch(ctx, args.Query)
	if err != nil {
		return "", fmt.Errorf("搜尋執行錯誤: %w", err)
	}

	if len(resp.Results) == 0 {
		return "記憶庫搜尋結果: 未找到相關資訊。", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 條相關記憶 (Backend: %s, Provider: %s):\n", len(resp.Results), resp.Backend, resp.Provider))
	for i, res := range resp.Results {
		sb.WriteString(fmt.Sprintf("--- 結果 %d (相關度: %.2f, 文字: %.2f, 向量: %.2f) ---\n",
			i+1, res.FinalScore, res.TextScore, res.VectorScore))
		sb.WriteString(fmt.Sprintf("來源: %s (L%d-%d)\n", res.Chunk.FilePath, res.Chunk.StartLine, res.Chunk.EndLine))
		sb.WriteString(res.Chunk.Content)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
