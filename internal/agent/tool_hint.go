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
		Keywords: []string{"瀏覽器", "網頁", "網址", "打開網址", "讀取網頁", "browser", "url", "頁面", "這頁", "這個頁面", "http", "https"},
		ToolName: "browser_open",
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 使用者要求讀取網頁內容。
你必須執行以下步驟來獲取答案：
1. 呼叫 {"name": "browser_open", "arguments": {"url": "https://..."}}。
2. 待開啟成功後，**立即**呼叫 {"name": "browser_get_text", "arguments": {}} 取得內容。
⚠️【禁止停頓】：不要只停在開啟頁面，也不要問使用者後續操作。你必須取得文字內容後直接回答使用者的問題。`
		},
	},
	{
		Keywords: []string{"讀取內容", "抓取文字", "get text", "read content"},
		ToolName: "browser_get_text",
		HintFunc: func(input, pendingID string) string {
			return `[SYSTEM INSTRUCTION] 網頁已開啟。請立即呼叫 browser_get_text 獲取內容，並針對使用者問題從中萃取答案回覆。`
		},
	},
	{
		Keywords: []string{"列出檔案", "目錄", "list files", "ls", "dir", "列出", "列下", "有什麼檔案"},
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

// ─────────────────────────────────────────────────────────────
// 多步驟意圖偵測 (Multi-Step Intent Detection)
// ─────────────────────────────────────────────────────────────

// chainingKeywords 連動關鍵字：使用者明確表達多步驟意圖
var chainingKeywords = []string{
	"然後", "之後", "接著", "以及", "同時",
	"整理成", "彙整", "統整", "幫我", "再",
	"步驟", "依序", "順序", "先", "最後",
	"and then", "after that", "also", "next", "finally",
	"step by step", "followed by",
}

// detectMultiStepIntent 偵測使用者輸入是否包含需要多步驟執行的意圖
// 回傳 Planning Prompt 或空字串
func detectMultiStepIntent(input string) string {
	lower := strings.ToLower(input)

	// 策略 1: 檢查是否命中 ≥2 個不同的工具類別關鍵字
	matchedCategories := make(map[string]bool)
	for _, rule := range toolHintRules {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				matchedCategories[rule.ToolName] = true
				break // 一個類別只計算一次
			}
		}
	}

	isMultiStep := len(matchedCategories) >= 2

	// 策略 2: 檢查是否包含連動關鍵字
	if !isMultiStep {
		chainingCount := 0
		for _, kw := range chainingKeywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				chainingCount++
				if chainingCount >= 2 {
					isMultiStep = true
					break
				}
			}
		}
	}

	if !isMultiStep {
		return ""
	}

	// 構建 Planning Prompt
	today := time.Now().Format("2006-01-02 15:04")
	return fmt.Sprintf(`[SYSTEM INSTRUCTION — 複雜任務編排模式]
⚠️ 系統偵測到此請求涉及多個步驟或上下文連動的操作。

你必須遵循以下「先計畫再執行」流程：

1. **分析使用者意圖**：仔細閱讀使用者的完整請求，拆解為具體的執行步驟。
2. **建立計畫**：立即呼叫 task_planner 工具建立計畫：
   {"name": "task_planner", "arguments": {"action": "create", "goal": "使用者的總目標", "steps": "步驟1;步驟2;步驟3"}}
3. **依序執行**：按照計畫中的步驟，逐一執行對應的工具呼叫。每完成一步後，使用 task_planner(action="update") 更新步驟狀態。
4. **彙整結果**：所有步驟完成後，彙整各步驟結果，給出最終回答。最後呼叫 task_planner(action="finish") 結束計畫。

📅 當前時間: %s

注意事項：
- 每個步驟應該具體到對應的工具呼叫
- 若某步驟失敗，記錄失敗原因後繼續下一步
- 不要跳過計畫建立步驟，直接執行工具`, today)
}
