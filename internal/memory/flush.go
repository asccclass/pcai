package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────
// MemoryWriter — 記憶寫入器
// ─────────────────────────────────────────────────────────────

// MemoryWriter 負責將記憶寫入 Markdown 檔案
type MemoryWriter struct {
	mgr *Manager
}

// NewMemoryWriter 建立記憶寫入器
func NewMemoryWriter(mgr *Manager) *MemoryWriter {
	return &MemoryWriter{mgr: mgr}
}

// WriteToday 寫入今日日誌 (memory/YYYY-MM-DD.md)
func (w *MemoryWriter) WriteToday(content string) error {
	today := time.Now().Format("2006-01-02")
	memDir := filepath.Join(w.mgr.cfg.WorkspaceDir, "memory")
	if err := os.MkdirAll(memDir, 0750); err != nil {
		return err
	}

	filePath := filepath.Join(memDir, today+".md")

	// 如果檔案不存在，建立標題
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		header := fmt.Sprintf("# 📝 記憶日誌 %s\n\n", today)
		if err := os.WriteFile(filePath, []byte(header), 0644); err != nil {
			return err
		}
	}

	// 追加內容
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format("15:04")
	entry := fmt.Sprintf("\n## %s\n\n%s\n", timestamp, strings.TrimSpace(content))
	_, err = f.WriteString(entry)
	if err != nil {
		return err
	}

	// 觸發重新索引
	w.mgr.indexDirty = true
	return nil
}

// WriteLongTerm 寫入長期記憶 (MEMORY.md)
func (w *MemoryWriter) WriteLongTerm(category string, content string) error {
	filePath := filepath.Join(w.mgr.cfg.WorkspaceDir, "MEMORY.md")

	// 如果檔案不存在，建立標題
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
			return err
		}
		header := "# 🧠 PCAI 長期記憶\n\n此文件包含經過篩選的持久記憶。\n"
		if err := os.WriteFile(filePath, []byte(header), 0644); err != nil {
			return err
		}
	}

	// 追加內容
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	cat := category
	if cat == "" {
		cat = "general"
	}

	entry := fmt.Sprintf("\n## [%s] %s\n\n%s\n\n---\n",
		cat, time.Now().Format("2006-01-02 15:04"), strings.TrimSpace(content))
	_, err = f.WriteString(entry)
	if err != nil {
		return err
	}

	// 觸發重新索引
	w.mgr.indexDirty = true
	return nil
}

// ─────────────────────────────────────────────────────────────
// MemoryReader — 記憶讀取器
// ─────────────────────────────────────────────────────────────

// MemoryReader 負責讀取記憶檔案
type MemoryReader struct {
	mgr *Manager
}

// NewMemoryReader 建立記憶讀取器
func NewMemoryReader(mgr *Manager) *MemoryReader {
	return &MemoryReader{mgr: mgr}
}

// Get 讀取指定檔案的指定行數
func (r *MemoryReader) Get(relPath string, startLine, numLines int) (string, error) {
	fp := relPath
	if !filepath.IsAbs(fp) {
		fp = filepath.Join(r.mgr.cfg.WorkspaceDir, relPath)
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", relPath, err)
	}

	lines := strings.Split(string(data), "\n")

	if startLine <= 0 {
		startLine = 1
	}
	if numLines <= 0 {
		numLines = len(lines)
	}

	end := startLine - 1 + numLines
	if end > len(lines) {
		end = len(lines)
	}
	if startLine-1 >= len(lines) {
		return "", nil
	}

	return strings.Join(lines[startLine-1:end], "\n"), nil
}

// LoadBootstrap 載入 Session 啟動用的記憶摘要（MEMORY.md）
func (r *MemoryReader) LoadBootstrap() (string, error) {
	// 讀取 MEMORY.md 全文
	memoryMD := filepath.Join(r.mgr.cfg.WorkspaceDir, "MEMORY.md")
	if data, err := os.ReadFile(memoryMD); err == nil {
		return string(data), nil
	}

	// 向下相容：嘗試 knowledge.md
	knowledgeMD := filepath.Join(r.mgr.cfg.WorkspaceDir, "knowledge.md")
	if data, err := os.ReadFile(knowledgeMD); err == nil {
		return string(data), nil
	}

	return "", nil
}

// ─────────────────────────────────────────────────────────────
// Flusher — 記憶沖洗決策器
// ─────────────────────────────────────────────────────────────

// Flusher 負責判斷是否需要記憶沖洗
type Flusher struct {
	mgr *Manager
}

// NewFlusher 建立沖洗器
func NewFlusher(mgr *Manager) *Flusher {
	return &Flusher{mgr: mgr}
}

// CompactionGuard 壓縮守衛回傳值
type CompactionGuard struct {
	ShouldFlush  bool   `json:"shouldFlush"`
	SystemPrompt string `json:"systemPrompt,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
}

// CheckFlush 檢查是否需要記憶沖洗
func (f *Flusher) CheckFlush(estimatedTokens int, cycleID string) CompactionGuard {
	cfg := f.mgr.cfg.Compaction

	if !cfg.MemoryFlush.Enabled {
		return CompactionGuard{ShouldFlush: false}
	}

	threshold := cfg.ReserveTokensFloor + cfg.MemoryFlush.SoftThresholdTokens

	if estimatedTokens < threshold {
		return CompactionGuard{ShouldFlush: false}
	}

	// 每個 cycle 只 flush 一次
	if cycleID != "" {
		if _, loaded := f.mgr.flushOnce.LoadOrStore(cycleID, true); loaded {
			return CompactionGuard{ShouldFlush: false}
		}
	}

	return CompactionGuard{
		ShouldFlush:  true,
		SystemPrompt: cfg.MemoryFlush.SystemPrompt,
		Prompt:       cfg.MemoryFlush.Prompt,
	}
}

// ─────────────────────────────────────────────────────────────
// FileWatcher — 檔案監視器（輪詢實作）
// ─────────────────────────────────────────────────────────────

// FileWatcher 監視 Markdown 檔案變更並觸發重新索引
type FileWatcher struct {
	mgr      *Manager
	ticker   *time.Ticker
	done     chan struct{}
	lastSync map[string]time.Time
}

// NewFileWatcher 建立檔案監視器
func NewFileWatcher(mgr *Manager) *FileWatcher {
	return &FileWatcher{
		mgr:      mgr,
		lastSync: make(map[string]time.Time),
	}
}

// Start 啟動監視迴圈
func (fw *FileWatcher) Start(ctx context.Context, interval time.Duration) {
	fw.ticker = time.NewTicker(interval)
	fw.done = make(chan struct{})

	go func() {
		indexer := NewIndexer(fw.mgr)
		for {
			select {
			case <-fw.done:
				return
			case <-ctx.Done():
				return
			case <-fw.ticker.C:
				if fw.mgr.indexDirty {
					fw.mgr.indexDirty = false
					if err := indexer.IndexAll(ctx); err != nil {
						fmt.Fprintf(os.Stderr, "⚠️ [Memory] 重新索引失敗: %v\n", err)
					}
				}
			}
		}
	}()
}

// Stop 停止監視
func (fw *FileWatcher) Stop() {
	if fw.ticker != nil {
		fw.ticker.Stop()
	}
	if fw.done != nil {
		close(fw.done)
	}
}
