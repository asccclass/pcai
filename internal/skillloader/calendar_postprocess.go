package skillloader

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// postProcessCalendarOutput 處理行事曆輸出，修正全天事件的結束日期
// Google Calendar API 的全天事件使用 exclusive end date：
// 例如 2 月 13 日整天的行程 → start.date="2026-02-13", end.date="2026-02-14"
// 此函式會將結束日期調整為正確的 inclusive date
func postProcessCalendarOutput(output string) string {
	// 嘗試解析 JSON 格式的輸出
	output = strings.TrimSpace(output)

	// 嘗試解析為事件容器 {"events": [...]}
	var container struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal([]byte(output), &container); err == nil && len(container.Events) > 0 {
		return processJSONEvents(container.Events)
	}

	// 嘗試解析為事件陣列 [...]
	var events []json.RawMessage
	if err := json.Unmarshal([]byte(output), &events); err == nil && len(events) > 0 {
		return processJSONEvents(events)
	}

	// 非 JSON 格式：使用文字替換修正常見的日期格式
	return processTextOutput(output)
}

// calendarEventRaw 用於解析和修正日期
type calendarEventRaw struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"end"`
	Status string `json:"status"`
}

// processJSONEvents 處理 JSON 格式的事件列表
func processJSONEvents(rawEvents []json.RawMessage) string {
	var results []string

	for _, raw := range rawEvents {
		var event calendarEventRaw
		if err := json.Unmarshal(raw, &event); err != nil {
			results = append(results, string(raw))
			continue
		}

		// 修正全天事件的結束日期
		if event.Start.Date != "" && event.End.Date != "" && event.Start.DateTime == "" {
			// 這是全天事件，end.date 是 exclusive 的
			correctedEnd := fixExclusiveEndDate(event.Start.Date, event.End.Date)
			event.End.Date = correctedEnd
		}

		// 格式化為人類可讀的文字
		results = append(results, formatEventForLLM(event))
	}

	return strings.Join(results, "\n")
}

// fixExclusiveEndDate 修正 Google Calendar 的 exclusive end date
// 如果 end = start + 1 天，表示是單天事件，end 應等於 start
// 如果 end > start + 1 天，表示是多天事件，end 應減 1 天
func fixExclusiveEndDate(startDate, endDate string) string {
	start, errS := time.Parse("2006-01-02", startDate)
	end, errE := time.Parse("2006-01-02", endDate)
	if errS != nil || errE != nil {
		return endDate
	}

	// end 減去一天，得到 inclusive 的最後一天
	corrected := end.AddDate(0, 0, -1)

	// 如果修正後小於 start，保持不變（異常情況）
	if corrected.Before(start) {
		return startDate
	}

	return corrected.Format("2006-01-02")
}

// formatEventForLLM 將事件格式化為 LLM 友好的文字
func formatEventForLLM(e calendarEventRaw) string {
	var sb strings.Builder

	// 判斷事件類型
	if e.Start.Date != "" && e.Start.DateTime == "" {
		// 全天事件
		if e.Start.Date == e.End.Date {
			// 單天全天事件
			sb.WriteString(fmt.Sprintf("📅 [整天] %s (%s)", e.Summary, e.Start.Date))
		} else {
			// 多天全天事件
			sb.WriteString(fmt.Sprintf("📅 [多天] %s (%s 至 %s)", e.Summary, e.Start.Date, e.End.Date))
		}
	} else if e.Start.DateTime != "" {
		// 有具體時間的事件
		startTime, errS := time.Parse(time.RFC3339, e.Start.DateTime)
		endTime, errE := time.Parse(time.RFC3339, e.End.DateTime)
		if errS == nil && errE == nil {
			sb.WriteString(fmt.Sprintf("🕐 %s (%s ~ %s)",
				e.Summary,
				startTime.Format("2006-01-02 15:04"),
				endTime.Format("15:04"),
			))
		} else {
			sb.WriteString(fmt.Sprintf("🕐 %s (%s)", e.Summary, e.Start.DateTime))
		}
	}

	// 附加位置和描述
	if e.Location != "" {
		sb.WriteString(fmt.Sprintf(" | 地點: %s", e.Location))
	}
	if e.Description != "" {
		// 截斷過長的描述
		desc := e.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf(" | 備註: %s", desc))
	}
	if e.Status == "tentative" {
		sb.WriteString(" [待確認]")
	}

	return sb.String()
}

// processTextOutput 處理非 JSON 的文字輸出
// 嘗試找出並修正日期模式
func processTextOutput(output string) string {
	// 直接回傳，不做修改（因為文字格式不容易精確修正）
	// 但如果輸出包含 JSON 物件段落，嘗試逐行處理
	return output
}
