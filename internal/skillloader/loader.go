// internal/skillloader/loader.go 目前沒有使用到，使用 skills/dynamic_tool.go
package skillloader

import (
	"fmt"
	"path/filepath"
)

// ConvertToOpenAISchema 將內部結構轉換為 OpenAI 格式
func ConvertToOpenAISchema(skill *SkillDoc) OpenAITool {
	tool := OpenAITool{
		Type: "function",
		Function: OpenAIFuncDefinition{
			Name:        skill.Name,
			Description: skill.Description,
			Parameters: JSONSchemaDefinition{
				Type:       "object",
				Properties: make(map[string]JSONProperty),
				Required:   []string{},
			},
		},
	}

	for _, param := range skill.Parameters {
		tool.Function.Parameters.Properties[param.Name] = JSONProperty{
			Type:        param.Type,
			Description: param.Description,
		}
		if param.Required {
			tool.Function.Parameters.Required = append(tool.Function.Parameters.Required, param.Name)
		}
	}

	return tool
}

// LoadSkills 掃描指定目錄下的所有 .md 檔案並回傳 Tools 列表
func LoadSkills(dirPath, llmName string) ([]OpenAITool, error) {
	// 1. 掃描所有 .md 檔案
	files, err := filepath.Glob(filepath.Join(dirPath, "*.md"))
	if err != nil {
		return nil, err
	}

	var tools []OpenAITool

	// 2. 迴圈處理每個檔案
	for _, file := range files {
		// A. 解析 Markdown
		skillDoc, err := ParseMarkdownSkill(file)
		if err != nil {
			fmt.Printf("警告: 無法解析檔案 %s: %v\n", file, err)
			continue
		}

		// B. 轉換 Schema
		tool := ConvertToOpenAISchema(skillDoc)
		tools = append(tools, tool)

		fmt.Printf("✅ 已載入 Skill: %s (%s)\n", skillDoc.Name, file)
	}

	return tools, nil
}
