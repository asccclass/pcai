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
			Description: "設定或取消定時背景任務。例如『每天早上8點讀取郵件』或『取消讀取郵件的任務』。時間需轉換為 Cron 格式 (分 時 日 月 週)。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"action": {
						"type": "string",
						"description": "執行動作: 'add' (新增/更新), 'remove' (移除), 'run_once' (立即執行)",
						"enum": ["add", "remove", "run_once"]
					},
					"cron_expression": {
						"type": "string",
						"description": "Cron 格式字串，例如 '0 8 * * *' 代表每天早上 8 點。移除任務時可為空。"
					},
					"task_type": {
						"type": "string",
						"description": "執行的任務類型",
						"enum": ["read_email", "read_calendars"]
					},
					"task_name": {
						"type": "string",
						"description": "任務的簡短名稱 (ID)，如 'morning_check'。 run_once 必填。"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"task_type", "action"},
				}
			}(),
		},
	}
}

// Run 滿足 AgentTool 介面，解析 Ollama 傳入的 JSON 參數
func (t *SchedulerTool) Run(argsJSON string) (string, error) {
	// 定義一個寬鬆的結構來接收可能的巢狀物件
	var rawArgs struct {
		Action   interface{} `json:"action"`
		CronExpr interface{} `json:"cron_expression"`
		TaskType interface{} `json:"task_type"`
		TaskName interface{} `json:"task_name"`
	}

	if err := json.Unmarshal([]byte(argsJSON), &rawArgs); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %v", err)
	}

	// 輔助函式：從 interface{} 提取字串，支援 string 或 {"value": "..."}
	getString := func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		if m, ok := v.(map[string]interface{}); ok {
			if val, ok := m["value"].(string); ok {
				return val
			}
		}
		return ""
	}

	action := getString(rawArgs.Action)
	cronExpr := getString(rawArgs.CronExpr)
	taskType := getString(rawArgs.TaskType)
	taskName := getString(rawArgs.TaskName)

	// 強制映射：將 LLM 可能輸出的 check_email 轉為註冊好的 read_gmail
	if taskType == "check_email" {
		taskType = "read_email"
	}

	if taskName == "" {
		taskName = fmt.Sprintf("auto_job_%s", taskType)
	}

	// 預設 action 為 add
	if action == "" {
		action = "add"
	}

	if action == "remove" {
		err := t.Mgr.RemoveJob(taskName)
		if err != nil {
			return "", fmt.Errorf("failed to remove job: %v", err)
		}
		return fmt.Sprintf("【SYSTEM】已移除背景任務, ID: %s", taskName), nil
	}

	if action == "run_once" {
		err := t.Mgr.RunJobNow(taskName)
		if err != nil {
			return "", fmt.Errorf("failed to run job: %v", err)
		}
		return fmt.Sprintf("【SYSTEM】已觸發任務立即執行: %s", taskName), nil
	}

	// Add logic
	if cronExpr == "" {
		return "", fmt.Errorf("cron_expression is required for add action")
	}

	err := t.Mgr.AddJob(taskName, cronExpr, taskType, "Created via Tool")
	if err != nil {
		fmt.Printf("Job Failed: %s\n", err)
		return "", fmt.Errorf("scheduler error: %v", err)
	}

	return fmt.Sprintf("【SYSTEM】背景啟動, ID: %s, Cron: %s. Please inform the user that their request has been set up and will run automatically.", taskName, cronExpr), nil
}
