package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/asccclass/pcai/skills"
	"github.com/ollama/ollama/api"
)

type SkillInstaller struct {
	Manager *SkillManager
	BaseDir string
}

func (i *SkillInstaller) Name() string { return "install_github_skill" }

func (i *SkillInstaller) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        i.Name(),
			Description: "從 GitHub 網址下載並安裝新的 AI 技能 (支援 Sidecar Docker 執行)",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"repo_url": {
					   "type": "string", 
					   "description": "GitHub 倉庫網址"
					},
					"skill_name": {
					   "type": "string", 
					   "description": "技能資料夾名稱 (將安裝到 skills/skill_name)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"repo_url", "skill_name"},
				}
			}(),
		},
	}
}

func (i *SkillInstaller) Run(argsJSON string) (string, error) {
	var args struct {
		RepoURL   string `json:"repo_url"`
		SkillName string `json:"skill_name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("參數解析失敗: %v", err)
	}

	// 1. Git Clone
	// 確保 BaseDir 存在
	if err := os.MkdirAll(i.BaseDir, 0755); err != nil {
		return "", fmt.Errorf("無法建立技能目錄: %v", err)
	}

	targetPath := filepath.Join(i.BaseDir, args.SkillName)
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		return "", fmt.Errorf("技能目錄已存在: %s", targetPath)
	}

	// 執行 git clone
	cmd := exec.Command("git", "clone", args.RepoURL, targetPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("下載失敗: %v\nOutput: %s", err, output)
	}

	// 2. 讀取 skill.json (優先) 或 SKILL.md
	// 定義 skill.json 結構
	var config struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Command     string          `json:"command"`
		Image       string          `json:"image"` // 支援 Docker Image
		Parameters  json.RawMessage `json:"parameters"`
	}

	var def *skills.SkillDefinition

	configPath := filepath.Join(targetPath, "skill.json")
	if _, err := os.Stat(configPath); err == nil {
		// 讀取 skill.json
		configData, err := os.ReadFile(configPath)
		if err != nil {
			return "", fmt.Errorf("讀取 skill.json 失敗: %v", err)
		}
		if err := json.Unmarshal(configData, &config); err != nil {
			return "", fmt.Errorf("解析 skill.json 失敗: %v", err)
		}

		// 建構各定義
		def = &skills.SkillDefinition{
			Name:        config.Name,
			Description: config.Description,
			Command:     config.Command,
			Image:       config.Image,
			RepoPath:    targetPath,
		}

		// 解析參數
		def.Params = skills.ParseParams(def.Command)

	} else {
		// 嘗試載入 SKILL.md
		loadedSkills, err := skills.LoadSkills(targetPath)
		if err != nil || len(loadedSkills) == 0 {
			return "", fmt.Errorf("安裝成功但無法在 %s 找到有效的技能定義 (skill.json or SKILL.md)", targetPath)
		}
		def = loadedSkills[0] // 取第一個
	}

	// 4. Register & Persist
	dynamicTool := skills.NewDynamicTool(def, i.Manager.Registry, i.Manager.DockerClient)
	i.Manager.Registry.Register(dynamicTool)

	// 5. 寫入持久化清單
	if err := i.Manager.RegisterSkill(def.Name, targetPath); err != nil {
		fmt.Printf("⚠️ [SkillInstaller] 持久化失敗: %v\n", err)
	}

	return fmt.Sprintf("成功安裝技能: %s (Repo: %s)。你現在可以開始使用它了。", def.Name, args.RepoURL), nil
}
