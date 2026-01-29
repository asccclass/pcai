package tools

/*
工具箱管理員 (tools/registry.go)
這個管理員負責保管所有工具，並提供統一的呼叫窗口。
*/

import (
	"fmt"

	"github.com/ollama/ollama/api"
)

// ToolRegistry 是我們的工具箱
type ToolRegistry struct {
	tools map[string]AgentTool
}

// NewRegistry 建立一個新工具箱
func NewRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]AgentTool),
	}
}

// Register 把工具放進箱子裡
func (r *ToolRegistry) Register(t AgentTool) {
	fmt.Printf("Registering tool: %s\n", t.Name())
	r.tools[t.Name()] = t
}

// GetDefinitions 一次打包所有工具的定義給 AI
func (r *ToolRegistry) GetDefinitions() []api.Tool {
	var defs []api.Tool
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Execute 根據名稱自動找到對應工具並執行
func (r *ToolRegistry) Execute(name string, argsJSON string) (string, error) {
	tool, exists := r.tools[name]
	if !exists {
		return "", fmt.Errorf("tool '%s' not found", name)
	}
	return tool.Run(argsJSON)
}
