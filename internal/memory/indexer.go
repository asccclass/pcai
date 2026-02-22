package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// ─────────────────────────────────────────────────────────────
// Chunker — Markdown 文件分塊器
// ─────────────────────────────────────────────────────────────

// Chunker 負責將 Markdown 文件切分為可搜尋的塊
type Chunker struct {
	ChunkSize    int // 目標每塊 Token 數 (≈ 字元數/4)
	ChunkOverlap int // 相鄰塊重疊 Token 數
}

// NewChunker 使用預設參數建立分塊器
func NewChunker() *Chunker {
	return &Chunker{
		ChunkSize:    400,
		ChunkOverlap: 80,
	}
}

// ChunkFile 將指定檔案讀取並分塊
func (c *Chunker) ChunkFile(filePath string) ([]*MemoryChunk, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return c.ChunkText(filePath, string(data)), nil
}

// ChunkText 將文本切分為塊
func (c *Chunker) ChunkText(source string, text string) []*MemoryChunk {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil
	}

	chunkChars := c.ChunkSize * 4 // 粗略將 Token -> 字元 (1 token ≈ 4 chars)
	overlapChars := c.ChunkOverlap * 4

	var chunks []*MemoryChunk
	start := 0

	for start < len(lines) {
		// 累積字元數直到到達目標 chunk 大小
		charCount := 0
		end := start
		for end < len(lines) && charCount < chunkChars {
			charCount += utf8.RuneCountInString(lines[end]) + 1 // +1 for newline
			end++
		}

		// 建立 Chunk
		content := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(content) == "" {
			start = end
			continue
		}

		chunk := &MemoryChunk{
			ID:        fmt.Sprintf("%s:%d-%d", source, start+1, end),
			FilePath:  source,
			StartLine: start + 1,
			EndLine:   end,
			Content:   content,
			Tokens:    estimateTokens(content),
			UpdatedAt: time.Now(),
		}
		chunks = append(chunks, chunk)

		// 滑動窗口：下一塊起始位置 = 當前結束 - 重疊
		overlapLines := 0
		overlapTotal := 0
		for i := end - 1; i >= start && overlapTotal < overlapChars; i-- {
			overlapTotal += utf8.RuneCountInString(lines[i]) + 1
			overlapLines++
		}
		start = end - overlapLines
		if start <= 0 || start == end-overlapLines && overlapLines == end-start {
			start = end // 避免無限迴圈
		}
	}

	return chunks
}

// estimateTokens 粗略估算文本 Token 數
func estimateTokens(text string) int {
	// 英文: ~1 token / 4 chars; 中文: ~1 token / 2 chars
	// 使用保守估算：rune count / 2
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}
	return n/2 + 1
}

// ─────────────────────────────────────────────────────────────
// Indexer — 索引管理器
// ─────────────────────────────────────────────────────────────

// Indexer 負責分塊、Embedding、寫入 SQLite
type Indexer struct {
	mgr     *Manager
	chunker *Chunker
}

// NewIndexer 建立索引器
func NewIndexer(mgr *Manager) *Indexer {
	return &Indexer{
		mgr:     mgr,
		chunker: NewChunker(),
	}
}

// IndexFile 將指定的 Markdown 檔案索引到 SQLite
func (idx *Indexer) IndexFile(ctx context.Context, filePath string) error {
	idx.mgr.mu.Lock()
	defer idx.mgr.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// 計算檔案 Hash
	hash := fmt.Sprintf("%x", sha256.Sum256(data))

	// 檢查是否需要重新索引
	var existingHash string
	err = idx.mgr.db.QueryRowContext(ctx,
		"SELECT value FROM index_meta WHERE key = ?",
		"file_hash:"+filePath,
	).Scan(&existingHash)
	if err == nil && existingHash == hash {
		return nil // 檔案未變更，跳過
	}

	// 刪除舊的 chunks
	if _, err := idx.mgr.db.ExecContext(ctx,
		"DELETE FROM chunks WHERE file_path = ?", filePath); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// 分塊
	chunks := idx.chunker.ChunkText(filePath, string(data))
	if len(chunks) == 0 {
		return nil
	}

	// 批次嵌入
	if idx.mgr.embedder != nil {
		texts := make([]string, len(chunks))
		for i, c := range chunks {
			texts[i] = c.Content
		}

		embeddings, err := idx.getEmbeddingsWithCache(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed: %w", err)
		}

		for i, emb := range embeddings {
			if i < len(chunks) {
				chunks[i].Embedding = emb
			}
		}
	}

	// 寫入 SQLite
	tx, err := idx.mgr.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmtChunk, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO chunks (id, file_path, start_line, end_line, content, search_content, tokens, updated_at, file_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmtChunk.Close()

	stmtEmbed, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO embeddings (chunk_id, provider, model, endpoint, vector, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmtEmbed.Close()

	providerName := "none"
	modelName := ""
	if idx.mgr.embedder != nil {
		providerName = idx.mgr.embedder.Name()
		modelName = idx.mgr.embedder.ModelName()
	}

	for _, c := range chunks {
		if _, err := stmtChunk.ExecContext(ctx,
			c.ID, c.FilePath, c.StartLine, c.EndLine,
			c.Content, cjkSpaced(c.Content), c.Tokens, c.UpdatedAt.Format(time.RFC3339), hash,
		); err != nil {
			return err
		}

		if c.Embedding != nil {
			if _, err := stmtEmbed.ExecContext(ctx,
				c.ID, providerName, modelName, "",
				float32SliceToBytes(c.Embedding), time.Now().Format(time.RFC3339),
			); err != nil {
				return err
			}
		}
	}

	// 更新檔案指紋
	if _, err := tx.ExecContext(ctx,
		"INSERT OR REPLACE INTO index_meta (key, value) VALUES (?, ?)",
		"file_hash:"+filePath, hash,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// IndexAll 對工作區內所有 Markdown 檔案建立索引
func (idx *Indexer) IndexAll(ctx context.Context) error {
	workDir := idx.mgr.cfg.WorkspaceDir

	// MEMORY.md
	memoryMD := filepath.Join(workDir, "MEMORY.md")
	if _, err := os.Stat(memoryMD); err == nil {
		if err := idx.IndexFile(ctx, memoryMD); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ [Memory] 索引 MEMORY.md 失敗: %v\n", err)
		}
	}

	// memory/*.md (每日日誌)
	memoryDir := filepath.Join(workDir, "memory")
	if _, err := os.Stat(memoryDir); err == nil {
		entries, _ := os.ReadDir(memoryDir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				fp := filepath.Join(memoryDir, e.Name())
				if err := idx.IndexFile(ctx, fp); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️ [Memory] 索引 %s 失敗: %v\n", e.Name(), err)
				}
			}
		}
	}

	// Extra paths
	for _, p := range idx.mgr.cfg.Search.ExtraPaths {
		fp := p
		if !filepath.IsAbs(fp) {
			fp = filepath.Join(workDir, fp)
		}
		info, err := os.Stat(fp)
		if err != nil {
			continue
		}
		if info.IsDir() {
			entries, _ := os.ReadDir(fp)
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					if err := idx.IndexFile(ctx, filepath.Join(fp, e.Name())); err != nil {
						fmt.Fprintf(os.Stderr, "⚠️ [Memory] 索引 %s 失敗: %v\n", e.Name(), err)
					}
				}
			}
		} else if strings.HasSuffix(fp, ".md") {
			if err := idx.IndexFile(ctx, fp); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ [Memory] 索引 %s 失敗: %v\n", filepath.Base(fp), err)
			}
		}
	}

	return nil
}

// getEmbeddingsWithCache 利用 Cache 減少 Embedding API 呼叫
func (idx *Indexer) getEmbeddingsWithCache(ctx context.Context, texts []string) ([][]float32, error) {
	if idx.mgr.embedder == nil {
		return make([][]float32, len(texts)), nil
	}

	results := make([][]float32, len(texts))
	providerName := idx.mgr.embedder.Name()
	modelName := idx.mgr.embedder.ModelName()

	// Phase 1: 查詢 Cache
	var uncachedIndices []int
	var uncachedTexts []string

	if idx.mgr.cfg.Search.Cache.Enabled {
		for i, text := range texts {
			hash := contentHash(text, providerName, modelName)
			var blob []byte
			err := idx.mgr.db.QueryRowContext(ctx,
				"SELECT vector FROM embedding_cache WHERE content_hash = ?", hash,
			).Scan(&blob)
			if err == nil {
				results[i] = bytesToFloat32Slice(blob)
			} else {
				uncachedIndices = append(uncachedIndices, i)
				uncachedTexts = append(uncachedTexts, text)
			}
		}
	} else {
		for i, text := range texts {
			uncachedIndices = append(uncachedIndices, i)
			uncachedTexts = append(uncachedTexts, text)
			_ = text
		}
	}

	if len(uncachedTexts) == 0 {
		return results, nil
	}

	// Phase 2: 批次 Embed 未快取的文本
	embeddings, err := idx.mgr.embedder.Embed(ctx, uncachedTexts)
	if err != nil {
		return nil, err
	}

	// Phase 3: 寫入 Cache + 組裝結果
	for j, idx2 := range uncachedIndices {
		if j < len(embeddings) {
			results[idx2] = embeddings[j]

			// 寫入 Cache (非阻塞)
			if idx.mgr.cfg.Search.Cache.Enabled {
				hash := contentHash(uncachedTexts[j], providerName, modelName)
				_, _ = idx.mgr.db.ExecContext(ctx,
					`INSERT OR REPLACE INTO embedding_cache (content_hash, provider, model, vector, created_at)
					 VALUES (?, ?, ?, ?, ?)`,
					hash, providerName, modelName,
					float32SliceToBytes(embeddings[j]), time.Now().Format(time.RFC3339),
				)
			}
		}
	}

	return results, nil
}
