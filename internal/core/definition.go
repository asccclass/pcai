package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ollama/ollama/api"
)

// AgentTool 是所有工具都必須實作的介面
type AgentTool interface {
	// Name 回傳工具名稱 (例如 "list_files")
	Name() string

	// Definition 回傳給 Ollama 看的 JSON Schema
	Definition() api.Tool

	// Run 接收 JSON 格式的參數字串，並回傳執行結果
	Run(argsJSON string) (string, error)
}

// toolEntry 包裝工具和其優先級
type toolEntry struct {
	tool     AgentTool
	priority int // 數字越大越優先
}

// Registry 管理所有可用的工具
type Registry struct {
	tools map[string]*toolEntry
}

// NewRegistry 建立新的註冊表
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*toolEntry)}
}

// Register 以預設優先級 (0) 註冊一個工具
func (r *Registry) Register(t AgentTool) {
	r.tools[t.Name()] = &toolEntry{tool: t, priority: 0}
}

// RegisterWithPriority 以指定優先級註冊一個工具（數字越大越優先）
func (r *Registry) RegisterWithPriority(t AgentTool, priority int) {
	r.tools[t.Name()] = &toolEntry{tool: t, priority: priority}
}

// sortedEntries 依優先級降序排列所有工具
func (r *Registry) sortedEntries() []*toolEntry {
	entries := make([]*toolEntry, 0, len(r.tools))
	for _, e := range r.tools {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})
	return entries
}

// GetDefinitions 取得所有工具的 api.Tool 定義（Skills 優先排列）
func (r *Registry) GetDefinitions() []api.Tool {
	sorted := r.sortedEntries()
	defs := make([]api.Tool, 0, len(sorted))
	for _, e := range sorted {
		defs = append(defs, e.tool.Definition())
	}
	return defs
}

// CallTool 根據 AI 的要求執行對應工具
func (r *Registry) CallTool(name string, argsJSON string) (string, error) {
	// [FIX] 工具名稱別名映射 (處理 LLM 幻覺)
	aliasMap := map[string]string{
		"manage_task":      "manage_cron_job",
		"manage_scheduler": "manage_cron_job",
		"schedule_task":    "manage_cron_job",
		"task_planner":     "manage_cron_job",
		"run_task":         "manage_cron_job",
		"cron":             "manage_cron_job",
		"manage_cron_task": "manage_cron_job",
	}
	if alias, ok := aliasMap[name]; ok {
		name = alias
	}

	// [FIX] 全域 JSON 參數清理：處理 LLM 幻覺產生的巢狀物件
	// 例如將 {"action":{"type":"string","value":"run_once"}}
	// 轉為   {"action":"run_once"}
	argsJSON = sanitizeToolArgs(argsJSON)

	entry, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("找不到工具: %s", name)
	}
	return entry.tool.Run(argsJSON)
}

// sanitizeToolArgs 清理 LLM 產生的巢狀 JSON 參數
// 將 {"type":"string","value":"X"} 轉為 "X"
// 將 {"type":"integer","value":5} 轉為 5
func sanitizeToolArgs(argsJSON string) string {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return argsJSON // 不是合法 JSON，原樣回傳
	}

	changed := false
	for key, val := range raw {
		if m, ok := val.(map[string]interface{}); ok {
			// 檢查是否為 {"type":"...", "value":"..."} 格式
			if _, hasType := m["type"]; hasType {
				if v, hasValue := m["value"]; hasValue {
					raw[key] = v
					changed = true
				}
			}
		}
	}

	if !changed {
		return argsJSON
	}

	fixed, err := json.Marshal(raw)
	if err != nil {
		return argsJSON
	}
	return string(fixed)
}

// GetToolPrompt 產生給 LLM 看的工具說明 (Schema)
// 高優先級工具會標註 [優先使用]
func (r *Registry) GetToolPrompt() string {
	sorted := r.sortedEntries()
	var sb strings.Builder
	for _, e := range sorted {
		def := e.tool.Definition()
		prefix := ""
		if e.priority > 0 {
			prefix = "[優先使用] "
		}
		sb.WriteString(fmt.Sprintf("- %s工具: %s\n", prefix, def.Function.Name))
		sb.WriteString(fmt.Sprintf("  描述: %s\n", def.Function.Description))
		// 將參數定義轉為 JSON 字串供 LLM 參考
		params, _ := json.Marshal(def.Function.Parameters)
		sb.WriteString(fmt.Sprintf("  參數Schema: %s\n", string(params)))
		sb.WriteString("\n")
	}
	return sb.String()
}
