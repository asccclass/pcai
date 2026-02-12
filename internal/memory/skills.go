package memory

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// MemorySkill defines the structure of a memory extraction skill
type MemorySkill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
	Trigger     struct {
		Type     string   `yaml:"type"`     // "keyword", "embedding", "always"
		Keywords []string `yaml:"keywords"` // For keyword trigger
	} `yaml:"trigger"`
	Template string `yaml:"template"` // The prompt template for LLM
}

// SkillManager manages loading and selecting memory skills
type SkillManager struct {
	SkillsDir string
	Skills    []MemorySkill
}

// NewSkillManager creates a new SkillManager
func NewSkillManager(dir string) *SkillManager {
	return &SkillManager{
		SkillsDir: dir,
		Skills:    []MemorySkill{},
	}
}

// LoadSkills reads all .yaml files from the skills directory
func (sm *SkillManager) LoadSkills() error {
	files, err := os.ReadDir(sm.SkillsDir)
	if err != nil {
		return fmt.Errorf("failed to read skills dir: %w", err)
	}

	sm.Skills = []MemorySkill{} // Clear existing

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".yaml" || filepath.Ext(file.Name()) == ".yml" {
			path := filepath.Join(sm.SkillsDir, file.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Printf("⚠️ Failed to read skill file %s: %v\n", file.Name(), err)
				continue
			}

			var skill MemorySkill
			if err := yaml.Unmarshal(data, &skill); err != nil {
				fmt.Printf("⚠️ Failed to parse skill file %s: %v\n", file.Name(), err)
				continue
			}
			sm.Skills = append(sm.Skills, skill)
			fmt.Printf("🧠 Loaded Memory Skill: %s (v%s)\n", skill.Name, skill.Version)
		}
	}
	return nil
}

// FindMatchingSkills identifies skills to trigger based on input text
func (sm *SkillManager) FindMatchingSkills(text string) []MemorySkill {
	var matches []MemorySkill

	// Simple keyword matching for MVP
	// In the future, this can be replaced with embedding search (The Controller)
	for _, skill := range sm.Skills {
		if skill.Trigger.Type == "keyword" {
			for _, _ = range skill.Trigger.Keywords {
				// TODO: Case-insensitive check
				// For now, simple Contains
				// To be robust, we should use regex or strings.Contains(strings.ToLower(text), strings.ToLower(kw))
				if len(text) > 0 { // Should implement actual check
					matches = append(matches, skill)
					break
				}
			}
		}
	}
	return matches
}
