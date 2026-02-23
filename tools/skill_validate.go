package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ollama/ollama/api"
)

// SkillValidateTool 驗證 Skill 目錄是否符合 agentskills.io 規格
type SkillValidateTool struct {
	SkillsDir string // skills/ 根目錄
}

func (t *SkillValidateTool) Name() string  { return "skill_validate" }
func (t *SkillValidateTool) IsSkill() bool { return false }

func (t *SkillValidateTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: "驗證指定的 AI 技能目錄是否符合 agentskills.io 規格。檢查 SKILL.md 存在性、YAML Frontmatter 格式、必要欄位等。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"skill_name": {
						"type": "string",
						"description": "要驗證的技能名稱 (skills/ 下的目錄名稱，如 weather、read_email)"
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

func (t *SkillValidateTool) Run(argsJSON string) (string, error) {
	var args struct {
		SkillName string `json:"skill_name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("參數解析失敗: %v", err)
	}

	if args.SkillName == "" {
		return "", fmt.Errorf("skill_name 不可為空")
	}

	skillDir := filepath.Join(t.SkillsDir, args.SkillName)

	// 執行驗證
	result := t.validate(skillDir, args.SkillName)
	return result, nil
}

// validate 執行完整驗證流程
func (t *SkillValidateTool) validate(skillDir, skillName string) string {
	var sb strings.Builder
	errors := 0

	sb.WriteString(fmt.Sprintf("🔍 驗證 Skill: %s\n", skillName))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. 檢查目錄是否存在
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		sb.WriteString(fmt.Sprintf("❌ 目錄不存在: %s\n", skillDir))
		return sb.String()
	}

	// 2. 檢查 SKILL.md 是否存在
	skillMdPath := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(skillMdPath)
	if err != nil {
		sb.WriteString("❌ [必要] 缺少 SKILL.md\n")
		errors++
	} else {
		sb.WriteString("✅ SKILL.md 存在\n")
		content := string(data)

		// 3. 解析 YAML Frontmatter
		parts := strings.SplitN(content, "---", 3)
		if len(parts) < 3 {
			sb.WriteString("❌ [必要] SKILL.md 缺少 YAML Frontmatter (需以 --- 開頭和結尾)\n")
			errors++
		} else {
			sb.WriteString("✅ YAML Frontmatter 格式正確\n")
			frontmatter := parts[1]

			// 4. 檢查 name 欄位
			name := extractField(frontmatter, "name")
			if name != "" {
				sb.WriteString(fmt.Sprintf("✅ name: %s\n", name))
			} else {
				sb.WriteString("❌ [必要] 缺少 name 欄位\n")
				errors++
			}

			// 5. 檢查 description 欄位
			desc := extractField(frontmatter, "description")
			if desc != "" {
				sb.WriteString("✅ description 欄位已填寫\n")
			} else {
				sb.WriteString("❌ [必要] 缺少 description 欄位\n")
				errors++
			}

			// 6. 檢查 command 欄位 (選填)
			cmd := extractField(frontmatter, "command")
			if cmd != "" {
				sb.WriteString(fmt.Sprintf("✅ command: %s\n", cmd))

				// 檢查參數格式
				params := extractParams(cmd)
				if len(params) > 0 {
					sb.WriteString(fmt.Sprintf("   📋 偵測到參數: %s\n", strings.Join(params, ", ")))
				}
			} else {
				sb.WriteString("ℹ️  無 command 欄位 (context-only 技能)\n")
			}

			// 檢查其他選填欄位
			if extractField(frontmatter, "cache_duration") != "" {
				sb.WriteString("✅ cache_duration 已設定\n")
			}
			if extractField(frontmatter, "image") != "" {
				sb.WriteString("✅ image (Docker) 已設定\n")
			}
		}
	}

	// 7. 檢查選填目錄
	for _, subdir := range []string{"scripts", "templates", "references"} {
		subdirPath := filepath.Join(skillDir, subdir)
		if info, err := os.Stat(subdirPath); err == nil && info.IsDir() {
			fileCount := countFiles(subdirPath)
			sb.WriteString(fmt.Sprintf("✅ %s/ 目錄存在 (%d 個檔案)\n", subdir, fileCount))
		} else {
			sb.WriteString(fmt.Sprintf("ℹ️  無 %s/ 目錄 (選填)\n", subdir))
		}
	}

	// 結果摘要
	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	if errors == 0 {
		sb.WriteString("✅ 驗證通過！Skill 符合 agentskills.io 規格。\n")
	} else {
		sb.WriteString(fmt.Sprintf("❌ 驗證失敗：發現 %d 個錯誤。\n", errors))
	}

	return sb.String()
}

// extractField 從 YAML frontmatter 中提取欄位值
func extractField(frontmatter, fieldName string) string {
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		prefix := fieldName + ":"
		if strings.HasPrefix(trimmed, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			// 處理多行值 (如 description: > 格式)
			if value == ">" || value == "|" {
				return "(multiline)" // 存在但為多行格式
			}
			return value
		}
	}
	return ""
}

// extractParams 從 command 中提取 {{param}} 格式的參數
func extractParams(command string) []string {
	var params []string
	remaining := command
	for {
		start := strings.Index(remaining, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(remaining[start:], "}}")
		if end == -1 {
			break
		}
		params = append(params, remaining[start:start+end+2])
		remaining = remaining[start+end+2:]
	}
	return params
}

// countFiles 計算目錄中的檔案數量 (遞迴)
func countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}
