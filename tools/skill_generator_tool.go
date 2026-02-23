package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

type SkillGeneratorTool struct {
	Client    *api.Client
	ModelName string
	SkillsDir string
}

func NewSkillGeneratorTool(client *api.Client, modelName, skillsDir string) *SkillGeneratorTool {
	return &SkillGeneratorTool{
		Client:    client,
		ModelName: modelName,
		SkillsDir: skillsDir,
	}
}

func (t *SkillGeneratorTool) Name() string {
	return "generate_skill"
}

func (t *SkillGeneratorTool) IsSkill() bool {
	return false
}

func (t *SkillGeneratorTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "generate_skill",
			Description: "Automatically analyze a user's goal and generate a new Skill (SKILL.md). Use this when the user asks for a capability that doesn't exist yet, or when you are hallucinating a missing tool.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"goal": {
						"type": "string",
						"description": "The high-level goal or requirement description, e.g., 'View system memory content' or 'List PDF files'."
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"goal"},
				}
			}(),
		},
	}
}

func (t *SkillGeneratorTool) Run(argsJSON string) (string, error) {
	var args struct {
		Goal string `json:"goal"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	// 1. Construct Prompt
	prompt := fmt.Sprintf(`You are an expert Skill Engineer for the PCAI agent. 
Your task is to create a new Skill based on the user's goal: "%s".

A Skill is defined by a single file: 'SKILL.md', which contains YAML frontmatter and a Markdown body.
The YAML frontmatter MUST include:
- name: (snake_case, unique)
- description: (what it does)
- command: (the shell command to execute, using {{param}} for placeholders)
- options: (optional, for enums)

The Markdown body explains the usage.

Goal Analysis:
1. Determine the best shell command (Windows/Linux compatible if possible, or assume the likely OS environment).
   - If the user asks to "View system memory", they might mean RAM usage or the agent's 'memory' file.
   - Context: The user likely means the agent's long-term memory file 'botmemory/MEMORY.md' if they say "system memory content".
   - If they mean RAM, use 'systeminfo' or 'free'.
   - **Crucial**: If the request is about "system memory content" (repository of knowledge), the command should be 'cat botmemory/MEMORY.md'.
   - If the request is about "RAM usage", use 'typeperf'.
   - Assume standard tools are available (cat, type, dir, etc.).

2. Generate the SKILL.md content.

Output Format:
Return ONLY the SKILL.md content. Do not include loose text. 
Start with '---' and end with the markdown body.

Example for 'read file':
---
name: read_file_example
description: Read a file
command: cat {{path}}
---
# Read File Example
Reads the file at {{path}}.
`, args.Goal)

	// 2. Call LLM
	ctx := context.Background()
	req := &api.GenerateRequest{
		Model:  t.ModelName,
		Prompt: prompt,
		Stream: new(bool), // false
	}

	var generatedContent strings.Builder
	respFunc := func(resp api.GenerateResponse) error {
		generatedContent.WriteString(resp.Response)
		return nil
	}

	if err := t.Client.Generate(ctx, req, respFunc); err != nil {
		return "", fmt.Errorf("LLM generation failed: %v", err)
	}

	content := generatedContent.String()

	// 3. Extract YAML header to find 'name'
	// Simplistic parsing: find "name: value"
	lines := strings.Split(content, "\n")
	var skillName string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			skillName = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			break
		}
	}

	if skillName == "" {
		// Fallback: generate a name from goal
		skillName = "generated_skill_" + fmt.Sprintf("%d", time.Now().Unix())
	}

	// Sanitize skill name
	skillName = strings.ReplaceAll(skillName, " ", "_")
	skillName = strings.ToLower(skillName)

	// 4. Save to file
	skillDir := filepath.Join(t.SkillsDir, skillName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write SKILL.md: %v", err)
	}

	return fmt.Sprintf("✅ Skill generated successfully!\nPath: %s\nName: %s\n\nPlease reload skills or use 'reload_skills' to enable it, then you can call '%s'.", skillPath, skillName, skillName), nil
}
