package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/tools"
	"golang.org/x/net/html"
)

// RecursiveDownloader 遞迴下載器結構
type RecursiveDownloader struct {
	startURL      string
	outputDir     string
	allowedDomain string
	waitTime      time.Duration
	urlsToVisit   map[string]bool
	visitedURLs   map[string]bool
	fetcher       *tools.WebFetcher // Use the new fetcher
	runDocs       bool              // Flag to run legacy docs mode
}

// NewRecursiveDownloader 創建新的下載器實例
func NewRecursiveDownloader(startURL, outputDir, allowedDomain string, waitSeconds int) *RecursiveDownloader {
	return &RecursiveDownloader{
		startURL:      startURL,
		outputDir:     outputDir,
		allowedDomain: allowedDomain,
		waitTime:      time.Duration(waitSeconds) * time.Second,
		urlsToVisit:   make(map[string]bool),
		visitedURLs:   make(map[string]bool),
		fetcher:       tools.NewWebFetcher(),
		runDocs:       false,
	}
}

// processURL 處理單個 URL：下載、保存、提取鏈接
func (rd *RecursiveDownloader) processURL(currentURL string) error {
	// Use tools.WebFetcher logic if NOT running docs mode
	// But currently this tool is only for docs mode or single file.
	// If runDocs is true, we use legacy.
	if rd.runDocs {
		return rd.legacyProcessURL(currentURL)
	}

	// If not legacy, we are likely not entering this method via Download() loop
	// unless we implement a recursive logic for WebFetcher too.
	// For now, Single Mode handled in main() doesn't use RecursiveDownloader.
	return nil
}

func (rd *RecursiveDownloader) legacyProcessURL(currentURL string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", currentURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := rd.saveFile(currentURL, body); err != nil {
		return err
	}
	if err := rd.extractLinks(currentURL, body); err != nil {
		fmt.Printf("  警告: 提取鏈接時發生錯誤 (%v)\n", err)
	}
	return nil
}

// saveFile ... (same as before)
func (rd *RecursiveDownloader) saveFile(currentURL string, content []byte) error {
	parsedURL, err := url.Parse(currentURL)
	if err != nil {
		return err
	}
	localPath := strings.TrimPrefix(parsedURL.Path, "/")
	if strings.HasSuffix(localPath, "/") || filepath.Ext(localPath) == "" {
		localPath = filepath.Join(localPath, "index.html")
	}
	filePath := filepath.Join(rd.outputDir, localPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(filePath, content, 0644)
}

// extractLinks ... (same as before)
func (rd *RecursiveDownloader) extractLinks(currentURL string, content []byte) error {
	// using x/net/html
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return err
	}
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					rd.processLink(currentURL, attr.Val)
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)
	return nil
}

func (rd *RecursiveDownloader) processLink(baseURL, href string) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return
	}
	link, err := url.Parse(href)
	if err != nil {
		return
	}
	absoluteURL := base.ResolveReference(link)
	absoluteURL.Fragment = ""
	newURL := absoluteURL.String()
	if rd.shouldDownload(newURL) {
		rd.urlsToVisit[newURL] = true
	}
}

func (rd *RecursiveDownloader) shouldDownload(newURL string) bool {
	parsedURL, err := url.Parse(newURL)
	if err != nil {
		return false
	}
	if parsedURL.Host != rd.allowedDomain {
		return false
	}
	if !strings.HasPrefix(newURL, rd.startURL) {
		return false
	}
	if rd.visitedURLs[newURL] {
		return false
	}
	ext := path.Ext(parsedURL.Path)
	if ext != "" && ext != ".html" {
		return false
	}
	return true
}

// MAIN
var (
	urlFlag       = flag.String("url", "", "Target URL to fetch (Single Mode)")
	docsFlag      = flag.Bool("docs", false, "Run legacy docs downloader (Recursive Mode)")
	modeFlag      = flag.String("mode", "markdown", "Extract mode: markdown, text (Single Mode)")
	firecrawlFlag = flag.Bool("firecrawl", true, "Use Firecrawl if available (Single Mode)")
)

func main() {
	flag.Parse()

	if *docsFlag {
		fmt.Println("=== Google Apps Script 文檔下載 ===")
		d1 := NewRecursiveDownloader("https://developers.google.com/apps-script/reference/", "gas_docs_html", "developers.google.com", 1)
		d1.runDocs = true // flag to use legacyProcessURL
		d1.Download()

		fmt.Println("\n=== Gemini API 文檔下載 ===")
		d2 := NewRecursiveDownloader("https://ai.google.dev/gemini-api/docs/", "gemini_api_docs_html", "ai.google.dev", 1)
		d2.runDocs = true
		d2.Download()
		return
	}

	if *urlFlag != "" {
		fetcher := tools.NewWebFetcher()
		ctx := context.Background()
		result, err := fetcher.Fetch(ctx, tools.WebFetchOptions{
			URL:          *urlFlag,
			ExtractMode:  *modeFlag,
			UseFirecrawl: *firecrawlFlag,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(result.Content)
		return
	}

	fmt.Println("Usage:")
	fmt.Println("  Single Page: go run web_fetch.go -url https://example.com")
	fmt.Println("  Legacy Docs: go run web_fetch.go -docs")
	flag.PrintDefaults()
}

// Add state to struct
// I have to add `runDocs bool` to `RecursiveDownloader` struct to allow `Download` to choose method?
// Or just hardcode `legacyProcessURL` inside `Download` since `Download` is ONLY for legacy docs?
// Yes, `Download` is only used by the legacy block.
