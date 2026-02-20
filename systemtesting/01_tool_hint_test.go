package systemtesting

import (
	"strings"
	"testing"
	"time"

	"github.com/asccclass/pcai/internal/agent"
)

// ============================================================
// Stage 1: Tool Hint — 關鍵字偵測與提示注入
// 測試 getToolHint 是否能正確偵測行事曆關鍵字並注入系統提示
// ============================================================

func TestToolHint_CalendarKeyword_Chinese(t *testing.T) {
	keywords := []string{
		"列出今天的行事曆",
		"查看行程",
		"今日日程",
		"幫我看行程表",
	}
	for _, input := range keywords {
		hint := agent.ExportGetToolHint(input)
		if hint == "" {
			t.Errorf("Expected hint for input %q, got empty string", input)
			continue
		}
		if !strings.Contains(hint, "read_calendars") {
			t.Errorf("Hint for %q should mention 'read_calendars', got: %s", input, hint)
		}
	}
}

func TestToolHint_CalendarKeyword_English(t *testing.T) {
	keywords := []string{
		"show my calendar",
		"check schedule today",
	}
	for _, input := range keywords {
		hint := agent.ExportGetToolHint(input)
		if hint == "" {
			t.Errorf("Expected hint for input %q, got empty string", input)
			continue
		}
		if !strings.Contains(hint, "read_calendars") {
			t.Errorf("Hint for %q should mention 'read_calendars', got: %s", input, hint)
		}
	}
}

func TestToolHint_ContainsTodayDate(t *testing.T) {
	hint := agent.ExportGetToolHint("列出今天的行事曆")
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(hint, today) {
		t.Errorf("Hint should contain today's date %s, got: %s", today, hint)
	}
}

func TestToolHint_NoMatch(t *testing.T) {
	inputs := []string{
		"你好",
		"幫我寫程式",
		"今天天氣如何", // 這會匹配天氣，不是行事曆
	}
	for _, input := range inputs {
		hint := agent.ExportGetToolHint(input)
		if hint != "" && strings.Contains(hint, "read_calendars") {
			t.Errorf("Input %q should NOT match calendar hint, got: %s", input, hint)
		}
	}
}

func TestToolHint_EmailKeyword(t *testing.T) {
	hint := agent.ExportGetToolHint("幫我讀取郵件")
	if hint == "" {
		t.Fatal("Expected hint for email keyword, got empty")
	}
	if !strings.Contains(hint, "read_email") {
		t.Errorf("Email hint should mention 'read_email', got: %s", hint)
	}
}

func TestToolHint_WeatherKeyword(t *testing.T) {
	hint := agent.ExportGetToolHint("台北天氣如何")
	if hint == "" {
		t.Fatal("Expected hint for weather keyword, got empty")
	}
	if !strings.Contains(hint, "get_taiwan_weather") {
		t.Errorf("Weather hint should mention 'get_taiwan_weather', got: %s", hint)
	}
}
