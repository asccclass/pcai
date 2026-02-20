// Package memory implements OpenClaw-compatible memory system in Go.
// Architecture: Markdown-first, multi-layer memory with hybrid search (BM25 + Vector).
package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ─────────────────────────────────────────────────────────────
// 核心類型定義
// ─────────────────────────────────────────────────────────────

// MemoryConfig 記憶系統全局配置
type MemoryConfig struct {
	Backend      string           `json:"backend"`      // "builtin" | "qmd"
	Citations    string           `json:"citations"`    // "auto" | "on" | "off"
	WorkspaceDir string           `json:"workspaceDir"` // Markdown 記憶根目錄
	AgentID      string           `json:"agentId"`
	StateDir     string           `json:"stateDir"` // SQLite 存儲目錄
	Search       SearchConfig     `json:"search"`
	Compaction   CompactionConfig `json:"compaction"`
}

// SearchConfig 搜尋配置
type SearchConfig struct {
	Provider     string             `json:"provider"` // "ollama" | "openai" | "gemini" | "none"
	Model        string             `json:"model"`
	Fallback     string             `json:"fallback"`
	ExtraPaths   []string           `json:"extraPaths"`
	Hybrid       HybridConfig       `json:"hybrid"`
	Cache        CacheConfig        `json:"cache"`
	Store        StoreConfig        `json:"store"`
	Sync         SyncConfig         `json:"sync"`
	Limits       LimitsConfig       `json:"limits"`
	Remote       RemoteConfig       `json:"remote"`
	Experimental ExperimentalConfig `json:"experimental"`
	// Ollama 專用配置
	OllamaURL string `json:"ollamaUrl"`
}

// HybridConfig BM25 + 向量混合搜尋配置
type HybridConfig struct {
	Enabled             bool    `json:"enabled"`
	VectorWeight        float64 `json:"vectorWeight"`        // 預設 0.7
	TextWeight          float64 `json:"textWeight"`          // 預設 0.3
	CandidateMultiplier int     `json:"candidateMultiplier"` // 預設 4
}

// CacheConfig Embedding 快取配置
type CacheConfig struct {
	Enabled    bool  `json:"enabled"`
	MaxEntries int64 `json:"maxEntries"` // 預設 50000
}

// StoreConfig SQLite 向量存儲配置
type StoreConfig struct {
	Path   string            `json:"path"`
	Vector VectorStoreConfig `json:"vector"`
}

// VectorStoreConfig sqlite-vec 加速配置
type VectorStoreConfig struct {
	Enabled       bool   `json:"enabled"`
	ExtensionPath string `json:"extensionPath"`
}

// SyncConfig 索引同步配置
type SyncConfig struct {
	Watch    bool              `json:"watch"`
	Sessions SessionSyncConfig `json:"sessions"`
}

// SessionSyncConfig Session 記憶同步 Delta 閾值
type SessionSyncConfig struct {
	DeltaBytes    int64 `json:"deltaBytes"`    // 預設 100000
	DeltaMessages int   `json:"deltaMessages"` // 預設 50
}

// LimitsConfig 搜尋限制配置
type LimitsConfig struct {
	MaxResults       int   `json:"maxResults"`
	MaxSnippetChars  int   `json:"maxSnippetChars"`
	MaxInjectedChars int   `json:"maxInjectedChars"`
	TimeoutMs        int64 `json:"timeoutMs"`
}

// RemoteConfig 遠端 Embedding API 配置
type RemoteConfig struct {
	BaseURL string            `json:"baseUrl"`
	APIKey  string            `json:"apiKey"`
	Headers map[string]string `json:"headers"`
}

// ExperimentalConfig 實驗性功能
type ExperimentalConfig struct {
	SessionMemory bool     `json:"sessionMemory"`
	Sources       []string `json:"sources"` // ["memory", "sessions"]
}

// CompactionConfig 壓縮 + MemoryFlush 配置
type CompactionConfig struct {
	ReserveTokensFloor int               `json:"reserveTokensFloor"` // 預設 20000
	MemoryFlush        MemoryFlushConfig `json:"memoryFlush"`
}

// MemoryFlushConfig 自動記憶沖洗配置
type MemoryFlushConfig struct {
	Enabled             bool   `json:"enabled"`
	SoftThresholdTokens int    `json:"softThresholdTokens"` // 預設 4000
	SystemPrompt        string `json:"systemPrompt"`
	Prompt              string `json:"prompt"`
}

// ─────────────────────────────────────────────────────────────
// 記憶搜尋結果
// ─────────────────────────────────────────────────────────────

// MemoryChunk 記憶文本分塊
type MemoryChunk struct {
	ID        string    `json:"id"`
	FilePath  string    `json:"filePath"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
	Content   string    `json:"content"`
	Tokens    int       `json:"tokens"`
	Embedding []float32 `json:"-"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SearchResult 搜尋結果
type SearchResult struct {
	Chunk       *MemoryChunk `json:"chunk"`
	VectorScore float64      `json:"vectorScore"`
	TextScore   float64      `json:"textScore"`
	FinalScore  float64      `json:"finalScore"`
	Source      string       `json:"source"` // "memory" | "sessions"
}

// MemorySearchResponse memory_search 工具回應
type MemorySearchResponse struct {
	Results  []SearchResult `json:"results"`
	Backend  string         `json:"backend"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Fallback bool           `json:"fallback"`
}

// ─────────────────────────────────────────────────────────────
// Embedding Provider 介面
// ─────────────────────────────────────────────────────────────

// EmbeddingProvider Embedding 服務介面
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
	Name() string
	ModelName() string
}

// ─────────────────────────────────────────────────────────────
// 記憶管理器主結構
// ─────────────────────────────────────────────────────────────

// Manager 記憶系統管理器（核心）
type Manager struct {
	cfg        MemoryConfig
	db         *sql.DB
	mu         sync.RWMutex
	embedder   EmbeddingProvider
	watcher    *FileWatcher
	indexDirty bool
	lastSync   time.Time
	flushOnce  sync.Map // 記錄每個 compaction cycle 的 flush 狀態
}

// NewManager 建立記憶管理器
func NewManager(cfg MemoryConfig) (*Manager, error) {
	if cfg.WorkspaceDir == "" {
		home, _ := os.Getwd()
		cfg.WorkspaceDir = filepath.Join(home, "botmemory", "knowledge")
	}
	if cfg.StateDir == "" {
		cfg.StateDir = cfg.WorkspaceDir
	}
	if cfg.AgentID == "" {
		cfg.AgentID = "pcai"
	}

	// 預設混合搜尋配置
	if cfg.Search.Hybrid.VectorWeight == 0 {
		cfg.Search.Hybrid.VectorWeight = 0.7
		cfg.Search.Hybrid.TextWeight = 0.3
		cfg.Search.Hybrid.CandidateMultiplier = 4
		cfg.Search.Hybrid.Enabled = true
	}

	// 預設限制
	if cfg.Search.Limits.MaxResults == 0 {
		cfg.Search.Limits.MaxResults = 6
		cfg.Search.Limits.MaxSnippetChars = 700
		cfg.Search.Limits.MaxInjectedChars = 4000
		cfg.Search.Limits.TimeoutMs = 4000
	}

	// 預設 Cache
	if cfg.Search.Cache.MaxEntries == 0 {
		cfg.Search.Cache.MaxEntries = 50000
	}

	// 預設 Compaction
	if cfg.Compaction.ReserveTokensFloor == 0 {
		cfg.Compaction.ReserveTokensFloor = 20000
	}
	if cfg.Compaction.MemoryFlush.SoftThresholdTokens == 0 {
		cfg.Compaction.MemoryFlush.SoftThresholdTokens = 4000
		cfg.Compaction.MemoryFlush.Enabled = true
		cfg.Compaction.MemoryFlush.SystemPrompt = "Session nearing compaction. Store durable memories now."
		cfg.Compaction.MemoryFlush.Prompt = "Write any lasting notes to memory/YYYY-MM-DD.md; reply with NO_REPLY if nothing to store."
	}

	m := &Manager{cfg: cfg}

	// 初始化 SQLite
	if err := m.initDB(); err != nil {
		return nil, fmt.Errorf("init db: %w", err)
	}

	return m, nil
}

// Config 回傳配置（唯讀）
func (m *Manager) Config() MemoryConfig {
	return m.cfg
}

// DB 回傳資料庫連線（供內部使用）
func (m *Manager) DB() *sql.DB {
	return m.db
}

// SetEmbedder 設定 Embedding Provider
func (m *Manager) SetEmbedder(e EmbeddingProvider) {
	m.embedder = e
}

// dbPath SQLite 存儲路徑
func (m *Manager) dbPath() string {
	storePath := m.cfg.Search.Store.Path
	if storePath == "" {
		storePath = filepath.Join(m.cfg.StateDir, "{agentId}_memory.sqlite")
	}
	return strings.ReplaceAll(storePath, "{agentId}", m.cfg.AgentID)
}

// initDB 初始化 SQLite 資料庫結構
func (m *Manager) initDB() error {
	dbPath := m.dbPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		return err
	}

	// modernc.org/sqlite 使用 driver name "sqlite"
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return err
	}

	// 建立 Schema
	schema := `
	CREATE TABLE IF NOT EXISTS chunks (
		id          TEXT PRIMARY KEY,
		file_path   TEXT NOT NULL,
		start_line  INTEGER NOT NULL,
		end_line    INTEGER NOT NULL,
		content     TEXT NOT NULL,
		tokens      INTEGER NOT NULL,
		updated_at  DATETIME NOT NULL,
		file_hash   TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS embeddings (
		chunk_id    TEXT PRIMARY KEY,
		provider    TEXT NOT NULL,
		model       TEXT NOT NULL,
		endpoint    TEXT NOT NULL,
		vector      BLOB NOT NULL,
		created_at  DATETIME NOT NULL,
		FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS embedding_cache (
		content_hash TEXT PRIMARY KEY,
		provider     TEXT NOT NULL,
		model        TEXT NOT NULL,
		vector       BLOB NOT NULL,
		created_at   DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS index_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// FTS5 虛擬表單獨建立（避免重複建立錯誤）
	ftsSchema := `
	CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
		content,
		content='chunks',
		content_rowid='rowid',
		tokenize='unicode61'
	);
	`
	if _, err := db.Exec(ftsSchema); err != nil {
		// FTS5 可能不被支援，印出警告但不中斷
		fmt.Fprintf(os.Stderr, "⚠️ [Memory] FTS5 初始化失敗（BM25 搜尋將不可用）: %v\n", err)
	}

	// FTS 同步觸發器
	triggers := `
	CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
		INSERT INTO chunks_fts(rowid, content) VALUES (new.rowid, new.content);
	END;
	CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
		INSERT INTO chunks_fts(chunks_fts, rowid, content) VALUES('delete', old.rowid, old.content);
	END;
	CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
		INSERT INTO chunks_fts(chunks_fts, rowid, content) VALUES('delete', old.rowid, old.content);
		INSERT INTO chunks_fts(rowid, content) VALUES (new.rowid, new.content);
	END;
	`
	if _, err := db.Exec(triggers); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ [Memory] FTS5 觸發器建立失敗: %v\n", err)
	}

	m.db = db
	return nil
}

// Close 關閉資料庫
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// 輔助工具（被多個檔案共用）
// ─────────────────────────────────────────────────────────────

// float32SliceToBytes 將 float32 切片轉為小端 byte slice（SQLite 儲存用）
func float32SliceToBytes(vec []float32) []byte {
	b := make([]byte, len(vec)*4)
	for i, v := range vec {
		bits := math.Float32bits(v)
		b[i*4] = byte(bits)
		b[i*4+1] = byte(bits >> 8)
		b[i*4+2] = byte(bits >> 16)
		b[i*4+3] = byte(bits >> 24)
	}
	return b
}

// bytesToFloat32Slice 將小端 byte slice 還原為 float32 切片
func bytesToFloat32Slice(b []byte) []float32 {
	vec := make([]float32, len(b)/4)
	for i := range vec {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		vec[i] = math.Float32frombits(bits)
	}
	return vec
}

// contentHash 計算文本內容的 SHA256 Hash（用於 Embedding Cache）
func contentHash(text, provider, model string) string {
	h := sha256.New()
	h.Write([]byte(provider + ":" + model + ":" + text))
	return fmt.Sprintf("%x", h.Sum(nil))
}
