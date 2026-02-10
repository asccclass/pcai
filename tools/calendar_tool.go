package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"time"

	"github.com/asccclass/pcai/internal/calendar"
	"github.com/ollama/ollama/api"
)

// CalendarTool 讓 LLM 可以主動查詢行事曆
type CalendarTool struct{}

type CalendarToolArgs struct {
	CalendarID string `json:"calendar_id,omitempty"`
	ID         string `json:"id,omitempty"`        // Alias for LLM convenience
	CalIDs     string `json:"cal_ids,omitempty"`   // Alias for LLM hallucination
	Calendars  string `json:"calendars,omitempty"` // Alias for LLM hallucination (plural)
	MaxResults int64  `json:"max_results,omitempty"`
}

func (t *CalendarTool) Name() string { return "read_calendar" }

func (t *CalendarTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "read_calendar",
			Description: "讀取 Google Calendar 行事曆。⚠️ 重要：若使用者指定了特定的行事曆 ID (例如 email)，務必將其填入 'calendars' 參數。例如 '讀取 liuchengood@gmail.com' -> calendars='liuchengood@gmail.com'。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"calendars": {
						"type": "string",
						"description": "【必要參數】要讀取的行事曆 ID (例如 'justgps@gmail.com')。若要讀取多個，請用『逗號分隔的字串』(例如 'a@g.com,b@g.com')。⚠️ 嚴禁使用 JSON Array (如 ['a','b'])。"
					},
					"max_results": {
						"type": "integer",
						"description": "最大事件數量 (預設 10)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"calendars"},
				}
			}(),
		},
	}
}

func (t *CalendarTool) Run(args string) (string, error) {
	fmt.Printf("🐞 [DEBUG] CalendarTool.Run called with args: %s\n", args)
	var a CalendarToolArgs
	if args != "" {
		_ = json.Unmarshal([]byte(args), &a)
	}
	if a.MaxResults <= 0 {
		a.MaxResults = 10
	}

	// 預設抓取「今天一整天」的事件 (包含已過去的)
	// 取得當地時間的 00:00:00
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	timeMin := startOfDay.Format(time.RFC3339)

	// 處理多個 Calendar ID (以逗號分隔)
	calendarIDs := []string{"primary"}

	// 支援 id 參數 (因為 LLM 有時候會用 id 而不是 calendar_id)
	inputID := a.CalendarID
	if inputID == "" && a.ID != "" {
		inputID = a.ID
	}
	// 支援 cal_ids 參數 (因為 LLM 有時候會幻覺出這個參數，且可能是 JSON array string)
	if inputID == "" && a.CalIDs != "" {
		// 嘗試移除 [" 和 "] 等字元，簡單處理 JSON array string
		cleaned := strings.Trim(a.CalIDs, "[]\" ")
		// 手動處理常見的 unicode escape (簡單替換)
		if strings.Contains(cleaned, "\\u") {
			var decoded string
			if err := json.Unmarshal([]byte("\""+cleaned+"\""), &decoded); err == nil {
				cleaned = decoded
			}
		}
		inputID = cleaned
	}
	// 支援 calendars 參數 (因為 LLM 有時候會幻覺出這個參數)
	if inputID == "" && a.Calendars != "" {
		cleaned := strings.Trim(a.Calendars, "[]\" ")
		if strings.Contains(cleaned, "\\u") {
			var decoded string
			if err := json.Unmarshal([]byte("\""+cleaned+"\""), &decoded); err == nil {
				cleaned = decoded
			}
		}
		inputID = cleaned
	}

	if inputID != "" {
		calendarIDs = strings.Split(inputID, ",")
	}

	var allEvents []calendar.Event
	var errors []string

	for _, calID := range calendarIDs {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		events, err := calendar.FetchUpcomingEvents(calID, timeMin, a.MaxResults)
		if err != nil {
			fmt.Printf("❌ [CalendarTool] Error fetching %s: %v\n", calID, err)
			// Return detailed error to LLM as well so it can explain better
			errors = append(errors, fmt.Sprintf("行事曆 %s 讀取失敗 (API Error): %v", calID, err))
			continue
		}
		fmt.Printf("✅ [CalendarTool] Successfully fetched %d events from %s\n", len(events), calID)
		allEvents = append(allEvents, events...)
	}

	if len(allEvents) == 0 {
		if len(errors) > 0 {
			return fmt.Sprintf("讀取失敗:\n%s", strings.Join(errors, "\n")), nil
		}
		return "目前沒有即將到來的行事曆活動。", nil
	}

	var sb strings.Builder
	if len(errors) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️ 部分讀取失敗:\n%s\n\n", strings.Join(errors, "\n")))
	}

	sb.WriteString("📅 **近期行事曆活動**:\n\n")
	for _, e := range allEvents {
		// 簡單的時間格式化
		sb.WriteString(fmt.Sprintf("- **%s** | %s", e.Start, e.Summary))
		if e.Location != "" {
			sb.WriteString(fmt.Sprintf(" @ %s", e.Location))
		}
		sb.WriteString("\n")
		if e.Description != "" {
			sb.WriteString(fmt.Sprintf("  > %s\n", e.Description))
		}
	}

	return sb.String(), nil

}
