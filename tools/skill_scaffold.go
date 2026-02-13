package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
)

// SkillScaffoldTool 產生新 Skill 骨架目錄
type SkillScaffoldTool struct {
	SkillsDir string // skills/ 根目錄
}

func (t *SkillScaffoldTool) Name() string { return "skill_scaffold" }

func (t *SkillScaffoldTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: "建立新的 AI 技能骨架目錄，包含 SKILL.md 範本與標準子目錄結構。使用者說「建立新技能」或「新增 Skill」時使用。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"skill_name": {
						"type": "string",
						"description": "技能名稱 (snake_case 格式，如 my_skill)，將作為目錄名稱建立在 skills/ 下"
					},
					"description": {
						"type": "string",
						"description": "技能的功能描述，會寫入 SKILL.md 的 description 欄位"
					},
					"command": {
						"type": "string",
						"description": "技能要執行的指令 (選填)，支援 {{param}} 參數佔位符。若省略則建立 context-only 技能"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"skill_name", "description"},
				}
			}(),
		},
	}
}

func (t *SkillScaffoldTool) Run(argsJSON string) (string, error) {
	var args struct {
		SkillName   string `json:"skill_name"`
		Description string `json:"description"`
		Command     string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("參數解析失敗: %v", err)
	}

	if args.SkillName == "" {
		return "", fmt.Errorf("skill_name 不可為空")
	}

	targetDir := filepath.Join(t.SkillsDir, args.SkillName)

	// 檢查目錄是否已存在
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		return "", fmt.Errorf("技能目錄已存在: %s", targetDir)
	}

	// 建立目錄結構
	subdirs := []string{"scripts", "templates", "references"}
	for _, sub := range subdirs {
		if err := os.MkdirAll(filepath.Join(targetDir, sub), 0755); err != nil {
			return "", fmt.Errorf("無法建立目錄 %s: %v", sub, err)
		}
	}

	// 嘗試從範本產生 SKILL.md
	content := t.generateSkillMD(args.SkillName, args.Description, args.Command)

	skillMdPath := filepath.Join(targetDir, "SKILL.md")
	if err := os.WriteFile(skillMdPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("無法寫入 SKILL.md: %v", err)
	}

	// 回傳結果
	result := fmt.Sprintf("✅ 已建立技能骨架: %s\n\n", args.SkillName)
	result += "📁 目錄結構:\n"
	result += "   " + args.SkillName + "/\n"
	result += "   ├── SKILL.md\n"
	result += "   ├── scripts/\n"
	result += "   ├── templates/\n"
	result += "   └── references/\n\n"
	result += "📝 請編輯 " + skillMdPath + " 完成技能定義。"

	return result, nil
}

// generateSkillMD 產生 SKILL.md 內容
func (t *SkillScaffoldTool) generateSkillMD(name, description, command string) string {
	// 嘗試讀取 skill-creator 的範本
	templatePath := filepath.Join(t.SkillsDir, "skill-creator", "templates", "SKILL_TEMPLATE.md")
	if data, err := os.ReadFile(templatePath); err == nil {
		content := string(data)
		content = strings.ReplaceAll(content, "{{SKILL_NAME}}", name)
		content = strings.ReplaceAll(content, "{{DESCRIPTION}}", description)
		if command != "" {
			// 替換範本中的 command 行
			content = strings.Replace(content, `command: echo "TODO: 請替換為實際指令 {{param_name}}"`, "command: "+command, 1)
		}
		return content
	}

	// 若範本不存在，使用內建最小結構
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	sb.WriteString(fmt.Sprintf("description: %s\n", description))
	if command != "" {
		sb.WriteString(fmt.Sprintf("command: %s\n", command))
	}
	sb.WriteString("---\n\n")
	sb.WriteString(fmt.Sprintf("# %s\n\n", name))
	sb.WriteString(fmt.Sprintf("%s\n\n", description))
	sb.WriteString("## Purpose\n\n請說明何時應該使用這個技能。\n\n")
	sb.WriteString("## Steps\n\n1. TODO\n\n")
	sb.WriteString("## Output Format\n\n請說明輸出格式。\n\n")
	sb.WriteString("## Examples\n\nTODO\n")

	return sb.String()
}
