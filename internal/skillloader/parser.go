package skillloader

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// ParseMarkdownSkill 讀取單個 MD 檔案並解析
func ParseMarkdownSkill(filePath string) (*SkillDoc, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	skill := &SkillDoc{}
	scanner := bufio.NewScanner(file)

	// 正則表達式：匹配參數行 "- param_name: (type, required) description"
	paramRegex := regexp.MustCompile(`^-\s+(\w+):\s*\(([^,]+)(?:,\s*(required|optional))?\)\s*(.*)`)

	parsingParams := false
	var descBuilder strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 解析標題 (# Name)
		if strings.HasPrefix(line, "# ") {
			skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}

		// 偵測是否進入參數區塊
		if strings.HasPrefix(line, "## Parameters") {
			parsingParams = true
			continue
		}

		if parsingParams {
			// 解析參數
			if matches := paramRegex.FindStringSubmatch(line); matches != nil {
				// matches[1]: Name, matches[2]: Type, matches[3]: Required/Optional, matches[4]: Description
				isRequired := strings.ToLower(matches[3]) == "required"

				skill.Parameters = append(skill.Parameters, ParamDoc{
					Name:        matches[1],
					Type:        strings.ToLower(matches[2]),
					Required:    isRequired,
					Description: matches[4],
				})
			}
		} else {
			// 解析描述 (如果在標題之後，參數區塊之前)
			if skill.Name != "" && !strings.HasPrefix(line, "#") {
				if descBuilder.Len() > 0 {
					descBuilder.WriteString(" ")
				}
				descBuilder.WriteString(line)
			}
		}
	}

	skill.Description = strings.TrimSpace(descBuilder.String())
	return skill, scanner.Err()
}
