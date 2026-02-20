package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────
// OllamaEmbedder — Ollama Embedding Provider
// ─────────────────────────────────────────────────────────────

// OllamaEmbedder 使用 Ollama API 產生 Embedding
type OllamaEmbedder struct {
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

// NewOllamaEmbedder 建立 Ollama Embedding Provider
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "mxbai-embed-large"
	}
	return &OllamaEmbedder{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		dimensions: 1024, // mxbai-embed-large: 1024 dims
		client:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OllamaEmbedder) Name() string      { return "ollama" }
func (o *OllamaEmbedder) ModelName() string { return o.model }
func (o *OllamaEmbedder) Dimensions() int   { return o.dimensions }

// Embed 批次取得文本 Embedding
func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(texts))

	// Ollama embed API 支援批次（/api/embed）
	reqBody := map[string]interface{}{
		"model": o.model,
		"input": texts,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/embed", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed returned status %d", resp.StatusCode)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	for i, emb := range result.Embeddings {
		if i < len(results) {
			results[i] = emb
		}
	}

	// 更新 dimensions 如果回傳了資料
	if len(result.Embeddings) > 0 && len(result.Embeddings[0]) > 0 {
		o.dimensions = len(result.Embeddings[0])
	}

	return results, nil
}

// ─────────────────────────────────────────────────────────────
// OpenAIEmbedder — OpenAI Embedding Provider
// ─────────────────────────────────────────────────────────────

// OpenAIEmbedder 使用 OpenAI API 產生 Embedding
type OpenAIEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewOpenAIEmbedder 建立 OpenAI Embedding Provider
func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: 1536,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAIEmbedder) Name() string      { return "openai" }
func (o *OpenAIEmbedder) ModelName() string { return o.model }
func (o *OpenAIEmbedder) Dimensions() int   { return o.dimensions }

// Embed 批次取得文本 Embedding
func (o *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := map[string]interface{}{
		"model": o.model,
		"input": texts,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embed returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(results) {
			results[d.Index] = d.Embedding
		}
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────
// GeminiEmbedder — Gemini Embedding Provider
// ─────────────────────────────────────────────────────────────

// GeminiEmbedder 使用 Gemini API 產生 Embedding
type GeminiEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
}

// NewGeminiEmbedder 建立 Gemini Embedding Provider
func NewGeminiEmbedder(apiKey, model string) *GeminiEmbedder {
	if model == "" {
		model = "text-embedding-004"
	}
	return &GeminiEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: 768,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiEmbedder) Name() string      { return "gemini" }
func (g *GeminiEmbedder) ModelName() string { return g.model }
func (g *GeminiEmbedder) Dimensions() int   { return g.dimensions }

// Embed 批次取得文本 Embedding
func (g *GeminiEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Gemini batchEmbedContents API
	requests := make([]map[string]interface{}, len(texts))
	for i, text := range texts {
		requests[i] = map[string]interface{}{
			"model": "models/" + g.model,
			"content": map[string]interface{}{
				"parts": []map[string]string{{"text": text}},
			},
		}
	}

	reqBody := map[string]interface{}{
		"requests": requests,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s",
		g.model, g.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini embed returned status %d", resp.StatusCode)
	}

	var result struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([][]float32, len(texts))
	for i, emb := range result.Embeddings {
		if i < len(results) {
			results[i] = emb.Values
		}
	}
	return results, nil
}

// ─────────────────────────────────────────────────────────────
// AutoSelectProvider — 自動選擇 Embedding Provider
// ─────────────────────────────────────────────────────────────

// AutoSelectProvider 根據環境變數自動選擇可用的 Embedding Provider
func AutoSelectProvider(cfg SearchConfig) EmbeddingProvider {
	switch cfg.Provider {
	case "openai":
		apiKey := cfg.Remote.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if apiKey != "" {
			return NewOpenAIEmbedder(apiKey, cfg.Model)
		}
	case "gemini":
		apiKey := cfg.Remote.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey != "" {
			return NewGeminiEmbedder(apiKey, cfg.Model)
		}
	case "ollama":
		url := cfg.OllamaURL
		if url == "" {
			url = os.Getenv("OLLAMA_HOST")
		}
		model := cfg.Model
		if model == "" {
			model = "mxbai-embed-large"
		}
		return NewOllamaEmbedder(url, model)
	case "none":
		return nil
	}

	// 預設：嘗試 Ollama
	url := cfg.OllamaURL
	if url == "" {
		url = os.Getenv("OLLAMA_HOST")
		if url == "" {
			url = os.Getenv("PCAI_OLLAMA_URL")
		}
	}
	model := cfg.Model
	if model == "" {
		model = "mxbai-embed-large"
	}
	return NewOllamaEmbedder(url, model)
}

// ─────────────────────────────────────────────────────────────
// ToolKit — 工具套件 API（公開介面）
// ─────────────────────────────────────────────────────────────

// ToolKit 記憶系統 API（供 tools 層、agent 層使用）
type ToolKit struct {
	mgr     *Manager
	writer  *MemoryWriter
	reader  *MemoryReader
	search  *SearchEngine
	indexer *Indexer
	flusher *Flusher
	watcher *FileWatcher
}

// NewToolKit 建立完整的記憶工具套件
func NewToolKit(cfg MemoryConfig) (*ToolKit, error) {
	mgr, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}

	// 選擇 Embedding Provider
	embedder := AutoSelectProvider(cfg.Search)
	mgr.SetEmbedder(embedder)

	tk := &ToolKit{
		mgr:     mgr,
		writer:  NewMemoryWriter(mgr),
		reader:  NewMemoryReader(mgr),
		search:  NewSearchEngine(mgr),
		indexer: NewIndexer(mgr),
		flusher: NewFlusher(mgr),
		watcher: NewFileWatcher(mgr),
	}

	// 初始索引
	ctx := context.Background()
	if err := tk.indexer.IndexAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ [Memory] 初始索引失敗: %v\n", err)
	}

	// 啟動檔案監視（5 秒輪詢）
	if cfg.Search.Sync.Watch {
		tk.watcher.Start(ctx, 5*time.Second)
	}

	return tk, nil
}

// Manager 回傳底層管理器（供 WebAPI 等特殊用途）
func (tk *ToolKit) Manager() *Manager {
	return tk.mgr
}

// MemorySearch 搜尋記憶
func (tk *ToolKit) MemorySearch(ctx context.Context, query string) (*MemorySearchResponse, error) {
	return tk.search.Search(ctx, query, 0)
}

// MemorySearchTopK 搜尋記憶（指定回傳筆數）
func (tk *ToolKit) MemorySearchTopK(ctx context.Context, query string, topK int) (*MemorySearchResponse, error) {
	return tk.search.Search(ctx, query, topK)
}

// MemoryGet 讀取指定記憶檔案
func (tk *ToolKit) MemoryGet(relPath string, startLine, numLines int) (string, error) {
	return tk.reader.Get(relPath, startLine, numLines)
}

// WriteToday 寫入今日日誌
func (tk *ToolKit) WriteToday(content string) error {
	return tk.writer.WriteToday(content)
}

// WriteLongTerm 寫入長期記憶
func (tk *ToolKit) WriteLongTerm(category, content string) error {
	return tk.writer.WriteLongTerm(category, content)
}

// LoadBootstrap 載入 Session 啟動記憶
func (tk *ToolKit) LoadBootstrap() (string, error) {
	return tk.reader.LoadBootstrap()
}

// CheckFlush 檢查是否需要記憶沖洗
func (tk *ToolKit) CheckFlush(estimatedTokens int, cycleID string) CompactionGuard {
	return tk.flusher.CheckFlush(estimatedTokens, cycleID)
}

// ReIndex 手動觸發重新索引
func (tk *ToolKit) ReIndex(ctx context.Context) error {
	return tk.indexer.IndexAll(ctx)
}

// Close 關閉記憶系統
func (tk *ToolKit) Close() error {
	if tk.watcher != nil {
		tk.watcher.Stop()
	}
	if tk.mgr != nil {
		return tk.mgr.Close()
	}
	return nil
}

// ChunkCount 回傳索引中的 chunk 數量
func (tk *ToolKit) ChunkCount() int {
	var count int
	if err := tk.mgr.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&count); err != nil {
		return 0
	}
	return count
}
