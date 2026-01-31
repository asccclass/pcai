package tools

import (
	"github.com/ollama/ollama/api"
)

type ListTasksTool struct {
	Mgr *BackgroundManager
}

func (t *ListTasksTool) Name() string { return "list_tasks" }

func (t *ListTasksTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "list_tasks",
			Description: "列出目前所有背景任務的執行狀態、ID 與結果。當使用者詢問進度或結果時使用。",
		},
	}
}

func (t *ListTasksTool) Run(argsJSON string) (string, error) {
	if t.Mgr == nil {
		return "錯誤：背景管理器未初始化。", nil
	}
	return t.Mgr.GetTaskList(), nil
}
