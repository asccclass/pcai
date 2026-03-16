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
	var toolMsg *ollama.Message
	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].Role == "tool" {
			toolMsg = &session.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("Expected a tool feedback message in session history, got: %+v", session.Messages)
	}

	expectedPhrase := "Self-Evolution Protocol"
	if !strings.Contains(toolMsg.Content, expectedPhrase) {
		t.Errorf("Expected tool feedback to contain '%s', got:\n%s", expectedPhrase, toolMsg.Content)
	}

	if !strings.Contains(toolMsg.Content, "skill_scaffold") {
		t.Errorf("Expected tool feedback to suggestion 'skill_scaffold', got:\n%s", toolMsg.Content)
	}
}
