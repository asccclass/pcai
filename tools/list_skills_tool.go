package tools

import (
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/core"
	"github.com/ollama/ollama/api"
)

type ListSkillsTool struct {
	Registry *core.Registry
}

func (t *ListSkillsTool) Name() string { return "list_skills" }

func (t *ListSkillsTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "list_skills",
			Description: "列出系統中所有可用的技能 (Skills) 與工具 (Tools)。當使用者詢問『你有什麼技能？』、『你會做什麼？』或『列出所有功能』時，務必呼叫此工具。",
		},
	}
}

func (t *ListSkillsTool) Run(argsJSON string) (string, error) {
	if t.Registry == nil {
		return "錯誤：工具註冊表未初始化。", nil
	}

	defs := t.Registry.GetDefinitions()
	var sb strings.Builder
	sb.WriteString("【目前可用技能與工具列表】\n\n")

	for _, tool := range defs {
		name := tool.Function.Name
		desc := tool.Function.Description
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
	}

	return sb.String(), nil
}
