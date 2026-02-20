package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode"
)

// ─────────────────────────────────────────────────────────────
// SearchEngine — 混合搜尋引擎（BM25 + Vector）
// ─────────────────────────────────────────────────────────────

// SearchEngine 混合搜尋引擎
type SearchEngine struct {
	mgr *Manager
}

// NewSearchEngine 建立搜尋引擎
func NewSearchEngine(mgr *Manager) *SearchEngine {
	return &SearchEngine{mgr: mgr}
}

// Search 執行混合搜尋
func (se *SearchEngine) Search(ctx context.Context, query string, topK int) (*MemorySearchResponse, error) {
	if topK == 0 {
		topK = se.mgr.cfg.Search.Limits.MaxResults
	}
	if topK == 0 {
		topK = 6
	}

	hybrid := se.mgr.cfg.Search.Hybrid
	candidateK := topK * hybrid.CandidateMultiplier
	if candidateK < topK*2 {
		candidateK = topK * 2
	}

	var vectorResults []SearchResult
	var textResults []SearchResult

	// Vector Search (if embedder available)
	if se.mgr.embedder != nil {
		var err error
		vectorResults, err = se.vectorSearch(ctx, query, candidateK)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ [Memory] 向量搜尋失敗: %v\n", err)
		}
	}

	// BM25 Search (always available if FTS5 exists)
	if hybrid.Enabled {
		var err error
		textResults, err = se.bm25Search(ctx, query, candidateK)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ [Memory] BM25 搜尋失敗: %v\n", err)
		}
	}

	// Merge results
	merged := se.mergeResults(vectorResults, textResults, topK, hybrid)

	// Truncate snippets
	maxChars := se.mgr.cfg.Search.Limits.MaxSnippetChars
	if maxChars == 0 {
		maxChars = 700
	}
	for i := range merged {
		if len(merged[i].Chunk.Content) > maxChars {
			merged[i].Chunk.Content = merged[i].Chunk.Content[:maxChars] + "…"
		}
	}

	providerName := "none"
	modelName := ""
	if se.mgr.embedder != nil {
		providerName = se.mgr.embedder.Name()
		modelName = se.mgr.embedder.ModelName()
	}

	return &MemorySearchResponse{
		Results:  merged,
		Backend:  "builtin",
		Provider: providerName,
		Model:    modelName,
	}, nil
}

// vectorSearch 向量餘弦搜尋
func (se *SearchEngine) vectorSearch(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// 取得 query embedding
	embeddings, err := se.mgr.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, nil
	}
	queryVec := embeddings[0]

	// 從 SQLite 讀取所有 embeddings（小規模資料集可行）
	rows, err := se.mgr.db.QueryContext(ctx, `
		SELECT e.chunk_id, e.vector, c.file_path, c.start_line, c.end_line, c.content, c.tokens, c.updated_at
		FROM embeddings e
		JOIN chunks c ON e.chunk_id = c.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var chunkID string
		var blob []byte
		var fp string
		var sl, el, tokens int
		var content, updatedAtStr string

		if err := rows.Scan(&chunkID, &blob, &fp, &sl, &el, &content, &tokens, &updatedAtStr); err != nil {
			continue
		}

		vec := bytesToFloat32Slice(blob)
		score := cosineSimilarity(queryVec, vec)
		if score < 0.1 { // 低相關性門檻
			continue
		}

		ut, _ := time.Parse(time.RFC3339, updatedAtStr)
		results = append(results, SearchResult{
			Chunk: &MemoryChunk{
				ID:        chunkID,
				FilePath:  fp,
				StartLine: sl,
				EndLine:   el,
				Content:   content,
				Tokens:    tokens,
				UpdatedAt: ut,
			},
			VectorScore: score,
			Source:      "memory",
		})
	}

	// 排序 + 截斷
	sortResults(results, func(r SearchResult) float64 { return r.VectorScore })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// bm25Search FTS5 全文搜尋
func (se *SearchEngine) bm25Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	ftsQuery := sanitizeFTS(query)
	if ftsQuery == "" {
		return nil, nil
	}

	rows, err := se.mgr.db.QueryContext(ctx, `
		SELECT c.id, c.file_path, c.start_line, c.end_line, c.content, c.tokens, c.updated_at,
		       bm25(chunks_fts) AS score
		FROM chunks_fts f
		JOIN chunks c ON f.rowid = c.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY score
		LIMIT ?
	`, ftsQuery, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var chunkID string
		var fp string
		var sl, el, tokens int
		var content, updatedAtStr string
		var score float64

		if err := rows.Scan(&chunkID, &fp, &sl, &el, &content, &tokens, &updatedAtStr, &score); err != nil {
			continue
		}

		// BM25 returns negative scores (lower = better), normalize to [0, 1]
		normalizedScore := math.Abs(score)
		if normalizedScore > 50 {
			normalizedScore = 50 // cap
		}
		normalizedScore = normalizedScore / 50 // → [0, 1] higher is better

		ut, _ := time.Parse(time.RFC3339, updatedAtStr)
		results = append(results, SearchResult{
			Chunk: &MemoryChunk{
				ID:        chunkID,
				FilePath:  fp,
				StartLine: sl,
				EndLine:   el,
				Content:   content,
				Tokens:    tokens,
				UpdatedAt: ut,
			},
			TextScore: normalizedScore,
			Source:    "memory",
		})
	}

	return results, nil
}

// mergeResults 加權融合向量 + BM25 搜尋結果
func (se *SearchEngine) mergeResults(vectorResults, textResults []SearchResult, topK int, cfg HybridConfig) []SearchResult {
	seen := make(map[string]*SearchResult)

	// 加入向量搜尋結果
	for _, r := range vectorResults {
		key := r.Chunk.ID
		sr := r
		sr.FinalScore = r.VectorScore * cfg.VectorWeight
		seen[key] = &sr
	}

	// 融合 BM25 搜尋結果
	for _, r := range textResults {
		key := r.Chunk.ID
		if existing, ok := seen[key]; ok {
			existing.TextScore = r.TextScore
			existing.FinalScore += r.TextScore * cfg.TextWeight
		} else {
			sr := r
			sr.FinalScore = r.TextScore * cfg.TextWeight
			seen[key] = &sr
		}
	}

	// 轉為切片並排序
	results := make([]SearchResult, 0, len(seen))
	for _, r := range seen {
		results = append(results, *r)
	}
	sortResults(results, func(r SearchResult) float64 { return r.FinalScore })

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// ─────────────────────────────────────────────────────────────
// 數學工具
// ─────────────────────────────────────────────────────────────

// cosineSimilarity 計算兩個向量的餘弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// sanitizeFTS 清理 FTS5 查詢字串，避免語法錯誤
func sanitizeFTS(query string) string {
	// 移除特殊字元保留文字
	var b strings.Builder
	words := strings.Fields(query)
	for i, w := range words {
		// 過濾掉純標點的 token
		clean := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return r
			}
			return -1
		}, w)
		if clean == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString(`"` + clean + `"`)
	}
	return b.String()
}

// sortResults 按照分數倒序排列搜尋結果
func sortResults(results []SearchResult, scoreFn func(SearchResult) float64) {
	// 使用插入排序（結果集通常較小）
	for i := 1; i < len(results); i++ {
		key := results[i]
		keyScore := scoreFn(key)
		j := i - 1
		for j >= 0 && scoreFn(results[j]) < keyScore {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}
