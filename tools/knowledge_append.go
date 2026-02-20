package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/memory"
	"github.com/ollama/ollama/api"
)

// KnowledgeAppendTool 向長期記憶中追加知識
type KnowledgeAppendTool struct {
	toolkit *memory.ToolKit
}

func NewKnowledgeAppendTool(tk *memory.ToolKit) *KnowledgeAppendTool {
	return &KnowledgeAppendTool{toolkit: tk}
}

func (t *KnowledgeAppendTool) Name() string { return "knowledge_append" }

func (t *KnowledgeAppendTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "knowledge_append",
			Description: "記錄使用者的個人事實、偏好或工作筆記。請根據內容自動選擇最合適的分類標籤。這將永久存入長期記憶庫。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"content": {
						"type": "string",
						"description": "要記錄的事實內容，請使用繁體中文，並以簡短事實的形式呈現。"
					},
					"category": {
						"type": "string",
						"description": "分類標籤。可選：個人資訊、工作紀錄、偏好設定、生活雜記、技術開發。"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"content", "category"},
				}
			}(),
		},
	}
}

func (t *KnowledgeAppendTool) Run(argsJSON string) (string, error) {
	var args struct {
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", err
	}

	if args.Content == "" {
		return "錯誤: 內容不能為空。", nil
	}

	cat := args.Category
	if cat == "" {
		cat = "general"
	}

	if err := t.toolkit.WriteLongTerm(cat, args.Content); err != nil {
		return "", fmt.Errorf("寫入記憶失敗: %w", err)
	}

	return fmt.Sprintf("✅ 已分類為 #%s 並存入長期記憶庫。", cat), nil
}
