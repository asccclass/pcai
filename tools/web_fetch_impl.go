package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/net/html"
)

const (
	DefaultFirecrawlBaseURL = "https://api.firecrawl.dev"
	DefaultUserAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// WebFetchOptions defines options for fetching web content
type WebFetchOptions struct {
	URL             string
	ExtractMode     string // "markdown" or "text"
	FirecrawlAPIKey string // Optional: if empty, checks env FIRECRAWL_API_KEY
	UseFirecrawl    bool   // If true, attempts to use Firecrawl first
	ConnectTimeout  time.Duration
}

// WebFetchResult contains the result of a web fetch operation
type WebFetchResult struct {
	Content     string
	Title       string
	Metadata    map[string]interface{}
	Source      string // "firecrawl", "readability", "raw"
	OriginalURL string
}

// WebFetcher handles fetching web content
type WebFetcher struct {
	client *resty.Client
}

// NewWebFetcher creates a new WebFetcher instance
func NewWebFetcher() *WebFetcher {
	return &WebFetcher{
		client: resty.New().
			SetTimeout(30 * time.Second).
			SetRedirectPolicy(resty.FlexibleRedirectPolicy(5)),
	}
}

// Fetch gets content from a URL using the specified options
func (wf *WebFetcher) Fetch(ctx context.Context, opts WebFetchOptions) (*WebFetchResult, error) {
	// 1. Try Firecrawl if enabled
	apiKey := opts.FirecrawlAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("FIRECRAWL_API_KEY")
	}

	if opts.UseFirecrawl && apiKey != "" {
		result, err := wf.fetchFirecrawl(ctx, opts.URL, apiKey)
		if err == nil {
			result.Source = "firecrawl"
			return result, nil
		}
		// Fallback to standard fetch on error
		fmt.Printf("Firecrawl fetch failed: %v. Falling back to standard fetch.\n", err)
	}

	// 2. Standard Fetch with "Readability" (HTML processing)
	return wf.fetchStandard(ctx, opts.URL, opts.ExtractMode)
}

func (wf *WebFetcher) fetchFirecrawl(ctx context.Context, url string, apiKey string) (*WebFetchResult, error) {
	type FirecrawlRequest struct {
		URL             string   `json:"url"`
		Formats         []string `json:"formats"`
		OnlyMainContent bool     `json:"onlyMainContent"`
	}

	type FirecrawlResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string `json:"markdown"`
			Metadata struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				SourceURL   string `json:"sourceURL"`
			} `json:"metadata"`
		} `json:"data"`
		Error string `json:"error"`
	}

	reqBody := FirecrawlRequest{
		URL:             url,
		Formats:         []string{"markdown"},
		OnlyMainContent: true,
	}

	var respBody FirecrawlResponse
	resp, err := wf.client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json").
		SetBody(reqBody).
		SetResult(&respBody). // Resty automatically unmarshals JSON
		Post(DefaultFirecrawlBaseURL + "/v2/scrape")

	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("status: %d, body: %s", resp.StatusCode(), resp.String())
	}
	if !respBody.Success {
		return nil, fmt.Errorf("firecrawl error: %s", respBody.Error)
	}

	return &WebFetchResult{
		Content:     respBody.Data.Markdown,
		Title:       respBody.Data.Metadata.Title,
		Metadata:    map[string]interface{}{"description": respBody.Data.Metadata.Description},
		OriginalURL: respBody.Data.Metadata.SourceURL,
	}, nil
}

func (wf *WebFetcher) fetchStandard(ctx context.Context, urlStr string, mode string) (*WebFetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	// Check Content-Type
	contentType := resp.Header.Get("Content-Type")
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	bodyStr := string(bodyBytes)

	if strings.Contains(contentType, "text/html") {
		// Parse HTML
		return wf.processHTML(bodyStr, urlStr, mode)
	} else if strings.Contains(contentType, "application/json") {
		// Pretty print JSON
		var jsonObj interface{}
		if err := json.Unmarshal(bodyBytes, &jsonObj); err == nil {
			prettyJSON, _ := json.MarshalIndent(jsonObj, "", "  ")
			return &WebFetchResult{
				Content:     string(prettyJSON),
				Title:       urlStr,
				Source:      "json",
				OriginalURL: urlStr,
			}, nil
		}
	}

	// Fallback for other content types
	return &WebFetchResult{
		Content:     bodyStr,
		Title:       urlStr,
		Source:      "raw",
		OriginalURL: urlStr,
	}, nil
}

// processHTML parses HTML and converts it to a simplified markdown-like format
func (wf *WebFetcher) processHTML(htmlContent string, urlStr string, mode string) (*WebFetchResult, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	var title string
	var extractText func(*html.Node) string

	// Simplified HTML processing similar to OpenClaw's Utils
	extractText = func(n *html.Node) string {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "iframe", "svg":
				return "" // Skip these tags
			case "title":
				// Extract title but don't output it in content body if we store it separately
				if title == "" && n.FirstChild != nil {
					title = n.FirstChild.Data
				}
				return ""
			}
		}

		if n.Type == html.TextNode {
			return strings.TrimSpace(n.Data)
		}

		var result strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			text := extractText(c)
			if text != "" {
				// Add spacing based on tag type
				if n.Type == html.ElementNode {
					switch n.Data {
					case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li":
						result.WriteString("\n" + text + "\n")
					case "a":
						// Basic markdown link
						href := ""
						for _, a := range n.Attr {
							if a.Key == "href" {
								href = a.Val
								break
							}
						}
						if href != "" {
							result.WriteString(fmt.Sprintf("[%s](%s)", text, href))
						} else {
							result.WriteString(text)
						}
					default:
						result.WriteString(" " + text + " ")
					}
				} else {
					result.WriteString(text)
				}
			}
		}
		return result.String()
	}

	content := extractText(doc)

	// Clean up newlines
	content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	content = strings.TrimSpace(content)

	return &WebFetchResult{
		Content:     content,
		Title:       title,
		Source:      "readability-lite",
		OriginalURL: urlStr,
	}, nil
}
