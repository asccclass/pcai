package agent

import (
	"fmt"
	"strings"
	"time"
)

// toolHintRule 定義關鍵字 → 工具提示的映射規則
type toolHintRule struct {
	Keywords []string                             // 使用者輸入中包含這些關鍵字時觸發
	ToolName string                               // 應該使用的工具名稱
	HintFunc func(input, pendingID string) string // 生成提示訊息的函式
}

// toolHintRules 定義所有工具路由提示規則
var toolHintRules = []toolHintRule{
	{
		Keywords: []string{"行事曆", "行程", "日程", "calendar", "schedule", "行程表"},
		ToolName: "read_calendars",
		HintFunc: func(input, pendingID string) string {
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
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求讀取或搜尋郵件。你必須呼叫 read_email 工具。
嚴禁使用 google_search、google_services、manage_cron_job 或編造 gog 指令。
呼叫工具的嚴格格式要求：
- 你必須使用標準 JSON 格式呼叫，範例：{"name": "read_email", "parameters": {"query": "is:inbox", "limit": "5"}}
- 工具必須包含 query (可用 is:inbox 或搜尋關鍵字) 與 limit (例如 "5" 或 "10")。`
		},
	},
	{
		Keywords: []string{"天氣", "氣象", "weather", "預報", "會冷", "會熱", "下雨", "溫度", "氣溫"},
		ToolName: "get_taiwan_weather",
		HintFunc: func(input, pendingID string) string {
			today := time.Now().Format("2006-01-02")
			return fmt.Sprintf(
				`[SYSTEM INSTRUCTION] 使用者詢問天氣。今天的日期是 %s。

判斷邏輯：
1. 若本訊息包含 [MEMORY CONTEXT] 且記憶中已有「溫度」、「降雨機率」等實際天氣預報數據，請直接引用該資料回答，不需呼叫工具。
2. 若記憶中沒有天氣預報數據，你必須呼叫 get_taiwan_weather 工具。

呼叫工具的嚴格格式要求：
- 你必須使用標準 JSON 格式呼叫，範例：{"name": "get_taiwan_weather", "parameters": {"location": "苗栗縣"}}
- 工具只接受一個參數 location，絕對不要傳 date 或其他參數（API 自動回傳未來預報）。
- location 的值必須從以下列表中精確複製一個：基隆市、臺北市、新北市、桃園市、新竹市、新竹縣、苗栗縣、臺中市、彰化縣、南投縣、雲林縣、嘉義市、嘉義縣、臺南市、高雄市、屏東縣、宜蘭縣、花蓮縣、臺東縣、澎湖縣、金門縣、連江縣。
- 嚴禁使用 [tool_name param=value] 或其他非 JSON 格式。
- 嚴禁使用 web_search、google_search。`,
				today,
			)
		},
	},
	{
		Keywords: []string{"立即執行", "執行簡報", "晨間簡報", "run briefing"},
		ToolName: "manage_cron_job",
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求立即執行排程任務。你必須呼叫 manage_cron_job 工具，參數為：{"action":"run_once","task_name":"daily_morning_briefing","task_type":"morning_briefing"}。嚴禁使用 task_planner 或其他不存在的工具。`
		},
	},
	{
		Keywords: []string{"瀏覽器", "網頁", "網址", "打開網址", "讀取網頁", "browser", "url"},
		ToolName: "browser_open",
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求使用瀏覽器讀取網頁內容或互動。
執行步驟：
1. 呼叫 {"name": "browser_open", "arguments": {"url": "https://..."}}。
2. 根據你的目的選擇下一步：
   - 若目的只是「讀取/尋找純粹的資訊」(如報價、匯率、文章)，請直接呼叫 {"name": "browser_get_text", "arguments": {}} 取得整個網頁的純文字內容。
   - 若你需要「點擊按鈕、輸入文字、導航」等互動，請呼叫 {"name": "browser_snapshot", "arguments": {"interactive_only": true}} 以取得可互動元素的參考 ID (ref)。
   
⚠️【最重要警告】：當你取得網頁內容後，請「精準回答使用者所詢問的對象」（例如使用者問南非幣，就只回答南非幣），絕對不要把網頁上所有不相關的項目（如其他國家的匯率或無關資訊）全部列出來。
⚠️【重要格式規範】：你必須使用標準 JSON 格式包裹參數，嚴禁單獨輸出純文字指令！`
		},
	},
	{
		Keywords: []string{"列出檔案", "目錄", "list files", "ls", "dir", "列出"},
		ToolName: "fs_list_dir",
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求列出檔案或目錄。你必須呼叫 fs_list_dir 工具（跨平台），參數為：{"path": "."}。嚴禁使用 shell_exec 搭配 ls 或 dir 指令。`
		},
	},
	{
		Keywords: []string{"記住", "記下來", "存入記憶", "幫我記", "remember", "memorize", "記錄", "存起來"},
		ToolName: "memory_save",
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求記住某些資訊。你必須呼叫 memory_save 工具，將資訊整理為簡潔的事實陳述句存入長期記憶。參數需包含 content（內容）、mode（設定為 "long_term"）和 category（分類：個人資訊、工作紀錄、偏好設定、生活雜記、技術開發）。若資訊包含多個主題，請拆成多次呼叫分別儲存。\n\n⚠️【重要格式規範】：你必須使用標準 JSON 格式包裹參數，嚴禁單獨輸出內部參數！格式必須如下：\n{"name": "memory_save", "arguments": {"content": "...", "mode": "long_term", "category": "..."}}`
		},
	},
	{
		Keywords: []string{"同意存入", "確認存入", "確認", "同意", "確認儲存", "同意記錄", "yes", "好", "可以", "沒問題", "ok", "OK"},
		ToolName: "memory_confirm",
		HintFunc: func(input, pendingID string) string {
			idStr := "pending_xxxx"
			if pendingID != "" {
				idStr = pendingID
			}
			return fmt.Sprintf(`[SYSTEM INSTRUCTION] 系統偵測到使用者同意某項操作或同意存入記憶。你必須立刻呼叫 memory_confirm 工具，並使用系統捕獲的暫存 ID: %s 。\n\n嚴禁填寫 "該ID" 或 "暫存ID" 等代替字，請精確按照以下格式輸出：\n{"name": "memory_confirm", "arguments": {"action": "confirm", "pending_id": "%s"}}。若找不到待確認的 ID，請以一般助理風格回覆。`, idStr, idStr)
		},
	},
	{
		Keywords: []string{"拒絕存入", "拒絕", "不要", "取消", "不用存", "刪掉"},
		ToolName: "memory_confirm",
		HintFunc: func(input, pendingID string) string {
			idStr := "pending_xxxx"
			if pendingID != "" {
				idStr = pendingID
			}
			return fmt.Sprintf(`[SYSTEM INSTRUCTION] 系統偵測到使用者拒絕某項操作或拒絕存入記憶。你必須立刻呼叫 memory_confirm 工具，取消暫存 ID: %s 。\n\n請按照以下格式輸出：\n{"name": "memory_confirm", "arguments": {"action": "reject", "pending_id": "%s"}}。`, idStr, idStr)
		},
	},
}

// getToolHint 檢查使用者輸入是否匹配任何工具提示規則
// 如果匹配，回傳提示訊息；否則回傳空字串
func getToolHint(input, pendingID string) string {
	lower := strings.ToLower(input)
	for _, rule := range toolHintRules {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return rule.HintFunc(input, pendingID)
			}
		}
	}
	return ""
}
