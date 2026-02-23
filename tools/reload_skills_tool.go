package tools

import (
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"
)

// ReloadSkillsTool 允許使用者在不重啟程式的情況下重新載入 Skills
type ReloadSkillsTool struct {
	Manager *SkillManager
}

func (t *ReloadSkillsTool) Name() string {
	return "reload_skills"
}

func (t *ReloadSkillsTool) IsSkill() bool {
	return false
}

func (t *ReloadSkillsTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: "Reload all skills from the disk. Use this after adding or modifying SKILL.md files to make changes take effect immediately.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"confirm": {
						"type":        "boolean",
						"description": "Set to true to confirm reload",
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"confirm"},
				}
			}(),
		},
	}
}

func (t *ReloadSkillsTool) Run(argsJSON string) (string, error) {
	if t.Manager == nil {
		return "", fmt.Errorf("SkillManager is not initialized")
	}

	fmt.Println("🔄 [ReloadSkills] Triggering skill reload...")
	if err := t.Manager.Reload(); err != nil {
		return fmt.Sprintf("❌ Reload failed: %v", err), nil
	}

	return "✅ Skills reloaded successfully! New commands should be available now.", nil
}
