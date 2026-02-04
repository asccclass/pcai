package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/utils"
	"github.com/ollama/ollama/api"
)

type CreateSkillTool struct{}

func (t *CreateSkillTool) Name() string { return "create_new_skill" }

func (t *CreateSkillTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "create_new_skill",
			Description: "當判定功能為 Skill 時，自動產生 Go 代碼腳手架。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"skill_name": {
						"type": "string",
						"description": "技能的英文名稱 (例如: Scheduler)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"skill_name"},
				}
			}(),
		},
	}
}

func (t *CreateSkillTool) Run(argsJSON string) (string, error) {
	var args struct {
		SkillName string `json:"skill_name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("無效的參數: %v", err)
	}

	name := args.SkillName
	if name == "" {
		return "", fmt.Errorf("skill_name 不能為空")
	}

	err := utils.GenerateSkillScaffold(name)
	if err != nil {
		return fmt.Sprintf("產生失敗: %v", err), nil
	}
	return fmt.Sprintf("已自動建立 skills/%s 檔案，請開始實作具體業務邏輯。", strings.ToLower(name)), nil
}
