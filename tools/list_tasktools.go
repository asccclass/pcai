package tools

import (
	"fmt"
	"runtime"

	"github.com/asccclass/pcai/internal/scheduler"
	"github.com/ollama/ollama/api"
)

type ListTasksTool struct {
	Mgr      *BackgroundManager
	SchedMgr *scheduler.Manager
}

func (t *ListTasksTool) Name() string { return "list_tasks" }

func (t *ListTasksTool) IsSkill() bool {
	return false
}

func (t *ListTasksTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "list_tasks",
			Description: "列出目前所有背景任務與排程工作 (Cron Jobs)。當使用者詢問『你在做什麼？』、『目前進度』、『任務狀態』或『有什麼在執行嗎？』時，務必呼叫此工具。",
		},
	}
}

func (t *ListTasksTool) Run(argsJSON string) (string, error) {
	if t.Mgr == nil {
		return "錯誤：背景管理器未初始化。", nil
	}
	// 1. 獲取原本的背景任務列表
	taskList := t.Mgr.GetTaskList()

	// 2. 獲取 Cron 排程任務列表
	var cronInfo string
	if t.SchedMgr != nil {
		jobs := t.SchedMgr.ListJobs()
		if len(jobs) == 0 {
			cronInfo = "目前無任何排程工作 (Cron Jobs)。"
		} else {
			cronInfo = "【排程工作 (Cron Jobs)】:\n"
			for name, job := range jobs {
				desc := job.Description
				if desc == "" {
					desc = "(無說明)"
				}
				cronInfo += fmt.Sprintf("- PERMANENT [%s] %s : %s\n  Spec: %s\n", name, job.TaskName, desc, job.CronSpec)
			}
		}
	} else {
		cronInfo = "排程管理器 (Scheduler) 未連接。"
	}

	// 3. 獲取關鍵系統指標
	rootPath := "/"
	if runtime.GOOS == "windows" {
		rootPath = "C:"
	}
	diskInfo := GetDiskUsageString(rootPath)

	// 4. 整合資訊回傳給 AI
	enhancedResult := fmt.Sprintf(
		"%s\n\n"+
			"%s\n\n"+
			"【當前系統狀態脈絡】:\n"+
			"- 磁碟空間: %s\n"+
			"- 備註: 若任務正在運行且磁碟空間不足，請提醒使用者。",
		taskList, cronInfo, diskInfo,
	)
	return enhancedResult, nil
}
