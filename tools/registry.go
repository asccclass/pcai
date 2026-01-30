package tools

import (
	"fmt"

	"github.com/ollama/ollama/api"
)

// Registry 管理所有可用的工具
type Registry struct {
	tools map[string]AgentTool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]AgentTool)}
}

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
