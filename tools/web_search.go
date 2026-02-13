package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/ollama/ollama/api"
)

// WebSearchTool uses Brave Search API to search the web
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "web_search",
			Description: "Search the web using Brave Search API. Returns titles, URLs, and snippets.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"query": {
						"type": "string",
						"description": "Search query string"
					},
					"count": {
						"type": "integer",
						"description": "Number of results to return (1-20). Default: 5"
					},
					"country": {
						"type": "string",
						"description": "2-letter country code (e.g., 'US', 'TW'). Default: 'US'"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"query"},
				}
			}(),
		},
	}
}

func (t *WebSearchTool) Run(argsJSON string) (string, error) {
	// Use interface{} to handle potential nested JSON objects (e.g. {"value": "..."})
	var rawArgs struct {
		Query   interface{} `json:"query"`
		Count   interface{} `json:"count"`
		Country interface{} `json:"country"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &rawArgs); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	// Helper to extract string from string or map
	getString := func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		if m, ok := v.(map[string]interface{}); ok {
			if val, ok := m["value"].(string); ok {
				return val
			}
		}
		return ""
	}

	query := getString(rawArgs.Query)
	country := getString(rawArgs.Country)

	// Handle count (int or float64 from JSON)
	count := 5
	if val, ok := rawArgs.Count.(float64); ok {
		count = int(val)
	} else if val, ok := rawArgs.Count.(int); ok {
		count = val
	}

	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return "⚠️ BRAVE_API_KEY is not set in envfile. Please configure it to use web search.", nil
	}

	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}
	if country == "" {
		country = "US"
	}

	return runBraveSearch(apiKey, query, count, country)
}

type BraveSearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Age         string `json:"age,omitempty"`
}

type BraveSearchResponse struct {
	Web *struct {
		Results []BraveSearchResult `json:"results"`
	} `json:"web"`
}

func runBraveSearch(apiKey, query string, count int, country string) (string, error) {
	endpoint := "https://api.search.brave.com/res/v1/web/search"

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("count", strconv.Itoa(count))
	q.Set("country", country)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var data BraveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if data.Web == nil || len(data.Web.Results) == 0 {
		return "No results found.", nil
	}

	// Format results
	var resultStr string
	for i, item := range data.Web.Results {
		ageStr := ""
		if item.Age != "" {
			ageStr = fmt.Sprintf(" (%s)", item.Age)
		}
		resultStr += fmt.Sprintf("%d. [%s](%s)%s\n   %s\n\n", i+1, item.Title, item.URL, ageStr, item.Description)
	}

	return resultStr, nil
}
