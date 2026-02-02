package tools

import (
	"fmt"
	"runtime"

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
			Description: "列出目前所有背景任務。當使用者詢問『你在做什麼？』、『目前進度』、『任務狀態』或『有什麼在執行嗎？』時，務必呼叫此工具。",
		},
	}
}

func (t *ListTasksTool) Run(argsJSON string) (string, error) {
	if t.Mgr == nil {
		return "錯誤：背景管理器未初始化。", nil
	}
	// 獲取原本的任務列表
	taskList := t.Mgr.GetTaskList()
	// 獲取關鍵系統指標 (假設這些函式定義在 tools 或透過 config 取得)
	// 這裡直接調用我們之前在 health.go 寫過的邏輯
	// 跨平台獲取根目錄資訊
	rootPath := "/"
	if runtime.GOOS == "windows" {
		rootPath = "C:"
	}
	diskInfo := GetDiskUsageString(rootPath) // 需確保此函式在 tools 內可被呼叫
	// 3. 整合資訊回傳給 AI
	// 我們用系統標籤包裝，讓 AI 明白這是當前的環境脈絡
	enhancedResult := fmt.Sprintf(
		"%s\n\n"+
			"【當前系統狀態脈絡】:\n"+
			"- 磁碟空間: %s\n"+
			"- 備註: 若任務正在運行且磁碟空間不足，請提醒使用者。",
		taskList, diskInfo,
	)
	return enhancedResult, nil
}
