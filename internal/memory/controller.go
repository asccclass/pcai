package memory

import (
	"fmt"
)

// Controller orchestrates the memory process (Router + Executor)
type Controller struct {
	Manager      *Manager
	SkillManager *SkillManager
	Executor     *MemoryExecutor
	PendingStore *PendingStore // [NEW] Pending Store for confirmation
}

// NewController creates a new memory controller
func NewController(m *Manager, sm *SkillManager, exec *MemoryExecutor, ps *PendingStore) *Controller {
	return &Controller{
		Manager:      m,
		SkillManager: sm,
		Executor:     exec,
		PendingStore: ps,
	}
}

// ConfirmPending approves a pending memory item and saves it to permanent storage
func (c *Controller) ConfirmPending(id string) (string, error) {
	entry, err := c.PendingStore.Confirm(id)
	if err != nil {
		return "", err
	}

	err = c.Manager.Add(entry.Content, entry.Tags)
	if err != nil {
		return "", fmt.Errorf("failed to save to permanent memory: %w", err)
	}

	return fmt.Sprintf("✅ Memory confirmed and saved. ID: %s", id), nil
}

// RejectPending discards a pending memory item
func (c *Controller) RejectPending(id string) (string, error) {
	if err := c.PendingStore.Reject(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("🗑️ Memory rejected and discarded. ID: %s", id), nil
}

// ListPending returns all pending items
func (c *Controller) ListPending() []*PendingEntry {
	return c.PendingStore.List()
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

				// [CHANGED] Instead of Manager.Add directly, we use PendingStore
				pendingID := c.PendingStore.Add(content, tags)

				logs = append(logs, fmt.Sprintf("⏳ Memory pending confirmation (ID: %s): %s", pendingID, content))
			}
		}
	}

	return logs, nil
}
