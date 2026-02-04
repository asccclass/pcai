package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/asccclass/pcai/internal/advisor"
	"github.com/ollama/ollama/api"
)

// AdvisorSkill 包裝架構分析邏輯
type AdvisorSkill struct {
	client    *api.Client
	modelName string
}

func NewAdvisorSkill(client *api.Client, modelName string) *AdvisorSkill {
	return &AdvisorSkill{
		client:    client,
		modelName: modelName,
	}
}

// AdvisorTool 是暴露給 AI 使用的工具
type AdvisorTool struct {
	skill *AdvisorSkill
}

// Ensure AgentTool interface is implemented
// var _ tools.AgentTool = (*AdvisorTool)(nil) // We can't import tools here carefully regarding cyclic imports if tools imports skills.
// Usually interfaces are defined in a common place or tools package.
// If skills imports tools, and tools imports skills, it's a cycle.
// Check tools/base.go -> it's in package tools.
// Check tools/init.go -> imports skills.
// So skills CANNOT import tools. We must define the Tool struct here, but its methods must satisfy the interface expected by `tools` package.
// However, the `tools` package expects `AgentTool` interface.
// If we want to register this in `tools/init.go`, we need to return something that satisfies `AgentTool`.
// The `AgentTool` interface is in `tools` package.
// To avoid cyclic dependency:
// 1. Define AdvisorTool in `tools/` package that holds `AdvisorSkill`. OR
// 2. Define AdvisorTool in `skills/` but don't import `tools`.
// The registry expects `AgentTool`. `AgentTool` returns `api.Tool`. `api` is from ollama/api.
// So `skills` can import `ollama/api` and implement the methods without importing `tools`.
// That works!

func (s *AdvisorSkill) CreateTool() *AdvisorTool {
	return &AdvisorTool{skill: s}
}

func (t *AdvisorTool) Name() string {
	return "analyze_architecture"
}

func (t *AdvisorTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "analyze_architecture",
			Description: "Analyze a requirement to decide if it should be a Skill or a Tool.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"description": {
						"type": "string",
						"description": "The requirement description to analyze."
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"description"},
				}
			}(),
		},
	}
}

func (t *AdvisorTool) Run(argsJSON string) (string, error) {
	var args struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	result, err := advisor.Analyze(context.Background(), t.skill.client, t.skill.modelName, args.Description)
	if err != nil {
		return "", err
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return string(output), nil
}
