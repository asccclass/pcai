package skillloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GenerateSnapshot 掃描 skillsDir 下所有 SKILL.md，產生 <available_skills> XML 字串
func GenerateSnapshot(skillsDir string) (string, error) {
	skills, err := LoadSkills(skillsDir)
	if err != nil {
		return "", fmt.Errorf("掃描技能失敗: %w", err)
	}

	if len(skills) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	for _, s := range skills {
		// 取得 SKILL.md 的相對位置
		location := filepath.Join(s.RepoPath, "SKILL.md")

		sb.WriteString("  <skill>\n")
		sb.WriteString(fmt.Sprintf("    <name>%s</name>\n", s.Name))
		sb.WriteString(fmt.Sprintf("    <description>%s</description>\n", strings.TrimSpace(s.Description)))
		sb.WriteString(fmt.Sprintf("    <location>%s</location>\n", location))
		sb.WriteString("  </skill>\n")
	}
	sb.WriteString("</available_skills>")

	return sb.String(), nil
}

// GenerateAndSaveSnapshot 產生快照並寫入 skillsDir/skills_snapshot.md
func GenerateAndSaveSnapshot(skillsDir string) (string, error) {
	snapshot, err := GenerateSnapshot(skillsDir)
	if err != nil {
		return "", err
	}
	if snapshot == "" {
		return "", nil
	}

	// 寫入檔案
	outPath := filepath.Join(skillsDir, "skills_snapshot.md")
	if err := os.WriteFile(outPath, []byte(snapshot), 0644); err != nil {
		return snapshot, fmt.Errorf("寫入 %s 失敗: %w", outPath, err)
	}

	fmt.Printf("📋 [Skills] 已產生技能快照: %s (%d 個技能)\n", outPath, strings.Count(snapshot, "<skill>"))
	return snapshot, nil
}
