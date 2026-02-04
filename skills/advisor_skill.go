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
