package tools

import (
	"encoding/json"
	"time"

	"github.com/ollama/ollama/api"
)

type TimeTool struct{}

func (t *TimeTool) Name() string { return "get_current_time" }

func (t *TimeTool) Definition() api.Tool {
	var tool api.Tool
	// 不需要參數的工具
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "get_current_time",
			"description": "Get the current system time.",
			"parameters": { "type": "object", "properties": {}, "required": [] }
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *TimeTool) Run(argsJSON string) (string, error) {
	return time.Now().Format("2006-01-02 15:04:05"), nil
}
