package core

import (
	"encoding/json"
	"fmt"
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

// Registry 管理所有可用的工具
type Registry struct {
	tools map[string]AgentTool
}

// NewRegistry 建立新的註冊表
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]AgentTool)}
}

// Register 註冊一個工具
func (r *Registry) Register(t AgentTool) {
	r.tools[t.Name()] = t
}

// GetDefinitions 取得所有工具的 api.Tool 定義，準備傳給 Ollama
func (r *Registry) GetDefinitions() []api.Tool {
	defs := []api.Tool{}
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// CallTool 根據 AI 的要求執行對應工具
func (r *Registry) CallTool(name string, argsJSON string) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("找不到工具: %s", name)
	}
	return tool.Run(argsJSON)
}

// GetToolPrompt 產生給 LLM 看的工具說明 (Schema)
func (r *Registry) GetToolPrompt() string {
	var sb strings.Builder
	for _, t := range r.tools {
		def := t.Definition()
		sb.WriteString(fmt.Sprintf("- 工具: %s\n", def.Function.Name))
		sb.WriteString(fmt.Sprintf("  描述: %s\n", def.Function.Description))
		// 將參數定義轉為 JSON 字串供 LLM 參考
		params, _ := json.Marshal(def.Function.Parameters)
		sb.WriteString(fmt.Sprintf("  參數Schema: %s\n", string(params)))
		sb.WriteString("\n")
	}
	return sb.String()
}
