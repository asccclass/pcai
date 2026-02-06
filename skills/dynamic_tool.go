package skills

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ollama/ollama/api"
)

// SkillDefinition 代表從 Markdown 解析出來的技能
type SkillDefinition struct {
	Name        string
	Description string
	Command     string
	Params      []string // 從 Command 解析出的參數參數名 (e.g. "query", "args")
}

// LoadSkills 從指定 Markdown 檔案載入技能定義
func LoadSkills(path string) ([]*SkillDefinition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var skills []*SkillDefinition
	var currentSkill *SkillDefinition

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 解析標題 "## SkillName"
		if strings.HasPrefix(line, "## ") {
			if currentSkill != nil {
				skills = append(skills, currentSkill)
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			currentSkill = &SkillDefinition{
				Name: name,
			}
		} else if currentSkill != nil {
			// 解析 Description
			if strings.HasPrefix(line, "Description:") {
				desc := strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
				currentSkill.Description = desc
			}
			// 解析 Command
			if strings.HasPrefix(line, "Command:") {
				cmd := strings.TrimSpace(strings.TrimPrefix(line, "Command:"))
				currentSkill.Command = cmd
				currentSkill.Params = parseParams(cmd)
			}
		}
	}
	// 加入最後一個
	if currentSkill != nil {
		skills = append(skills, currentSkill)
	}

	return skills, nil
}

// parseParams 解析 {{param}} 形式的參數
func parseParams(cmd string) []string {
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(cmd, -1)
	var params []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			p := m[1]
			if !seen[p] {
				params = append(params, p)
				seen[p] = true
			}
		}
	}
	return params
}

// DynamicTool 實作 core.AgentTool 介面
type DynamicTool struct {
	Def *SkillDefinition
}

func NewDynamicTool(def *SkillDefinition) *DynamicTool {
	return &DynamicTool{Def: def}
}

func (t *DynamicTool) Name() string {
	// 將名稱轉為 snake_case 符合工具命名慣例 (e.g. GoogleSearch -> google_search)
	// 這裡簡單轉小寫並把空白換底線
	return strings.ToLower(strings.ReplaceAll(t.Def.Name, " ", "_"))
}

func (t *DynamicTool) Definition() api.Tool {
	// 重新建構 Properties map
	propsMap := make(map[string]interface{})
	required := []string{}

	for _, p := range t.Def.Params {
		propsMap[p] = map[string]string{
			"type":        "string",
			"description": fmt.Sprintf("Parameter %s for command", p),
		}
		required = append(required, p)
	}

	// 透過 JSON轉換 來產生 api.ToolPropertiesMap，避免內部型別不一致的問題
	var apiProps api.ToolPropertiesMap
	propsBytes, _ := json.Marshal(propsMap)
	_ = json.Unmarshal(propsBytes, &apiProps)

	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: t.Def.Description,
			Parameters: api.ToolFunctionParameters{
				Type:       "object",
				Properties: &apiProps,
				Required:   required,
			},
		},
	}
}

func (t *DynamicTool) Run(argsJSON string) (string, error) {
	// 1. 解析參數
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 2. 替換指令中的變數
	finalCmd := t.Def.Command
	for k, v := range args {
		valStr := fmt.Sprintf("%v", v)
		placeholder := fmt.Sprintf("{{%s}}", k)
		finalCmd = strings.ReplaceAll(finalCmd, placeholder, valStr)
	}

	// 3. 背景執行
	// 使用開頭的字作為執行檔，後面的作為參數 (需要簡單的 split，不支援複雜的 
	// quote 處理)
	// 為了支援 shell features (如 &&, |)，我們統一使用 sh -c (Linux) 或 
	// cmd /c (Windows)
	// 根據 User OS (Windows)，使用 cmd /c

	// 注意：這裡直接執行可能會有安全風險 (Command Injection)，
	// 但基於 User Request 為個人助理，暫時允許。

	go func(commandStr string) {
		// Detect OS logic if needed, simplify to Windows PowerShell or Cmd
		cmd := exec.Command("cmd", "/C", commandStr)
		// 也可以試試 PowerShell: exec.Command("powershell", "-Command", commandStr)

		out, err := cmd.CombinedOutput()
		output := string(out)
		if err != nil {
			output += fmt.Sprintf("\nErrors: %v", err)
		}

		// 這裡可以考慮將結果寫回 Log 或通知使用者 (透過 Signal 或其他機制)
		// 目前先 Println
		fmt.Printf("[Background Task %s] Result:\n%s\n", t.Name(), output)
	}(finalCmd)

	return fmt.Sprintf("已於背景啟動指令: %s", finalCmd), nil
}
