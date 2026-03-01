package memory

import (
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────
// TestScoring — 多階段評分管線測試
// ─────────────────────────────────────────────────────────────

func TestScoringRecencyBoost(t *testing.T) {
	now := time.Now()
	results := []SearchResult{
		{
			Chunk:      &MemoryChunk{ID: "old", Content: "old memory", UpdatedAt: now.Add(-30 * 24 * time.Hour), Importance: 0.7},
			FinalScore: 0.5,
		},
		{
			Chunk:      &MemoryChunk{ID: "new", Content: "new memory", UpdatedAt: now.Add(-1 * time.Hour), Importance: 0.7},
			FinalScore: 0.5,
		},
	}

	cfg := DefaultRetrievalConfig()
	boosted := ApplyRecencyBoost(results, cfg)

	// 新記憶應該得到更高分
	var newScore, oldScore float64
	for _, r := range boosted {
		if r.Chunk.ID == "new" {
			newScore = r.FinalScore
		}
		if r.Chunk.ID == "old" {
			oldScore = r.FinalScore
		}
	}

	if newScore <= oldScore {
		t.Errorf("New memory (%.4f) should score higher than old memory (%.4f)", newScore, oldScore)
	}
	t.Logf("RecencyBoost: new=%.4f, old=%.4f", newScore, oldScore)
}

func TestScoringImportanceWeight(t *testing.T) {
	results := []SearchResult{
		{
			Chunk:      &MemoryChunk{ID: "high", Content: "important", Importance: 1.0},
			FinalScore: 0.5,
		},
		{
			Chunk:      &MemoryChunk{ID: "low", Content: "casual", Importance: 0.3},
			FinalScore: 0.5,
		},
	}

	weighted := ApplyImportanceWeight(results)

	var highScore, lowScore float64
	for _, r := range weighted {
		if r.Chunk.ID == "high" {
			highScore = r.FinalScore
		}
		if r.Chunk.ID == "low" {
			lowScore = r.FinalScore
		}
	}

	if highScore <= lowScore {
		t.Errorf("High importance (%.4f) should score higher than low (%.4f)", highScore, lowScore)
	}
	t.Logf("ImportanceWeight: high=%.4f, low=%.4f", highScore, lowScore)
}

func TestScoringLengthNormalization(t *testing.T) {
	shortContent := "短內容"
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "很長的記憶內容段落"
	}

	results := []SearchResult{
		{
			Chunk:      &MemoryChunk{ID: "short", Content: shortContent, Importance: 0.7},
			FinalScore: 0.7,
		},
		{
			Chunk:      &MemoryChunk{ID: "long", Content: longContent, Importance: 0.7},
			FinalScore: 0.7,
		},
	}

	cfg := DefaultRetrievalConfig()
	normalized := ApplyLengthNormalization(results, cfg)

	var shortScore, longScore float64
	for _, r := range normalized {
		if r.Chunk.ID == "short" {
			shortScore = r.FinalScore
		}
		if r.Chunk.ID == "long" {
			longScore = r.FinalScore
		}
	}

	if shortScore <= longScore {
		t.Errorf("Short content (%.4f) should score >= long content (%.4f)", shortScore, longScore)
	}
	t.Logf("LengthNorm: short=%.4f, long=%.4f", shortScore, longScore)
}

func TestScoringTimeDecay(t *testing.T) {
	now := time.Now()
	results := []SearchResult{
		{
			Chunk:      &MemoryChunk{ID: "recent", Content: "recent", UpdatedAt: now, Importance: 0.7},
			FinalScore: 0.8,
		},
		{
			Chunk:      &MemoryChunk{ID: "stale", Content: "stale", UpdatedAt: now.Add(-120 * 24 * time.Hour), Importance: 0.7},
			FinalScore: 0.8,
		},
	}

	cfg := DefaultRetrievalConfig()
	decayed := ApplyTimeDecay(results, cfg)

	var recentScore, staleScore float64
	for _, r := range decayed {
		if r.Chunk.ID == "recent" {
			recentScore = r.FinalScore
		}
		if r.Chunk.ID == "stale" {
			staleScore = r.FinalScore
		}
	}

	if recentScore <= staleScore {
		t.Errorf("Recent memory (%.4f) should score higher than stale (%.4f)", recentScore, staleScore)
	}
	// 底線保證：即使非常舊也至少 50%
	if staleScore < 0.8*0.5 {
		t.Errorf("Stale score (%.4f) should be >= floor %.4f", staleScore, 0.8*0.5)
	}
	t.Logf("TimeDecay: recent=%.4f, stale=%.4f", recentScore, staleScore)
}

func TestScoringHardMinScore(t *testing.T) {
	results := []SearchResult{
		{Chunk: &MemoryChunk{ID: "a"}, FinalScore: 0.5},
		{Chunk: &MemoryChunk{ID: "b"}, FinalScore: 0.2},
		{Chunk: &MemoryChunk{ID: "c"}, FinalScore: 0.1},
	}

	filtered := ApplyHardMinScore(results, 0.35)
	if len(filtered) != 1 {
		t.Errorf("Expected 1 result after hard min score, got %d", len(filtered))
	}
	if filtered[0].Chunk.ID != "a" {
		t.Errorf("Expected result 'a', got '%s'", filtered[0].Chunk.ID)
	}
}

func TestScoringMMRDiversity(t *testing.T) {
	// 兩個非常相似的向量和一個不同的
	vec1 := []float32{1.0, 0.0, 0.0}
	vec2 := []float32{0.99, 0.1, 0.0} // 與 vec1 非常相似
	vec3 := []float32{0.0, 1.0, 0.0}  // 與 vec1 不同

	results := []SearchResult{
		{Chunk: &MemoryChunk{ID: "a", Embedding: vec1}, FinalScore: 0.9},
		{Chunk: &MemoryChunk{ID: "b", Embedding: vec2}, FinalScore: 0.85},
		{Chunk: &MemoryChunk{ID: "c", Embedding: vec3}, FinalScore: 0.8},
	}

	diverse := ApplyMMRDiversity(results, 0.85)

	// a 和 c 應該在前（因為 b 與 a 太相似被降級）
	if len(diverse) != 3 {
		t.Errorf("Expected 3 results, got %d", len(diverse))
	}
	if diverse[0].Chunk.ID != "a" {
		t.Errorf("First result should be 'a', got '%s'", diverse[0].Chunk.ID)
	}
	if diverse[1].Chunk.ID != "c" {
		t.Errorf("Second result should be 'c' (diverse), got '%s'", diverse[1].Chunk.ID)
	}
	t.Logf("MMR order: %s, %s, %s", diverse[0].Chunk.ID, diverse[1].Chunk.ID, diverse[2].Chunk.ID)
}

func TestRunScoringPipeline(t *testing.T) {
	now := time.Now()
	results := []SearchResult{
		{
			Chunk: &MemoryChunk{
				ID: "good", Content: "重要的專案資訊", Importance: 0.9,
				UpdatedAt: now.Add(-2 * 24 * time.Hour),
				Embedding: []float32{1, 0, 0},
			},
			FinalScore: 0.7,
		},
		{
			Chunk: &MemoryChunk{
				ID: "noise", Content: "hello", Importance: 0.5,
				UpdatedAt: now,
				Embedding: []float32{0, 1, 0},
			},
			FinalScore: 0.4,
		},
		{
			Chunk: &MemoryChunk{
				ID: "low", Content: "很低分的結果", Importance: 0.3,
				UpdatedAt: now.Add(-90 * 24 * time.Hour),
				Embedding: []float32{0, 0, 1},
			},
			FinalScore: 0.2,
		},
	}

	cfg := DefaultRetrievalConfig()
	pipeline := RunScoringPipeline(results, cfg)

	// 噪音 "hello" 應該被過濾
	for _, r := range pipeline {
		if r.Chunk.ID == "noise" {
			t.Error("Noise result 'hello' should have been filtered")
		}
	}

	// 低分結果可能被 HardMinScore 過濾
	t.Logf("Pipeline results: %d items", len(pipeline))
	for _, r := range pipeline {
		t.Logf("  %s: %.4f", r.Chunk.ID, r.FinalScore)
	}
}

// ─────────────────────────────────────────────────────────────
// TestNoiseFilter — 噪音過濾器測試
// ─────────────────────────────────────────────────────────────

func TestNoiseFilterDenials(t *testing.T) {
	opts := DefaultNoiseFilterOptions()

	denials := []string{
		"I don't have any information about that",
		"I'm not sure about your preferences",
		"我沒有相關的資料",
		"沒有找到相關記憶",
		"No relevant memories found",
	}

	for _, d := range denials {
		if !IsNoise(d, opts) {
			t.Errorf("Expected denial to be noise: %q", d)
		}
	}
}

func TestNoiseFilterBoilerplate(t *testing.T) {
	opts := DefaultNoiseFilterOptions()

	boilerplate := []string{
		"hello",
		"HEARTBEAT",
		"你好",
		"早安",
	}

	for _, b := range boilerplate {
		if !IsNoise(b, opts) {
			t.Errorf("Expected boilerplate to be noise: %q", b)
		}
	}
}

func TestNoiseFilterValidContent(t *testing.T) {
	opts := DefaultNoiseFilterOptions()

	valid := []string{
		"使用者偏好使用 Go 語言開發後端",
		"The API key for the project is stored in the config file",
		"專案使用 SQLite 作為主要資料庫",
	}

	for _, v := range valid {
		if IsNoise(v, opts) {
			t.Errorf("Expected valid content to NOT be noise: %q", v)
		}
	}
}

func TestNoiseFilterShortText(t *testing.T) {
	opts := DefaultNoiseFilterOptions()
	if !IsNoise("ok", opts) {
		t.Error("Very short text should be noise")
	}
	if !IsNoise("", opts) {
		t.Error("Empty text should be noise")
	}
}

// ─────────────────────────────────────────────────────────────
// TestAdaptiveRetrieval — 自適應檢索測試
// ─────────────────────────────────────────────────────────────

func TestAdaptiveRetrieval_Skip(t *testing.T) {
	skip := []string{
		"hello",
		"hi",
		"/start",
		"ok",
		"yes",
		"HEARTBEAT",
		"好的",
		"繼續",
		"你好",
		"👍",
	}

	for _, q := range skip {
		if !ShouldSkipRetrieval(q) {
			t.Errorf("Expected to skip retrieval for: %q", q)
		}
	}
}

func TestAdaptiveRetrieval_Force(t *testing.T) {
	force := []string{
		"你記得我上次說了什麼嗎",
		"Do you remember my name?",
		"What did I tell you yesterday?",
		"之前我們討論過什麼",
		"我的名字是什麼？",
	}

	for _, q := range force {
		if ShouldSkipRetrieval(q) {
			t.Errorf("Expected to force retrieval for: %q", q)
		}
	}
}

func TestAdaptiveRetrieval_Normal(t *testing.T) {
	normal := []string{
		"請幫我查詢一下專案的架構設計",
		"How does the memory system work?",
		"這個功能的實作邏輯是什麼？",
	}

	for _, q := range normal {
		if ShouldSkipRetrieval(q) {
			t.Errorf("Expected to NOT skip retrieval for normal query: %q", q)
		}
	}
}

func TestAdaptiveRetrieval_CJKThreshold(t *testing.T) {
	// CJK 短查詢（< 6 字元且無問號）
	if !ShouldSkipRetrieval("好吧") {
		t.Error("Very short CJK without question mark should be skipped")
	}

	// CJK 帶問號 (5 runes: 那叫什麼？ — passes 5-char minimum, question mark exempts from 6-char CJK threshold)
	if ShouldSkipRetrieval("那叫什麼？") {
		t.Error("CJK with question mark should NOT be skipped")
	}
}
