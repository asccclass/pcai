package systemtesting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/asccclass/pcai/internal/skillloader"
)

// ============================================================
// Stage 5: Calendar Post-Processing — 日期修正與格式化
// 測試 fixExclusiveEndDate / formatEventForLLM / postProcessCalendarOutput
// ============================================================

// --- fixExclusiveEndDate ---

func TestFixExclusiveEndDate_SingleDay(t *testing.T) {
	// 單天全天事件：start=2026-02-17, end=2026-02-18 → 應修正為 2026-02-17
	result := skillloader.ExportFixExclusiveEndDate("2026-02-17", "2026-02-18")
	if result != "2026-02-17" {
		t.Errorf("Expected 2026-02-17, got %s", result)
	}
}

func TestFixExclusiveEndDate_MultiDay(t *testing.T) {
	// 多天全天事件：start=2026-02-15, end=2026-02-18 → 應修正為 2026-02-17
	result := skillloader.ExportFixExclusiveEndDate("2026-02-15", "2026-02-18")
	if result != "2026-02-17" {
		t.Errorf("Expected 2026-02-17, got %s", result)
	}
}

func TestFixExclusiveEndDate_SameStartEnd(t *testing.T) {
	// 異常情況：start=end → end-1 < start → 應回傳 startDate
	result := skillloader.ExportFixExclusiveEndDate("2026-02-17", "2026-02-17")
	// end-1 = 2026-02-16, 2026-02-16 < start(2026-02-17) → 回傳 start
	if result != "2026-02-17" {
		t.Errorf("Expected 2026-02-17 (startDate), got %s", result)
	}
}

func TestFixExclusiveEndDate_InvalidDate(t *testing.T) {
	// 無效日期格式：應回傳原始 endDate
	result := skillloader.ExportFixExclusiveEndDate("invalid", "2026-02-18")
	if result != "2026-02-18" {
		t.Errorf("Expected original endDate for invalid input, got %s", result)
	}
}

func TestFixExclusiveEndDate_CrossMonth(t *testing.T) {
	// 跨月事件：start=2026-01-31, end=2026-02-02 → 修正為 2026-02-01
	result := skillloader.ExportFixExclusiveEndDate("2026-01-31", "2026-02-02")
	if result != "2026-02-01" {
		t.Errorf("Expected 2026-02-01, got %s", result)
	}
}

// --- formatEventForLLM ---

func TestFormatEventForLLM_AllDay_SingleDay(t *testing.T) {
	event := skillloader.ExportCalendarEventRaw{
		Summary: "Team Meeting",
	}
	event.Start.Date = "2026-02-17"
	event.End.Date = "2026-02-17" // 已修正後的 inclusive date

	result := skillloader.ExportFormatGoogleEventForLLM(event)
	if !strings.Contains(result, "📅") {
		t.Errorf("All-day event should contain 📅, got: %s", result)
	}
	if !strings.Contains(result, "[整天]") {
		t.Errorf("Single-day event should contain [整天], got: %s", result)
	}
	if !strings.Contains(result, "Team Meeting") {
		t.Errorf("Should contain event summary, got: %s", result)
	}
}

func TestFormatEventForLLM_AllDay_MultiDay(t *testing.T) {
	event := skillloader.ExportCalendarEventRaw{
		Summary: "Conference",
	}
	event.Start.Date = "2026-02-15"
	event.End.Date = "2026-02-17"

	result := skillloader.ExportFormatGoogleEventForLLM(event)
	if !strings.Contains(result, "[多天]") {
		t.Errorf("Multi-day event should contain [多天], got: %s", result)
	}
	if !strings.Contains(result, "2026-02-15") || !strings.Contains(result, "2026-02-17") {
		t.Errorf("Should contain start and end dates, got: %s", result)
	}
}

func TestFormatEventForLLM_TimedEvent(t *testing.T) {
	event := skillloader.ExportCalendarEventRaw{
		Summary: "Standup",
	}
	event.Start.DateTime = "2026-02-17T09:00:00+08:00"
	event.End.DateTime = "2026-02-17T09:30:00+08:00"

	result := skillloader.ExportFormatGoogleEventForLLM(event)
	if !strings.Contains(result, "🕐") {
		t.Errorf("Timed event should contain 🕐, got: %s", result)
	}
	if !strings.Contains(result, "Standup") {
		t.Errorf("Should contain event summary, got: %s", result)
	}
}

func TestFormatEventForLLM_WithLocation(t *testing.T) {
	event := skillloader.ExportCalendarEventRaw{
		Summary:  "Lunch",
		Location: "台北101",
	}
	event.Start.DateTime = "2026-02-17T12:00:00+08:00"
	event.End.DateTime = "2026-02-17T13:00:00+08:00"

	result := skillloader.ExportFormatGoogleEventForLLM(event)
	if !strings.Contains(result, "地點: 台北101") {
		t.Errorf("Should contain location, got: %s", result)
	}
}

func TestFormatEventForLLM_WithDescription(t *testing.T) {
	event := skillloader.ExportCalendarEventRaw{
		Summary:     "Review",
		Description: "討論 Q1 成果",
	}
	event.Start.Date = "2026-02-17"
	event.End.Date = "2026-02-17"

	result := skillloader.ExportFormatGoogleEventForLLM(event)
	if !strings.Contains(result, "備註:") {
		t.Errorf("Should contain description prefix, got: %s", result)
	}
}

func TestFormatEventForLLM_TentativeStatus(t *testing.T) {
	event := skillloader.ExportCalendarEventRaw{
		Summary: "Maybe meeting",
		Status:  "tentative",
	}
	event.Start.Date = "2026-02-17"
	event.End.Date = "2026-02-17"

	result := skillloader.ExportFormatGoogleEventForLLM(event)
	if !strings.Contains(result, "[待確認]") {
		t.Errorf("Tentative event should contain [待確認], got: %s", result)
	}
}

// --- postProcessCalendarOutput ---

func TestPostProcessCalendarOutput_ContainerFormat(t *testing.T) {
	// 模擬 gog 回傳的 {"events": [...]} 格式
	input := `{"events":[{"summary":"Weekly Sync","start":{"date":"2026-02-17"},"end":{"date":"2026-02-18"},"status":"confirmed"}]}`
	result := skillloader.ExportPostProcessCalendarOutput(input)

	if !strings.Contains(result, "Weekly Sync") {
		t.Errorf("Should contain event summary, got: %s", result)
	}
	if !strings.Contains(result, "📅") {
		t.Errorf("Should contain calendar emoji, got: %s", result)
	}
}

func TestPostProcessCalendarOutput_ArrayFormat(t *testing.T) {
	input := `[{"summary":"Standup","start":{"dateTime":"2026-02-17T09:00:00+08:00"},"end":{"dateTime":"2026-02-17T09:30:00+08:00"}}]`
	result := skillloader.ExportPostProcessCalendarOutput(input)

	if !strings.Contains(result, "Standup") {
		t.Errorf("Should contain event summary, got: %s", result)
	}
}

func TestPostProcessCalendarOutput_PlainText(t *testing.T) {
	input := "No events found"
	result := skillloader.ExportPostProcessCalendarOutput(input)

	if result != "No events found" {
		t.Errorf("Plain text should be returned as-is, got: %s", result)
	}
}

func TestPostProcessCalendarOutput_MultipleEvents(t *testing.T) {
	events := []map[string]interface{}{
		{
			"summary": "Morning Standup",
			"start":   map[string]string{"dateTime": "2026-02-17T09:00:00+08:00"},
			"end":     map[string]string{"dateTime": "2026-02-17T09:30:00+08:00"},
		},
		{
			"summary": "Lunch Break",
			"start":   map[string]string{"date": "2026-02-17"},
			"end":     map[string]string{"date": "2026-02-18"},
		},
	}

	container := map[string]interface{}{"events": events}
	jsonBytes, _ := json.Marshal(container)
	result := skillloader.ExportPostProcessCalendarOutput(string(jsonBytes))

	if !strings.Contains(result, "Morning Standup") {
		t.Errorf("Should contain first event, got: %s", result)
	}
	if !strings.Contains(result, "Lunch Break") {
		t.Errorf("Should contain second event, got: %s", result)
	}
}

func TestPostProcessCalendarOutput_ExclusiveDateCorrection(t *testing.T) {
	// 驗證後處理是否自動修正 exclusive end date
	input := `{"events":[{"summary":"全天會議","start":{"date":"2026-02-17"},"end":{"date":"2026-02-18"}}]}`
	result := skillloader.ExportPostProcessCalendarOutput(input)

	// 修正後的單天事件應該顯示 [整天] 而不是 [多天]
	if !strings.Contains(result, "[整天]") {
		t.Errorf("Single-day event (after correction) should show [整天], got: %s", result)
	}
	// 不應該出現 2026-02-18
	if strings.Contains(result, "2026-02-18") {
		t.Errorf("Corrected event should not contain exclusive end date 2026-02-18, got: %s", result)
	}
}
