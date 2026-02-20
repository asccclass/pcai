package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/skillloader"
	dclient "github.com/docker/docker/client"
)

// SkillEntry 定義在 registry.json 中的結構
type SkillEntry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	InstalledAt time.Time `json:"installed_at"`
}

// SkillRegistry 定義 JSON 檔案的根結構
type SkillRegistry struct {
	InstalledSkills []SkillEntry `json:"installed_skills"`
}

// SkillManager 負責管理已安裝技能的持久化與載入
type SkillManager struct {
	BaseDir      string
	DBPath       string
	Registry     *core.Registry
	DockerClient *dclient.Client
}

// NewSkillManager 建立 SkillManager 實例
func NewSkillManager(baseDir, dbPath string, registry *core.Registry, dockerCli *dclient.Client) *SkillManager {
	return &SkillManager{
		BaseDir:      baseDir,
		DBPath:       dbPath,
		Registry:     registry,
		DockerClient: dockerCli,
	}
}

// LoadAll 從磁碟載入所有已安裝的技能
func (m *SkillManager) LoadAll() error {
	data, err := os.ReadFile(m.DBPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 檔案不存在，視為無已安裝技能
		}
		return fmt.Errorf("讀取 registry 失敗: %v", err)
	}

	var registry SkillRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return fmt.Errorf("解析 registry 失敗: %v", err)
	}

	fmt.Printf("📦 [SkillManager] Found %d installed skills in registry.\n", len(registry.InstalledSkills))

	for _, s := range registry.InstalledSkills {
		// 確保路徑是絕對路徑或相對於 BaseDir
		// 如果是 ./skills/xxx，則解析為絕對路徑
		// 簡單起見，我們假設 s.Path 是正確的可存取路徑

		// 恢復技能
		if err := m.restoreSkill(s.Path); err != nil {
			fmt.Printf("⚠️ [SkillManager] 載入技能 %s (%s) 失敗: %v\n", s.Name, s.Path, err)
		} else {
			fmt.Printf("✅ [SkillManager] 已載入技能: %s\n", s.Name)
		}
	}
	return nil
}

// RegisterSkill 記錄新安裝的技能並寫入檔案
func (m *SkillManager) RegisterSkill(name, path string) error {
	// 1. 讀取現有
	var registry SkillRegistry
	data, err := os.ReadFile(m.DBPath)
	if err == nil {
		_ = json.Unmarshal(data, &registry)
	}

	// 2. 檢查是否已存在，若存在則更新，否則新增
	found := false
	for i, s := range registry.InstalledSkills {
		if s.Name == name {
			registry.InstalledSkills[i].Path = path
			registry.InstalledSkills[i].InstalledAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		registry.InstalledSkills = append(registry.InstalledSkills, SkillEntry{
			Name:        name,
			Path:        path,
			InstalledAt: time.Now(),
		})
	}

	// 3. 寫回檔案
	newData, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}

	// 確保目錄存在
	if err := os.MkdirAll(filepath.Dir(m.DBPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(m.DBPath, newData, 0644)
}

// LoadLocalSkills 掃描指定目錄載入 SKILL.md (向下相容)
func (m *SkillManager) LoadLocalSkills(dir string) error {
	dynamicSkills, err := skillloader.LoadSkills(dir)
	if err != nil {
		return fmt.Errorf("載入本地技能失敗: %v", err)
	}

	count := 0
	for _, ds := range dynamicSkills {
		toolStr := skillloader.NewDynamicTool(ds, m.Registry, m.DockerClient)
		m.Registry.RegisterWithPriority(toolStr, 10) // Skills 優先於 Tools
		fmt.Printf("✅ [SkillManager] Loaded local skill: %s (%s)\n", ds.Name, ds.Description)
		count++
	}
	fmt.Printf("📂 [SkillManager] Loaded %d local skills from %s\n", count, dir)
	return nil
}

// Reload 重新載入所有技能 (Registry + Local)
func (m *SkillManager) Reload() error {
	fmt.Println("🔄 [SkillManager] Reloading skillloader...")

	// 1. Reload from Registry (Persistent)
	if err := m.LoadAll(); err != nil {
		return err
	}

	// 2. Reload local skills (from BaseDir)
	if err := m.LoadLocalSkills(m.BaseDir); err != nil {
		return err
	}

	return nil
}

// restoreSkill 負責載入並註冊單個技能
func (m *SkillManager) restoreSkill(path string) error {
	// 邏輯類似 SkillInstaller 的載入部分
	// 嘗試讀取 skill.json 或 SKILL.md

	// 1. 嘗試載入 SKILL.md (使用 existing shared logic)
	// 此邏輯同時支援 skill.json 如果我們之前的 SkillInstaller 實作正確轉換了它
	// 但 SkillInstaller 目前是 "安裝時轉換"。
	// 如果安裝後的目錄結構包含 skill.json，我們需要再讀一次。

	// 優化：統一使用 `skillloader.LoadSkills`。
	// 但 `skillloader.LoadSkills` 目前只讀 `SKILL.md`。
	// 如果 `SkillInstaller` 在安裝時產生了 `SKILL.md`，那就完美了。
	// 如果 `SkillInstaller` 只是保留原樣 (可能只有 skill.json)，那我們需要在這裡處理 skill.json。

	// 為了穩健，我們在這裡複製 SkillInstaller 的讀取邏輯，或者重構 `skills` package 支援 skill.json。
	// 鑑於 `skills` 是獨立模組，我們在 `tools` 層處理 `skill.json`。

	var def *skillloader.SkillDefinition

	configPath := filepath.Join(path, "skill.json")
	if _, err := os.Stat(configPath); err == nil {
		// 讀取 skill.json
		configData, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("讀取 skill.json 失敗: %v", err)
		}

		var config struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Command     string `json:"command"`
			Image       string `json:"image"`
		}
		if err := json.Unmarshal(configData, &config); err != nil {
			return fmt.Errorf("解析 skill.json 失敗: %v", err)
		}

		def = &skillloader.SkillDefinition{
			Name:        config.Name,
			Description: config.Description,
			Command:     config.Command,
			Image:       config.Image,
			RepoPath:    path,
		}
		def.Params = skillloader.ParseParams(def.Command)

	} else {
		// 嘗試載入 SKILL.md
		loadedSkills, err := skillloader.LoadSkills(path)
		if err != nil || len(loadedSkills) == 0 {
			return fmt.Errorf("目錄 %s 無效的技能定義", path)
		}
		def = loadedSkills[0]
	}

	// 註冊
	dynamicTool := skillloader.NewDynamicTool(def, m.Registry, m.DockerClient)
	m.Registry.RegisterWithPriority(dynamicTool, 10) // Skills 優先於 Tools

	return nil
}
