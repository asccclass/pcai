package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ollama/ollama/api"
)

// WebFetchTool Wrapper for the robust WebFetcher function
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "web_fetch",
			Description: "Fetch and extract readable content from a URL (HTML → markdown/text). Support Firecrawl and Readability.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"url": {
						"type": "string",
						"description": "Target URL to fetch"
					},
					"extractMode": {
						"type": "string",
						"description": "Extraction mode ('markdown' or 'text'). Default is 'markdown'.",
						"enum": ["markdown", "text"]
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"url"},
				}
			}(),
		},
	}
}

func (t *WebFetchTool) Run(argsJSON string) (string, error) {
	var args struct {
		URL         string `json:"url"`
		ExtractMode string `json:"extractMode"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	if args.ExtractMode == "" {
		args.ExtractMode = "markdown"
	}

	fetcher := NewWebFetcher()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := fetcher.Fetch(ctx, WebFetchOptions{
		URL:          args.URL,
		ExtractMode:  args.ExtractMode,
		UseFirecrawl: true, // Enable by default if key is present
	})
	if err != nil {
		return "", fmt.Errorf("web_fetch failed: %v", err)
	}

	// Format output to be similar to OpenClaw's JSON result but simpler for now, or just return content?
	// The prompt implies "become the project's tool to get web pages".
	// Returning JSON with metadata is good for agents.

	output := map[string]interface{}{
		"content":      result.Content,
		"title":        result.Title,
		"url":          result.OriginalURL,
		"source":       result.Source,
		"extracted_at": time.Now().Format(time.RFC3339),
	}

	bytes, _ := json.MarshalIndent(output, "", "  ")
	return string(bytes), nil
}
