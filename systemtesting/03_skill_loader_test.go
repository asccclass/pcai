package systemtesting

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/asccclass/pcai/internal/skillloader"
)

// ============================================================
// Stage 3: Skill Loader — SKILL.md 解析與參數提取
// 測試 ParseParams / LoadSkills / loadSkillFromFile
// ============================================================

// --- ParseParams ---

func TestParseParams_Basic(t *testing.T) {
	cmd := "gog calendar events --all --from {{from}} --to {{to}} --json"
	params := skillloader.ParseParams(cmd)

	if len(params) != 2 {
		t.Fatalf("Expected 2 params, got %d: %v", len(params), params)
	}

	expected := map[string]bool{"from": true, "to": true}
	for _, p := range params {
		if !expected[p] {
			t.Errorf("Unexpected param: %q", p)
		}
	}
}

func TestParseParams_URLEncoded(t *testing.T) {
	cmd := "web_fetch https://example.com/api?q={{url:query}}"
	params := skillloader.ParseParams(cmd)

	if len(params) != 1 || params[0] != "query" {
		t.Errorf("Expected [query], got %v", params)
	}
}

func TestParseParams_DuplicateParams(t *testing.T) {
	cmd := "echo {{name}} hello {{name}}"
	params := skillloader.ParseParams(cmd)

	if len(params) != 1 {
		t.Errorf("Expected 1 unique param, got %d: %v", len(params), params)
	}
}

func TestParseParams_NoParams(t *testing.T) {
	cmd := "echo hello world"
	params := skillloader.ParseParams(cmd)

	if len(params) != 0 {
		t.Errorf("Expected 0 params, got %d: %v", len(params), params)
	}
}

func TestParseParams_MultipleWithPrefix(t *testing.T) {
	cmd := "fetch {{url:location}} --output {{file}}"
	params := skillloader.ParseParams(cmd)

	if len(params) != 2 {
		t.Fatalf("Expected 2 params, got %d: %v", len(params), params)
	}

	expected := map[string]bool{"location": true, "file": true}
	for _, p := range params {
		if !expected[p] {
			t.Errorf("Unexpected param: %q", p)
		}
	}
}

// --- LoadSkills (從實際目錄) ---

func TestLoadSkills_ReadCalendarsExists(t *testing.T) {
	// 找到專案根目錄
	cwd, _ := os.Getwd()
	skillsDir := filepath.Join(cwd, "..", "skills")

	loadedSkills, err := skillloader.LoadSkills(skillsDir)
	if err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	// 應該至少載入 read_calendars
	found := false
	for _, s := range loadedSkills {
		if s.Name == "read_calendars" {
			found = true
			// 驗證指令模板
			if s.Command == "" {
				t.Error("read_calendars command should not be empty")
			}
			// 驗證參數
			if len(s.Params) < 2 {
				t.Errorf("read_calendars should have at least 2 params (from, to), got %d: %v", len(s.Params), s.Params)
			}
			break
		}
	}
	if !found {
		t.Error("read_calendars skill not found in loaded skills")
	}
}

// --- LoadSkills (無效目錄) ---

func TestLoadSkills_InvalidDirectory(t *testing.T) {
	_, err := skillloader.LoadSkills("/nonexistent/path/to/skills")
	if err == nil {
		t.Error("Expected error for nonexistent directory, got nil")
	}
}

// --- LoadSkills (建立暫時 SKILL.md 測試) ---

func TestLoadSkills_TempSkill(t *testing.T) {
	// 建立臨時目錄和 SKILL.md
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test_skill")
	os.MkdirAll(skillDir, 0755)

	skillContent := `---
name: test_hello
description: A test skill
command: echo {{message}}
---
# Test Skill
This is a test.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644)

	loadedSkills, err := skillloader.LoadSkills(tmpDir)
	if err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	if len(loadedSkills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(loadedSkills))
	}

	s := loadedSkills[0]
	if s.Name != "test_hello" {
		t.Errorf("Expected name 'test_hello', got %q", s.Name)
	}
	if len(s.Params) != 1 || s.Params[0] != "message" {
		t.Errorf("Expected params [message], got %v", s.Params)
	}
}

func TestLoadSkills_InvalidFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "bad_skill")
	os.MkdirAll(skillDir, 0755)

	// 缺少 frontmatter
	badContent := `# No Frontmatter
Just a plain markdown file.
`
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(badContent), 0644)

	loadedSkills, err := skillloader.LoadSkills(tmpDir)
	if err != nil {
		t.Fatalf("LoadSkills should not fail entirely, got: %v", err)
	}
	// 應該跳過無效的 SKILL.md
	if len(loadedSkills) != 0 {
		t.Errorf("Expected 0 valid skills from invalid frontmatter, got %d", len(loadedSkills))
	}
}
