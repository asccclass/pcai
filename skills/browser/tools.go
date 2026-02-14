package browserskill

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/asccclass/pcai/internal/browser"
	"github.com/ollama/ollama/api"
)

// BrowserOpenTool
type BrowserOpenTool struct{}

func (t *BrowserOpenTool) Name() string { return "browser_open" }

func (t *BrowserOpenTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "browser_open",
			"description": "開啟瀏覽器並導航到指定網址 (Navigate to URL). 支援 http/https/file.",
			"parameters": {
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "要開啟的網址 (URL to navigate to)"
					}
				},
				"required": ["url"]
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *BrowserOpenTool) Run(argsJSON string) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	mgr := browser.GetManager()
	if err := mgr.Navigate(args.URL); err != nil {
		return "", fmt.Errorf("navigation failed: %v", err)
	}

	return fmt.Sprintf("Opened %s", args.URL), nil
}

// BrowserSnapshotTool
type BrowserSnapshotTool struct{}

func (t *BrowserSnapshotTool) Name() string { return "browser_snapshot" }

func (t *BrowserSnapshotTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "browser_snapshot",
			"description": "分析當前頁面，回傳可互動元素的參考 (refs like @e1, @e2). 必須先執行 browser_open.",
			"parameters": {
				"type": "object",
				"properties": {
					"interactive_only": {
						"type": "boolean",
						"description": "是否只抓取可互動元素 (default: true). Set false for full page structure (expensive)."
					}
				}
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *BrowserSnapshotTool) Run(argsJSON string) (string, error) {
	var args struct {
		InteractiveOnly bool `json:"interactive_only"`
	}
	// Default true if not present or handle unmarshal
	args.InteractiveOnly = true // default

	// Custom unmarshal to handle default?
	// Actually json unmarshal will set bool to false if missing.
	// We check map to be sure? map[string]interface{}.
	// Or just assume: if user wants full page, they explicitly say false.
	// But bool default is false. We want default true.
	// Let's parse as map.
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err == nil {
		if v, ok := raw["interactive_only"]; ok {
			if b, ok := v.(bool); ok {
				args.InteractiveOnly = b
			}
		}
	}

	mgr := browser.GetManager()
	res, err := mgr.Snapshot(args.InteractiveOnly)
	if err != nil {
		return "", fmt.Errorf("snapshot failed: %v", err)
	}
	return res, nil
}

// BrowserClickTool
type BrowserClickTool struct{}

func (t *BrowserClickTool) Name() string { return "browser_click" }

func (t *BrowserClickTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "browser_click",
			"description": "點擊指定的元素參考 (Click element ref, e.g., @e1).",
			"parameters": {
				"type": "object",
				"properties": {
					"ref": {
						"type": "string",
						"description": "元素參考 ID (如 @e1) (Element reference ID)"
					}
				},
				"required": ["ref"]
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *BrowserClickTool) Run(argsJSON string) (string, error) {
	var args struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid args: %v", err)
	}
	if !strings.HasPrefix(args.Ref, "@e") {
		return "", fmt.Errorf("invalid ref format: %s. Must start with @e", args.Ref)
	}

	mgr := browser.GetManager()
	if err := mgr.Click(args.Ref); err != nil {
		return "", fmt.Errorf("click failed: %v", err)
	}
	return fmt.Sprintf("Clicked %s", args.Ref), nil
}

// BrowserTypeTool
type BrowserTypeTool struct{}

func (t *BrowserTypeTool) Name() string { return "browser_type" }

func (t *BrowserTypeTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "browser_type",
			"description": "在指定元素輸入文字 (Type text into element ref).",
			"parameters": {
				"type": "object",
				"properties": {
					"ref": {
						"type": "string",
						"description": "元素參考 ID (如 @e1)"
					},
					"text": {
						"type": "string",
						"description": "要輸入的文字 (Text to type)"
					}
				},
				"required": ["ref", "text"]
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *BrowserTypeTool) Run(argsJSON string) (string, error) {
	var args struct {
		Ref  string `json:"ref"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid args: %v", err)
	}

	mgr := browser.GetManager()
	if err := mgr.Type(args.Ref, args.Text); err != nil {
		return "", fmt.Errorf("type failed: %v", err)
	}
	return fmt.Sprintf("Typed %q into %s", args.Text, args.Ref), nil
}

// BrowserScrollTool
type BrowserScrollTool struct{}

func (t *BrowserScrollTool) Name() string { return "browser_scroll" }

func (t *BrowserScrollTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "browser_scroll",
			"description": "捲動頁面 (Scroll page).",
			"parameters": {
				"type": "object",
				"properties": {
					"direction": {
						"type": "string",
						"description": "捲動方向: up, down (default), top, bottom"
					}
				}
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *BrowserScrollTool) Run(argsJSON string) (string, error) {
	var args struct {
		Direction string `json:"direction"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.Direction == "" {
		args.Direction = "down"
	}

	mgr := browser.GetManager()
	if err := mgr.Scroll(args.Direction); err != nil {
		return "", fmt.Errorf("scroll failed: %v", err)
	}
	return fmt.Sprintf("Scrolled %s", args.Direction), nil
}

// BrowserGetTool
type BrowserGetTool struct{}

func (t *BrowserGetTool) Name() string { return "browser_get" }

func (t *BrowserGetTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "browser_get",
			"description": "取得元素內容 (Get element text/html).",
			"parameters": {
				"type": "object",
				"properties": {
					"what": {
						"type": "string",
						"description": "要取得的內容: text (default), html"
					},
					"ref": {
						"type": "string",
						"description": "元素參考 ID (如 @e1)"
					}
				},
				"required": ["ref"]
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *BrowserGetTool) Run(argsJSON string) (string, error) {
	var args struct {
		What string `json:"what"`
		Ref  string `json:"ref"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	if args.What == "" {
		args.What = "text"
	}

	mgr := browser.GetManager()
	if args.What == "text" || args.What == "html" {
		res, err := mgr.GetText(args.Ref) // Implementation gets OuterHTML currently
		if err != nil {
			return "", err
		}
		return res, nil
	}
	return "", fmt.Errorf("unknown get target: %s", args.What)
}
