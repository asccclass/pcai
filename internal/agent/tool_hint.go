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
