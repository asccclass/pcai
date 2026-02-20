package systemtesting

import (
	"strings"
	"testing"

	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/skillloader"
)

// ============================================================
// Stage 4: Dynamic Tool — 工具定義生成與參數替換
// 測試 DynamicTool.Name / Definition / Run 的參數替換邏輯
// ============================================================

// newTestDef 建立測試用的 SkillDefinition
func newTestDef(name, command string) *skillloader.SkillDefinition {
	return &skillloader.SkillDefinition{
		Name:        name,
		Description: "Test skill: " + name,
		Command:     command,
		Params:      skillloader.ParseParams(command),
	}
}

// --- Name ---

func TestDynamicTool_Name(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"read_calendars", "read_calendars"},
		{"My Tool", "my_tool"},
		{"GoogleSearch", "googlesearch"},
	}
	for _, tc := range tests {
		def := newTestDef(tc.input, "echo hello")
		tool := skillloader.NewDynamicTool(def, nil, nil)
		if tool.Name() != tc.expected {
			t.Errorf("Name(%q): expected %q, got %q", tc.input, tc.expected, tool.Name())
		}
	}
}

// --- Definition ---

func TestDynamicTool_Definition_HasCorrectParams(t *testing.T) {
	def := newTestDef("read_calendars", "gog calendar events --from {{from}} --to {{to}} --json")
	tool := skillloader.NewDynamicTool(def, nil, nil)
	apiDef := tool.Definition()

	if apiDef.Function.Name != "read_calendars" {
		t.Errorf("Expected name 'read_calendars', got %q", apiDef.Function.Name)
	}

	params := apiDef.Function.Parameters
	if params.Type != "object" {
		t.Errorf("Expected params type 'object', got %q", params.Type)
	}

	if len(params.Required) != 2 {
		t.Errorf("Expected 2 required params, got %d: %v", len(params.Required), params.Required)
	}

	// 確認 required 包含 from 和 to
	requiredMap := map[string]bool{}
	for _, r := range params.Required {
		requiredMap[r] = true
	}
	if !requiredMap["from"] || !requiredMap["to"] {
		t.Errorf("Required should contain 'from' and 'to', got %v", params.Required)
	}
}

// --- Registration in Registry ---

func TestDynamicTool_RegisterInRegistry(t *testing.T) {
	def := newTestDef("read_calendars", "gog calendar events --from {{from}} --to {{to}} --json")
	reg := core.NewRegistry()
	tool := skillloader.NewDynamicTool(def, reg, nil)
	reg.Register(tool)

	defs := reg.GetDefinitions()
	found := false
	for _, d := range defs {
		if d.Function.Name == "read_calendars" {
			found = true
			break
		}
	}
	if !found {
		t.Error("read_calendars not found in registry definitions")
	}
}

// --- Parameter Substitution (透過 Run，但 echo 指令可在 Windows 執行) ---

func TestDynamicTool_Run_ParamSubstitution(t *testing.T) {
	// 使用 echo 指令來驗證參數替換
	def := newTestDef("test_echo", "echo from={{from}} to={{to}}")
	tool := skillloader.NewDynamicTool(def, nil, nil)

	result, err := tool.Run(`{"from":"2026-02-17","to":"2026-02-17"}`)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !strings.Contains(result, "from=2026-02-17") {
		t.Errorf("Expected 'from=2026-02-17' in output, got: %q", result)
	}
	if !strings.Contains(result, "to=2026-02-17") {
		t.Errorf("Expected 'to=2026-02-17' in output, got: %q", result)
	}
}

func TestDynamicTool_Run_UnresolvedPlaceholder(t *testing.T) {
	def := newTestDef("test_missing", "echo {{from}} {{to}}")
	tool := skillloader.NewDynamicTool(def, nil, nil)

	// 只傳入 from，缺少 to
	_, err := tool.Run(`{"from":"2026-02-17"}`)
	if err == nil {
		t.Error("Expected error for unresolved placeholder, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "未完全替換") {
		t.Errorf("Expected '未完全替換' in error message, got: %v", err)
	}
}

func TestDynamicTool_Run_InvalidJSON(t *testing.T) {
	def := newTestDef("test_invalid", "echo {{msg}}")
	tool := skillloader.NewDynamicTool(def, nil, nil)

	_, err := tool.Run("not-a-json")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
