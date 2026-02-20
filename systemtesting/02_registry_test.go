package systemtesting

import (
	"testing"

	"github.com/asccclass/pcai/internal/core"
	"github.com/ollama/ollama/api"
)

// ============================================================
// Stage 2: Registry — 工具註冊、查找與別名映射
// 測試 Registry 的 Register / CallTool / GetDefinitions
// ============================================================

// mockTool 是用於測試的假工具
type mockTool struct {
	name   string
	result string
}

func (m *mockTool) Name() string          { return m.name }
func (m *mockTool) Definition() api.Tool  { return api.Tool{Type: "function", Function: api.ToolFunction{Name: m.name, Description: "mock"}} }
func (m *mockTool) Run(argsJSON string) (string, error) { return m.result, nil }

// --- Register & CallTool ---

func TestRegistry_RegisterAndCall(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&mockTool{name: "read_calendars", result: "events_json"})

	result, err := reg.CallTool("read_calendars", `{"from":"2026-02-17","to":"2026-02-17"}`)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result != "events_json" {
		t.Errorf("Expected 'events_json', got %q", result)
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	reg := core.NewRegistry()
	_, err := reg.CallTool("nonexistent_tool", `{}`)
	if err == nil {
		t.Fatal("Expected error for unknown tool, got nil")
	}
}

func TestRegistry_AliasMapping(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&mockTool{name: "manage_cron_job", result: "cron_ok"})

	// 這些別名應該都能映射到 manage_cron_job
	aliases := []string{"manage_task", "manage_scheduler", "schedule_task", "task_planner"}
	for _, alias := range aliases {
		result, err := reg.CallTool(alias, `{}`)
		if err != nil {
			t.Errorf("Alias %q should map to manage_cron_job, got error: %v", alias, err)
			continue
		}
		if result != "cron_ok" {
			t.Errorf("Alias %q: expected 'cron_ok', got %q", alias, result)
		}
	}
}

// --- GetDefinitions ---

func TestRegistry_GetDefinitions(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&mockTool{name: "tool_a", result: ""})
	reg.Register(&mockTool{name: "tool_b", result: ""})

	defs := reg.GetDefinitions()
	if len(defs) != 2 {
		t.Errorf("Expected 2 definitions, got %d", len(defs))
	}
}

// --- SanitizeToolArgs (透過 CallTool 間接測試) ---

func TestRegistry_SanitizeNestedArgs(t *testing.T) {
	// 建立一個工具，將接收到的 argsJSON 回傳
	echoTool := &echoArgsTool{name: "echo_tool"}
	reg := core.NewRegistry()
	reg.Register(echoTool)

	// 傳入巢狀 JSON (LLM 幻覺格式)
	nestedArgs := `{"action":{"type":"string","value":"run_once"},"name":{"type":"string","value":"test"}}`
	result, err := reg.CallTool("echo_tool", nestedArgs)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	// sanitizeToolArgs 應該將其扁平化
	if result == nestedArgs {
		t.Error("Expected sanitized args, but got original nested args back")
	}
}

// echoArgsTool 回傳接收到的 argsJSON，用於驗證參數清理
type echoArgsTool struct {
	name string
}

func (e *echoArgsTool) Name() string          { return e.name }
func (e *echoArgsTool) Definition() api.Tool  { return api.Tool{Type: "function", Function: api.ToolFunction{Name: e.name}} }
func (e *echoArgsTool) Run(argsJSON string) (string, error) { return argsJSON, nil }

// --- Priority ---

func TestRegistry_PriorityOrdering(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&mockTool{name: "low_priority", result: ""})
	reg.RegisterWithPriority(&mockTool{name: "high_priority", result: ""}, 10)

	defs := reg.GetDefinitions()
	if len(defs) < 2 {
		t.Fatal("Expected at least 2 definitions")
	}
	// 高優先級應該排在前面
	if defs[0].Function.Name != "high_priority" {
		t.Errorf("Expected high_priority first, got %q", defs[0].Function.Name)
	}
}
