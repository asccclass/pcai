package tools

import (
	"encoding/json"
	"fmt"

	"github.com/asccclass/pcai/internal/scheduler"

	"github.com/ollama/ollama/api"
)

type SchedulerTool struct {
	Mgr *scheduler.Manager
}

// Name 滿足 AgentTool 介面
func (t *SchedulerTool) Name() string {
	return "manage_cron_job"
}

// Definition 滿足 AgentTool 介面，使用 ollama/api 結構
func (t *SchedulerTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: "設定定時背景任務。例如『每天早上8點讀取郵件』。時間需轉換為 Cron 格式 (分 時 日 月 週)。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"cron_expression": {
						"type":        "string",
						"description": "Cron 格式字串，例如 '0 8 * * *' 代表每天早上 8 點"
					},
					"task_type": {
						"type":        "string",
						"description": "執行的任務類型",
						"enum":        ["read_email"]
					},
					"task_name": {
						"type":        "string",
						"description": "任務的簡短名稱，如 'morning_check'"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"cron_expression", "task_type"},
				}
			}(),
		},
	}
}

// Run 滿足 AgentTool 介面，解析 Ollama 傳入的 JSON 參數
func (t *SchedulerTool) Run(argsJSON string) (string, error) {
	var args struct {
		CronExpr string `json:"cron_expression"`
		TaskType string `json:"task_type"`
		TaskName string `json:"task_name"`
	}

	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %v", err)
	}
	// 強制映射：將 LLM 可能輸出的 check_email 轉為註冊好的 read_gmail
	actualTaskType := args.TaskType
	if actualTaskType == "check_email" {
		actualTaskType = "read_email"
	}

	if args.TaskName == "" {
		args.TaskName = fmt.Sprintf("auto_job_%s", actualTaskType)
	}

	err := t.Mgr.AddJob(args.TaskName, args.CronExpr, actualTaskType, "Created via Tool")
	if err != nil {
		fmt.Printf("Job Failed: %s\n", err)
		return "", fmt.Errorf("scheduler error: %v", err)
	}

	return fmt.Sprintf("【SYSTEM】背景啟動, ID: %s, Cron: %s. Please inform the user that their request has been set up and will run automatically.", args.TaskName, args.CronExpr), nil
}
