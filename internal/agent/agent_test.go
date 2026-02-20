package agent

import (
	"strings"
	"testing"

	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/ollama/ollama/api"
)

func TestSelfEvolutionProtocolInjection(t *testing.T) {
	// 1. Setup
	registry := core.NewRegistry()
	session := &history.Session{}

	// Mock Logger (optional, nil is fine)
	agent := NewAgent("mock-model", "system prompt", session, registry, nil)

	// 2. Mock Provider
	// Simulate LLM calling a non-existent tool
	callCount := 0
	mockProvider := func(model string, messages []ollama.Message, tools []api.Tool, options ollama.Options, onStream func(string)) (ollama.Message, error) {
		callCount++
		if callCount == 1 {
			return ollama.Message{
				Role: "assistant",
				ToolCalls: []api.ToolCall{
					{
						Function: api.ToolCallFunction{
							Name:      "magic_tool",
							Arguments: api.ToolCallFunctionArguments{},
						},
					},
				},
			}, nil
		}
		return ollama.Message{
			Role:    "assistant",
			Content: "I apologize.",
		}, nil
	}
	agent.Provider = mockProvider

	// 3. Execute
	_, err := agent.Chat("Please use magic tool", nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	// 4. Verify
	// Check the last message in session history
	lastMsg := session.Messages[len(session.Messages)-1]
	if lastMsg.Role != "tool" {
		t.Errorf("Expected last message role to be 'tool', got '%s'", lastMsg.Role)
	}

	expectedPhrase := "觸發「自我演化協議」(Self-Evolution Protocol)"
	if !strings.Contains(lastMsg.Content, expectedPhrase) {
		t.Errorf("Expected tool feedback to contain '%s', got:\n%s", expectedPhrase, lastMsg.Content)
	}

	if !strings.Contains(lastMsg.Content, "skill_scaffold") {
		t.Errorf("Expected tool feedback to suggestion 'skill_scaffold', got:\n%s", lastMsg.Content)
	}
}
