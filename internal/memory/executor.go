package memory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/llms"
	"github.com/asccclass/pcai/llms/ollama"
)

// MemoryExecutor executes a memory skill using an LLM
type MemoryExecutor struct {
	LLMProvider llms.ChatStreamFunc
	ModelName   string
}

// NewMemoryExecutor creates a new executor
func NewMemoryExecutor(provider llms.ChatStreamFunc, model string) *MemoryExecutor {
	return &MemoryExecutor{
		LLMProvider: provider,
		ModelName:   model,
	}
}

// Execute runs the skill on the given text and returns the extracted memory result
func (e *MemoryExecutor) Execute(skill MemorySkill, text string) (map[string]interface{}, error) {
	// 1. Render Template
	prompt := strings.ReplaceAll(skill.Template, "{{TEXT}}", text)

	// 2. Call LLM
	// We need a non-streaming call, but ChatStreamFunc is designed for streaming.
	// We can wrap it.
	var responseBuilder strings.Builder

	messages := []ollama.Message{
		{Role: "system", Content: "You are a helpful AI memory assistant."},
		{Role: "user", Content: prompt},
	}
	// TODO: Options?

	_, err := e.LLMProvider(e.ModelName, messages, nil, ollama.Options{Temperature: 0.1}, func(chunk string) {
		responseBuilder.WriteString(chunk)
	})

	if err != nil {
		return nil, fmt.Errorf("LLM execution failed: %w", err)
	}

	resultStr := responseBuilder.String()

	// 3. Parse JSON
	// Attempt to find JSON block if LLM adds markdown
	start := strings.Index(resultStr, "{")
	end := strings.LastIndex(resultStr, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response: %s", resultStr)
	}

	jsonStr := resultStr[start : end+1]
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result, nil
}
