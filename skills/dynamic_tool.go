// skills/dynamic_tool.go 應該移到 internal/skillloader 目錄下
package skills

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asccclass/pcai/internal/core"
	"github.com/ollama/ollama/api"
	"gopkg.in/yaml.v3"
)

// SkillDefinition 代表從 Markdown 解析出來的技能
type SkillDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Command     string   `yaml:"command"`
	Params      []string `yaml:"-"` // 從 Command 解析出的參數參數名 (e.g. "query", "args")
}

// loadSkillFromFile 解析單一 SKILL.md 檔案
func loadSkillFromFile(path string) (*SkillDefinition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 解析 Frontmatter
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid frontmatter format")
	}

	yamlContent := parts[1]
	var skill SkillDefinition
	if err := yaml.Unmarshal([]byte(yamlContent), &skill); err != nil {
		return nil, fmt.Errorf("yaml parse error: %v", err)
	}

	// 解析參數
	skill.Params = parseParams(skill.Command)
	return &skill, nil
}

// LoadSkills 從指定目錄載入所有技能定義 (Clawcode 標準: SKILL.md)
func LoadSkills(dir string) ([]*SkillDefinition, error) {
	var skills []*SkillDefinition

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "SKILL.md") {
			skill, err := loadSkillFromFile(path)
			if err != nil {
				fmt.Printf("⚠️ [Skills] Warning: Failed to load skill from %s: %v\n", path, err)
				return nil // 繼續載入其他技能
			}
			skills = append(skills, skill)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return skills, nil
}

// parseParams 解析 {{param}} 形式的參數
func parseParams(cmd string) []string {
	// 正則表達式：匹配 {{param}}
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
	Def      *SkillDefinition
	Registry *core.Registry
}

func NewDynamicTool(def *SkillDefinition, registry *core.Registry) *DynamicTool {
	return &DynamicTool{
		Def:      def,
		Registry: registry,
	}
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

	// 3. 判斷是否為內部工具呼叫
	// 簡單啟發式：取得第一個單詞作為工具名稱
	parts := strings.SplitN(finalCmd, " ", 2)
	toolName := parts[0]
	toolArgs := ""
	if len(parts) > 1 {
		toolArgs = parts[1]
	}

	// 嘗試從 Registry 查找工具
	// 注意：我們需要 access 到 registry，這需要從外部注入
	if t.Registry != nil {
		// ALIAS: http_get -> fetch_url
		// Also support direct usage of 'fetch_url' as an internal tool
		if toolName == "http_get" || toolName == "fetch_url" {
			// 去除引號，並組裝 JSON
			url := strings.Trim(toolArgs, "\"'")
			jsonParams := fmt.Sprintf(`{"url": "%s"}`, url)
			return t.Registry.CallTool("fetch_url", jsonParams)
		}
	}

	// 4. 背景執行 (Fallback to Shell)
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
