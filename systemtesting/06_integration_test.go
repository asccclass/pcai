package systemtesting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/skillloader"
)

// ============================================================
// Stage 6: Integration — 端對端流程串接測試
// 模擬從使用者輸入到最終格式化輸出的完整流程
// ============================================================

// TestIntegration_ToolHintToRegistry 測試從關鍵字偵測到工具查找
func TestIntegration_ToolHintToRegistry(t *testing.T) {
	// Step 1: Tool Hint 偵測
	hint := agent.ExportGetToolHint("列出今天的行事曆")
	if hint == "" {
		t.Fatal("Tool hint should detect calendar keyword")
	}
	if !strings.Contains(hint, "manage_calendar") {
		t.Fatal("Hint should mention manage_calendar")
	}

	// Step 2: Registry 查找
	reg := core.NewRegistry()
	def := &skillloader.SkillDefinition{
		Name:        "manage_calendar",
		Description: "Read calendar events",
		Command:     "echo {{from}} {{to}}",
		Params:      skillloader.ParseParams("echo {{from}} {{to}}"),
	}
	tool := skillloader.NewDynamicTool(def, reg, nil)
	reg.Register(tool)

	// Step 3: 驗證 Registry 能找到工具
	result, err := reg.CallTool("manage_calendar", `{"from":"2026-02-17","to":"2026-02-17"}`)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !strings.Contains(result, "2026-02-17") {
		t.Errorf("Expected date in result, got: %s", result)
	}
}

// TestIntegration_FullCalendarPipeline 測試 SkillLoader → DynamicTool → PostProcess 的完整流程
func TestIntegration_FullCalendarPipeline(t *testing.T) {
	// Step 1: 從實際檔案載入技能
	cwd, _ := os.Getwd()
	skillsDir := filepath.Join(cwd, "..", "skills")

	loadedSkills, err := skillloader.LoadSkills(skillsDir)
	if err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	// Step 2: 找到 manage_calendar 技能
	var calendarSkill *skillloader.SkillDefinition
	for _, s := range loadedSkills {
		if s.Name == "manage_calendar" {
			calendarSkill = s
			break
		}
	}
	if calendarSkill == nil {
		t.Fatal("manage_calendar skill not found")
	}

	// Step 3: 驗證技能定義
	if !strings.Contains(calendarSkill.Command, "{{from}}") {
		t.Error("Command should contain {{from}} placeholder")
	}
	if !strings.Contains(calendarSkill.Command, "{{to}}") {
		t.Error("Command should contain {{to}} placeholder")
	}

	// Step 4: 建立 DynamicTool 並驗證定義
	tool := skillloader.NewDynamicTool(calendarSkill, nil, nil)
	apiDef := tool.Definition()
	if apiDef.Function.Name != "manage_calendar" {
		t.Errorf("Expected name 'manage_calendar', got %q", apiDef.Function.Name)
	}

	// Step 5: 模擬後處理
	mockGogOutput := `{"events":[
		{"summary":"Team Standup","start":{"dateTime":"2026-02-17T09:00:00+08:00"},"end":{"dateTime":"2026-02-17T09:30:00+08:00"}},
		{"summary":"All Day Event","start":{"date":"2026-02-17"},"end":{"date":"2026-02-18"},"status":"tentative"},
		{"summary":"Multi Day","start":{"date":"2026-02-15"},"end":{"date":"2026-02-18"},"location":"台北"}
	]}`

	formatted := skillloader.ExportPostProcessCalendarOutput(mockGogOutput)

	// 驗證格式化結果
	if !strings.Contains(formatted, "Team Standup") {
		t.Error("Formatted output should contain 'Team Standup'")
	}
	if !strings.Contains(formatted, "🕐") {
		t.Error("Timed event should have 🕐 icon")
	}
	if !strings.Contains(formatted, "[整天]") {
		t.Error("Single all-day event should show [整天]")
	}
	if !strings.Contains(formatted, "[待確認]") {
		t.Error("Tentative event should show [待確認]")
	}
	if !strings.Contains(formatted, "[多天]") {
		t.Error("Multi-day event should show [多天]")
	}
	if !strings.Contains(formatted, "地點: 台北") {
		t.Error("Event with location should show location")
	}
}

// TestIntegration_HintKeywordsMatchToolNames 確認所有 hint 關鍵字都有對應的工具名稱
func TestIntegration_HintKeywordsMatchToolNames(t *testing.T) {
	// 測試各種關鍵字都能產生包含正確工具名稱的 hint
	testCases := []struct {
		input    string
		toolName string
	}{
		{"今天的行事曆", "manage_calendar"},
		{"查看行程", "manage_calendar"},
		{"check my schedule", "manage_calendar"},
		{"幫我讀郵件", "read_email"},
		{"check my email", "read_email"},
		{"天氣如何", "get_taiwan_weather"},
	}

	for _, tc := range testCases {
		hint := agent.ExportGetToolHint(tc.input)
		if hint == "" {
			t.Errorf("Input %q: expected non-empty hint", tc.input)
			continue
		}
		if !strings.Contains(hint, tc.toolName) {
			t.Errorf("Input %q: expected hint to contain %q, got: %s", tc.input, tc.toolName, hint[:80])
		}
	}
}
