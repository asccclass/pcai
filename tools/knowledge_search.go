package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
)

type KnowledgeSearchTool struct{}

func (t *KnowledgeSearchTool) Name() string { return "knowledge_search" }

func (t *KnowledgeSearchTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "knowledge_search",
			Description: "搜尋長期記憶知識庫 (knowledge.md) 中的內容。",
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
	json.Unmarshal([]byte(argsJSON), &args)

	home, _ := os.Executable()
	path := filepath.Join(home, "botmemory", "knowledge", "knowledge.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return "目前沒有長期記憶紀錄。", nil
	}

	lines := strings.Split(string(content), "\n")
	var results []string
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(args.Query)) {
			results = append(results, line)
		}
	}

	if len(results) == 0 {
		return "找不到相關內容。", nil
	}
	return "搜尋結果:\n" + strings.Join(results, "\n"), nil
}
