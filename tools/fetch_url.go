package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// WebFetchTool 使用 trafilatura CLI 擷取網頁內容
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) IsSkill() bool {
	return false
}

func (t *WebFetchTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "web_fetch",
			Description: "Fetch and extract readable content from a URL (HTML → markdown/text). Uses trafilatura for high-quality extraction. Supports static and JavaScript-rendered (SPA) pages via optional headless Chromium.",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"url": {
						"type": "string",
						"description": "Target URL to fetch"
					},
					"format": {
						"type": "string",
						"description": "Output format: 'markdown', 'txt', 'json'. Default is 'markdown'.",
						"enum": ["markdown", "txt", "json"]
					},
					"headless": {
						"type": "boolean",
						"description": "Use Playwright headless Chromium to render SPA/JS-heavy pages. Default is false."
					},
					"tables": {
						"type": "boolean",
						"description": "Include tables in output. Default is true."
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

// trafilaturaResult 對應 trafilatura -format json 的輸出結構
type trafilaturaResult struct {
	Title    string `json:"title"`
	Text     string `json:"text"`
	URL      string `json:"url"`
	Hostname string `json:"hostname"`
	Language string `json:"language"`
	Date     string `json:"date"`
	Author   string `json:"author"`
}

// trafilaturaBin 回傳 trafilatura.exe 的絕對路徑
func trafilaturaBin() string {
	// 取得可執行檔所在目錄（支援開發與部署）
	execPath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(execPath)
		bin := filepath.Join(dir, "bin", "trafilatura.exe")
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}
	// fallback：從工作目錄找
	cwd, _ := os.Getwd()
	binName := "trafilatura.exe"
	if runtime.GOOS != "windows" {
		binName = "trafilatura"
	}
	return filepath.Join(cwd, "bin", binName)
}

func (t *WebFetchTool) Run(argsJSON string) (string, error) {
	var args struct {
		URL      string `json:"url"`
		Format   string `json:"format"`
		Headless bool   `json:"headless"`
		Tables   *bool  `json:"tables"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	if args.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	// 決定最終格式：工具內部一律以 json 擷取，再依需求轉換輸出
	internalFormat := "json"
	outputFormat := args.Format
	if outputFormat == "" {
		outputFormat = "markdown"
	}

	// 組建命令列
	bin := trafilaturaBin()
	cmdArgs := []string{
		args.URL,
		"-format=" + internalFormat,
		"-pretty",
		"-tables=true",
	}
	if args.Headless {
		cmdArgs = append(cmdArgs, "-headless")
	}
	if args.Tables != nil && !*args.Tables {
		cmdArgs = append(cmdArgs, "-tables=false")
	}

	cmd := exec.Command(bin, cmdArgs...)
	out, err := cmd.Output()

	raw := ""
	if err == nil {
		raw = strings.TrimSpace(string(out))
	}

	// 檢查是否需要 HTTP 回退機制 (Fallback)
	// 當 trafilatura 無法抽取內容 (通常發生在純 JSON API 或 302 重定向腳本) 時會回傳 empty 或 stderr 包含錯誤
	needsFallback := false
	var stderrMsg string
	if err != nil {
		needsFallback = true
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderrMsg = strings.TrimSpace(string(exitErr.Stderr))
		}
	} else if raw == "" || strings.Contains(raw, "no content extracted") {
		needsFallback = true
	} else {
		// 檢查解析出來的 JSON 是否 text 也是空的
		var checkResult trafilaturaResult
		if jErr := json.Unmarshal([]byte(raw), &checkResult); jErr == nil && checkResult.Text == "" {
			needsFallback = true
		}
	}

	if needsFallback {
		fallbackContent, fErr := fallbackHTTPGet(args.URL)
		if fErr == nil && strings.TrimSpace(fallbackContent) != "" {
			// Fallback 成功，根據輸出的格式直接回傳
			if strings.ToLower(outputFormat) == "json" {
				// 嘗試美化 JSON
				var jsonObj interface{}
				if json.Unmarshal([]byte(fallbackContent), &jsonObj) == nil {
					prettyJSON, _ := json.MarshalIndent(jsonObj, "", "  ")
					output := map[string]interface{}{
						"content":      string(prettyJSON),
						"title":        args.URL,
						"url":          args.URL,
						"source":       "http_fallback",
						"extracted_at": time.Now().Format(time.RFC3339),
					}
					bytes, _ := json.MarshalIndent(output, "", "  ")
					return string(bytes), nil
				}
				// 若不是合法的 JSON (例如純字串)，放入 content 中
				output := map[string]interface{}{
					"content":      fallbackContent,
					"title":        args.URL,
					"url":          args.URL,
					"source":       "http_fallback",
					"extracted_at": time.Now().Format(time.RFC3339),
				}
				bytes, _ := json.MarshalIndent(output, "", "  ")
				return string(bytes), nil
			}
			return fallbackContent, nil
		}
	}

	// 若 Fallback 也失敗或沒觸發，回報原本 trafilatura 的錯誤
	if err != nil {
		hint := ""
		switch {
		case strings.Contains(stderrMsg, "no such host") || strings.Contains(stderrMsg, "dial tcp"):
			hint = "DNS 解析失敗：請確認 URL 正確或網路連線是否正常。"
		case strings.Contains(stderrMsg, "HTTP 404"):
			hint = "頁面不存在（HTTP 404）：請嘗試其他路徑或改用搜尋引擎找正確網址。"
		case strings.Contains(stderrMsg, "HTTP 403") || strings.Contains(stderrMsg, "HTTP 401"):
			hint = "伺服器拒絕存取（HTTP 403/401）：頁面可能需要登入或封鎖爬蟲，可嘗試 headless 模式。"
		case strings.Contains(stderrMsg, "timeout") || strings.Contains(stderrMsg, "context deadline"):
			hint = "請求逾時：網站可能太慢或不可用，可嘗試使用 headless 模式或稍後再試。"
		default:
			if !args.Headless {
				hint = "可嘗試加上 headless:true 參數以使用無頭瀏覽器渲染 JS 動態頁面。"
			}
		}
		msg := fmt.Sprintf("[web_fetch 失敗] URL: %s\n原因: %s", args.URL, stderrMsg)
		if hint != "" {
			msg += "\n建議: " + hint
		}
		return msg, nil
	}

	if raw == "" {
		msg := fmt.Sprintf("[web_fetch 警告] trafilatura 未能從 %s 擷取到任何內容。", args.URL)
		if !args.Headless {
			msg += "\n建議：該頁面可能依賴 JavaScript 動態渲染，請嘗試加上 headless:true 參數。"
		}
		return msg, nil
	}

	// 解析 JSON 結果
	var result trafilaturaResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		// 若解析失敗直接回傳原始文字
		return raw, nil
	}

	// 依格式要求輸出
	switch strings.ToLower(outputFormat) {
	case "json":
		output := map[string]interface{}{
			"content":      result.Text,
			"title":        result.Title,
			"url":          result.URL,
			"hostname":     result.Hostname,
			"language":     result.Language,
			"date":         result.Date,
			"author":       result.Author,
			"source":       "trafilatura",
			"extracted_at": time.Now().Format(time.RFC3339),
		}
		bytes, _ := json.MarshalIndent(output, "", "  ")
		return string(bytes), nil

	case "txt", "text":
		return result.Text, nil

	default: // markdown
		var sb strings.Builder
		if result.Title != "" {
			sb.WriteString("# " + result.Title + "\n\n")
		}
		if result.URL != "" {
			sb.WriteString("> 來源：" + result.URL + "\n\n")
		}
		sb.WriteString(result.Text)
		return sb.String(), nil
	}
}

// fallbackHTTPGet 提供原生的 HTTP GET，支援像 Google Apps Script 回傳 JSON 的純資料 API 端點
func fallbackHTTPGet(urlStr string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bodyBytes), nil
}
