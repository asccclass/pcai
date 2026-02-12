package memory

import (
	"fmt"
)

// Controller orchestrates the memory process (Router + Executor)
type Controller struct {
	Manager      *Manager
	SkillManager *SkillManager
	Executor     *MemoryExecutor
}

// NewController creates a new memory controller
func NewController(m *Manager, sm *SkillManager, exec *MemoryExecutor) *Controller {
	return &Controller{
		Manager:      m,
		SkillManager: sm,
		Executor:     exec,
	}
}

// ProcessChatHistory analyzes chat history and triggers relevant memory skills
func (c *Controller) ProcessChatHistory(chatText string) ([]string, error) {
	// 1. Select Skills (The Router)
	// Currently using simple keyword matching from SkillManager
	skills := c.SkillManager.FindMatchingSkills(chatText)
	if len(skills) == 0 {
		return nil, nil // No skills triggered
	}

	var logs []string

	// 2. Execute Skills (The Worker)
	for _, skill := range skills {
		fmt.Printf("🧠 [Memory] Triggering skill: %s\n", skill.Name)
		result, err := c.Executor.Execute(skill, chatText)
		if err != nil {
			fmt.Printf("⚠️ Skill %s failed: %v\n", skill.Name, err)
			continue
		}

		// 3. Apply changes (The Writer)
		if found, ok := result["found"].(bool); ok && found {
			content, _ := result["content"].(string)
			category, _ := result["category"].(string)

			if content != "" {
				// Add to Vector DB & Markdown
				// For now, we tag it with the skill name/category
				tags := []string{"autosave", skill.Name}
				if category != "" {
					tags = append(tags, category)
				}

				err := c.Manager.Add(content, tags)
				if err != nil {
					logs = append(logs, fmt.Sprintf("❌ Failed to save memory from %s: %v", skill.Name, err))
				} else {
					logs = append(logs, fmt.Sprintf("✅ Encoded memory from %s: %s", skill.Name, content))
				}
			}
		}
	}

	return logs, nil
}
