package agent

import (
	"fmt"
	"strings"
	"time"
)

// toolHintRule 定義關鍵字 → 工具提示的映射規則
type toolHintRule struct {
	Keywords []string                  // 使用者輸入中包含這些關鍵字時觸發
	ToolName string                    // 應該使用的工具名稱
	HintFunc func(input string) string // 生成提示訊息的函式
}

// toolHintRules 定義所有工具路由提示規則
var toolHintRules = []toolHintRule{
	{
		Keywords: []string{"行事曆", "行程", "日程", "calendar", "schedule", "行程表"},
		ToolName: "read_calendars",
		HintFunc: func(input string) string {
			today := time.Now().Format("2006-01-02")
			return fmt.Sprintf(
				"[SYSTEM INSTRUCTION] 使用者要求查看行事曆。你必須呼叫 read_calendars 工具，並提供 from 和 to 參數（格式: YYYY-MM-DD）。今天的日期是 %s。如果使用者說「今天」，from 和 to 都設為 %s。嚴禁使用 google_search、google_services 或 manage_cron_job。",
				today, today,
			)
		},
	},
	{
		Keywords: []string{"郵件", "信件", "信箱", "email", "mail", "gmail"},
		ToolName: "read_email",
		HintFunc: func(input string) string {
			return "[SYSTEM INSTRUCTION] 使用者要求讀取郵件。你必須呼叫 read_email 工具。嚴禁使用 google_search、google_services 或 manage_cron_job。"
		},
	},
	{
		Keywords: []string{"天氣", "氣象", "weather", "預報"},
		ToolName: "get_taiwan_weather",
		HintFunc: func(input string) string {
			return "[SYSTEM INSTRUCTION] 使用者詢問天氣。若地點在台灣 (例如台北、高雄、台中...)，**必須**優先呼叫 `get_taiwan_weather` 技能，而非 `web_search` 或 `google_search`。"
		},
	},
	{
		Keywords: []string{"立即執行", "執行簡報", "晨間簡報", "run briefing"},
		ToolName: "manage_cron_job",
		HintFunc: func(input string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求立即執行排程任務。你必須呼叫 manage_cron_job 工具，參數為：{"action":"run_once","task_name":"daily_morning_briefing","task_type":"morning_briefing"}。嚴禁使用 task_planner 或其他不存在的工具。`
		},
	},
	{
		Keywords: []string{"列出檔案", "目錄", "list files", "ls", "dir", "列出"},
		ToolName: "fs_list_dir",
		HintFunc: func(input string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求列出檔案或目錄。你必須呼叫 fs_list_dir 工具（跨平台），參數為：{"path": "."}。嚴禁使用 shell_exec 搭配 ls 或 dir 指令。`
		},
	},
	{
		Keywords: []string{"記住", "記下來", "存入記憶", "幫我記", "remember", "memorize", "記錄", "存起來"},
		ToolName: "knowledge_append",
		HintFunc: func(input string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求記住某些資訊。你必須呼叫 knowledge_append 工具，將資訊整理為簡潔的事實陳述句存入長期記憶。參數需包含 content（內容）和 category（分類：個人資訊、工作紀錄、偏好設定、生活雜記、技術開發）。若資訊包含多個主題，請拆成多次呼叫分別儲存。嚴禁使用 memory_save 或其他工具。`
		},
	},
}

// getToolHint 檢查使用者輸入是否匹配任何工具提示規則
// 如果匹配，回傳提示訊息；否則回傳空字串
func getToolHint(input string) string {
	lower := strings.ToLower(input)
	for _, rule := range toolHintRules {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return rule.HintFunc(input)
			}
		}
	}
	return ""
}
