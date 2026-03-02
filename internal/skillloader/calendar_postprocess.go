package skillloader

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// --- calendar.exe 實際回傳格式的結構定義 ---

// calendarGroup 代表 calendar.exe 輸出的每個行事曆群組
type calendarGroup struct {
	Creator string          `json:"creator"`
	Events  []calendarEvent `json:"events"`
}

// calendarEvent 代表 calendar.exe 輸出的單一事件
type calendarEvent struct {
	ID        string `json:"id"`         // Google Calendar 事實上的事件 ID
	StartTime string `json:"start_time"` // "2026-02-27 10:00:00"
	EndTime   string `json:"end_time"`   // "2026-02-27 11:00:00"
	EventName string `json:"event_name"` // "[繳費]玉山銀行信用卡..."
	Summary   string `json:"summary"`    // 備註/描述
}

// postProcessCalendarOutput 處理行事曆輸出，將 calendar.exe 的 JSON 轉換為 LLM 友好的文字
// 同時修正 Google Calendar 全天事件的 exclusive end date
func postProcessCalendarOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return output
	}

	// 1. 嘗試解析為 calendar.exe 的格式: [{creator, events: [...]}]
	var groups []calendarGroup
	if err := json.Unmarshal([]byte(output), &groups); err == nil && len(groups) > 0 {
		result := processCalendarGroups(groups)
		if result != "" {
			return result
		}
	}

	// 2. Fallback: 嘗試解析為 Google Calendar API 原始格式 {"events": [...]}
	var container struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal([]byte(output), &container); err == nil && len(container.Events) > 0 {
		return processGoogleAPIEvents(container.Events)
	}

	// 3. Fallback: 嘗試解析為純事件陣列 [...]
	var rawEvents []json.RawMessage
	if err := json.Unmarshal([]byte(output), &rawEvents); err == nil && len(rawEvents) > 0 {
		return processGoogleAPIEvents(rawEvents)
	}

	// 4. 非 JSON 格式：直接回傳原始文字
	return output
}

// processCalendarGroups 處理 calendar.exe 格式的行事曆群組
func processCalendarGroups(groups []calendarGroup) string {
	var results []string

	for _, group := range groups {
		if len(group.Events) == 0 {
			continue
		}
		for _, event := range group.Events {
			line := formatCalendarEvent(group.Creator, event)
			if line != "" {
				results = append(results, line)
			}
		}
	}

	if len(results) == 0 {
		return ""
	}
	return strings.Join(results, "\n")
}

// formatCalendarEvent 將 calendar.exe 的事件格式化為 LLM 友好的文字
func formatCalendarEvent(creator string, e calendarEvent) string {
	var sb strings.Builder

	// 解析時間 (格式: "2006-01-02 15:04:05")
	const timeLayout = "2006-01-02 15:04:05"
	startTime, errS := time.Parse(timeLayout, e.StartTime)
	endTime, errE := time.Parse(timeLayout, e.EndTime)

	if errS != nil || errE != nil {
		// 無法解析時間，直接拼接原始資訊
		sb.WriteString(fmt.Sprintf("📅 %s (%s ~ %s)", e.EventName, e.StartTime, e.EndTime))
	} else {
		// 判斷是否為全天事件：開始時間為 00:00:00 且結束時間為隔天 00:00:00
		isAllDay := startTime.Hour() == 0 && startTime.Minute() == 0 && startTime.Second() == 0 &&
			endTime.Hour() == 0 && endTime.Minute() == 0 && endTime.Second() == 0

		if isAllDay {
			// 修正 exclusive end date (隔天 00:00 → 回到前一天)
			correctedEnd := endTime.AddDate(0, 0, -1)
			if correctedEnd.Before(startTime) {
				correctedEnd = startTime
			}
			if startTime.Format("2006-01-02") == correctedEnd.Format("2006-01-02") {
				sb.WriteString(fmt.Sprintf("📅 [整天] %s (%s)", e.EventName, startTime.Format("2006-01-02")))
			} else {
				sb.WriteString(fmt.Sprintf("📅 [多天] %s (%s 至 %s)", e.EventName, startTime.Format("2006-01-02"), correctedEnd.Format("2006-01-02")))
			}
		} else {
			// 有具體時間的事件
			if startTime.Format("2006-01-02") == endTime.Format("2006-01-02") {
				// 同一天
				sb.WriteString(fmt.Sprintf("🕐 %s (%s %s ~ %s)",
					e.EventName,
					startTime.Format("2006-01-02"),
					startTime.Format("15:04"),
					endTime.Format("15:04"),
				))
			} else {
				// 跨天
				sb.WriteString(fmt.Sprintf("🕐 %s (%s ~ %s)",
					e.EventName,
					startTime.Format("2006-01-02 15:04"),
					endTime.Format("2006-01-02 15:04"),
				))
			}
		}
	}

	// 附加行事曆來源
	sb.WriteString(fmt.Sprintf(" @%s", creator))

	// 附加備註
	if e.Summary != "" {
		desc := e.Summary
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf(" | 備註: %s", desc))
	}

	// 附加 ID
	if e.ID != "" {
		sb.WriteString(fmt.Sprintf("\n    - ID: %s", e.ID))
	}

	return sb.String()
}

// --- Google Calendar API 原始格式的支援 (Fallback) ---

// googleCalendarEventRaw 用於解析 Google Calendar API 原始格式
type googleCalendarEventRaw struct {
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

// processGoogleAPIEvents 處理 Google Calendar API 原始格式的事件
func processGoogleAPIEvents(rawEvents []json.RawMessage) string {
	var results []string

	for _, raw := range rawEvents {
		var event googleCalendarEventRaw
		if err := json.Unmarshal(raw, &event); err != nil {
			results = append(results, string(raw))
			continue
		}

		// 修正全天事件的結束日期
		if event.Start.Date != "" && event.End.Date != "" && event.Start.DateTime == "" {
			correctedEnd := fixExclusiveEndDate(event.Start.Date, event.End.Date)
			event.End.Date = correctedEnd
		}

		results = append(results, formatGoogleEventForLLM(event))
	}

	return strings.Join(results, "\n")
}

// fixExclusiveEndDate 修正 Google Calendar 的 exclusive end date
func fixExclusiveEndDate(startDate, endDate string) string {
	start, errS := time.Parse("2006-01-02", startDate)
	end, errE := time.Parse("2006-01-02", endDate)
	if errS != nil || errE != nil {
		return endDate
	}

	corrected := end.AddDate(0, 0, -1)
	if corrected.Before(start) {
		return startDate
	}

	return corrected.Format("2006-01-02")
}

// formatGoogleEventForLLM 將 Google API 格式的事件轉成文字
func formatGoogleEventForLLM(e googleCalendarEventRaw) string {
	var sb strings.Builder

	if e.Start.Date != "" && e.Start.DateTime == "" {
		if e.Start.Date == e.End.Date {
			sb.WriteString(fmt.Sprintf("📅 [整天] %s (%s)", e.Summary, e.Start.Date))
		} else {
			sb.WriteString(fmt.Sprintf("📅 [多天] %s (%s 至 %s)", e.Summary, e.Start.Date, e.End.Date))
		}
	} else if e.Start.DateTime != "" {
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

	if e.Location != "" {
		sb.WriteString(fmt.Sprintf(" | 地點: %s", e.Location))
	}
	if e.Description != "" {
		desc := e.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf(" | 備註: %s", desc))
	}
	if e.Status == "tentative" {
		sb.WriteString(" [待確認]")
	}
	if e.ID != "" {
		sb.WriteString(fmt.Sprintf("\n    - ID: %s", e.ID))
	}

	return sb.String()
}
