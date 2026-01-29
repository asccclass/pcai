package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ollama/ollama/api"
)

// ListFilesTool 結構體
type ListFilesTool struct{}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Definition() api.Tool {
	// 使用我們之前學到的 JSON 轉換技巧，確保型別安全
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "list_files",
			"description": "List files in a specific directory.",
			"parameters": {
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "The folder path (e.g. 'Downloads')." }
				},
				"required": ["path"]
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *ListFilesTool) Run(argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}

	// 模擬簡單邏輯
	targetPath := args.Path
	if targetPath == "Downloads" {
		home, _ := os.UserHomeDir()
		targetPath = filepath.Join(home, targetPath)
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	// 簡化回傳
	return fmt.Sprintf("Found %d files in %s", len(entries), targetPath), nil
}
