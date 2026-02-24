// Package memory — 多階段評分管線
// 移植自 memory-lancedb-pro (retriever.ts)
// 在混合搜尋（BM25 + Vector）融合後，依序套用多個評分階段
package memory

import (
	"math"
	"sort"
	"time"
)

// ─────────────────────────────────────────────────────────────
// RetrievalConfig — 檢索管線配置
// ─────────────────────────────────────────────────────────────

// RetrievalConfig 控制多階段評分管線的行為
type RetrievalConfig struct {
	// RRF 融合權重
	VectorWeight float64 `json:"vectorWeight"` // 預設 0.7
	BM25Weight   float64 `json:"bm25Weight"`   // 預設 0.3
	MinScore     float64 `json:"minScore"`     // 融合後最低分 (預設 0.3)

	// 硬性最低分數：全階段完成後低於此值的結果會被丟棄 (預設 0.35)
	HardMinScore float64 `json:"hardMinScore"`

	// 新鮮度加成：較新的記憶得到加法式加分
	// 公式: boost = exp(-ageDays / halfLife) * weight
	RecencyHalfLifeDays float64 `json:"recencyHalfLifeDays"` // 預設 14，設 0 停用
	RecencyWeight       float64 `json:"recencyWeight"`       // 預設 0.10

	// 長度正規化：懲罰過長條目以避免關鍵字密度主導
	// 公式: score *= 1 / (1 + 0.5 * log2(max(charLen/anchor, 1)))
	LengthNormAnchor int `json:"lengthNormAnchor"` // 預設 500，設 0 停用

	// 時間衰減：乘法式懲罰陳舊條目
	// 公式: score *= 0.5 + 0.5 * exp(-ageDays / halfLife)
	// 0 天: 1.0x, halfLife: ~0.68x, 2*halfLife: ~0.59x, 底線 0.5x
	TimeDecayHalfLifeDays float64 `json:"timeDecayHalfLifeDays"` // 預設 60，設 0 停用

	// MMR 去重：避免前 K 名都是近似重複
	// 餘弦相似度 > 此閾值的候選將被降級
	MMRThreshold float64 `json:"mmrThreshold"` // 預設 0.85

	// 噪音過濾：啟用後過濾低品質結果
	FilterNoise bool `json:"filterNoise"` // 預設 true
}

// DefaultRetrievalConfig 回傳推薦的預設配置
func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		VectorWeight:          0.7,
		BM25Weight:            0.3,
		MinScore:              0.3,
		HardMinScore:          0.35,
		RecencyHalfLifeDays:   14,
		RecencyWeight:         0.10,
		LengthNormAnchor:      500,
		TimeDecayHalfLifeDays: 60,
		MMRThreshold:          0.85,
		FilterNoise:           true,
	}
}

// ─────────────────────────────────────────────────────────────
// 輔助函式
// ─────────────────────────────────────────────────────────────

// clamp01 將值限制在 [0, 1] 範圍內，若非有限數則使用 fallback
func clamp01(value, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		if math.IsNaN(fallback) || math.IsInf(fallback, 0) {
			return 0
		}
		return fallback
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// ─────────────────────────────────────────────────────────────
// 多階段評分函式
// ─────────────────────────────────────────────────────────────

// ApplyRecencyBoost 新鮮度加成：較新的記憶得到額外加分
// 公式: boost = exp(-ageDays / halfLife) * weight
func ApplyRecencyBoost(results []SearchResult, cfg RetrievalConfig) []SearchResult {
	if cfg.RecencyHalfLifeDays <= 0 || cfg.RecencyWeight <= 0 {
		return results
	}

	now := time.Now()
	for i := range results {
		ts := results[i].Chunk.UpdatedAt
		if ts.IsZero() {
			ts = now
		}
		ageDays := now.Sub(ts).Hours() / 24.0
		boost := math.Exp(-ageDays/cfg.RecencyHalfLifeDays) * cfg.RecencyWeight
		results[i].FinalScore = clamp01(results[i].FinalScore+boost, results[i].FinalScore)
		results[i].RecencyBoost = boost
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	return results
}

// ApplyImportanceWeight 重要度加權：高重要度的記憶得分更高
// 公式: score *= (0.7 + 0.3 * importance)
// importance=1.0 → ×1.0, importance=0.5 → ×0.85, importance=0.0 → ×0.7
func ApplyImportanceWeight(results []SearchResult) []SearchResult {
	const baseWeight = 0.7
	for i := range results {
		importance := results[i].Chunk.Importance
		if importance <= 0 {
			importance = 0.7 // 預設重要度
		}
		factor := baseWeight + (1-baseWeight)*importance
		results[i].FinalScore = clamp01(results[i].FinalScore*factor, results[i].FinalScore*baseWeight)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	return results
}

// ApplyLengthNormalization 長度正規化：懲罰過長條目
// 公式: score *= 1 / (1 + 0.5 * log2(max(charLen/anchor, 1)))
func ApplyLengthNormalization(results []SearchResult, cfg RetrievalConfig) []SearchResult {
	anchor := cfg.LengthNormAnchor
	if anchor <= 0 {
		return results
	}

	for i := range results {
		charLen := len([]rune(results[i].Chunk.Content))
		ratio := float64(charLen) / float64(anchor)
		if ratio < 1 {
			ratio = 1 // 不加分短條目
		}
		logRatio := math.Log2(ratio)
		factor := 1.0 / (1.0 + 0.5*logRatio)
		results[i].FinalScore = clamp01(results[i].FinalScore*factor, results[i].FinalScore*0.3)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	return results
}

// ApplyTimeDecay 時間衰減：乘法式懲罰陳舊條目
// 公式: score *= 0.5 + 0.5 * exp(-ageDays / halfLife)
// 底線保證即使非常舊的記憶也至少保留 50% 分數
func ApplyTimeDecay(results []SearchResult, cfg RetrievalConfig) []SearchResult {
	halfLife := cfg.TimeDecayHalfLifeDays
	if halfLife <= 0 {
		return results
	}

	now := time.Now()
	for i := range results {
		ts := results[i].Chunk.UpdatedAt
		if ts.IsZero() {
			ts = now
		}
		ageDays := now.Sub(ts).Hours() / 24.0
		factor := 0.5 + 0.5*math.Exp(-ageDays/halfLife)
		results[i].FinalScore = clamp01(results[i].FinalScore*factor, results[i].FinalScore*0.5)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})
	return results
}

// ApplyHardMinScore 硬性最低分數過濾：丟棄所有低於閾值的結果
func ApplyHardMinScore(results []SearchResult, threshold float64) []SearchResult {
	if threshold <= 0 {
		return results
	}
	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if r.FinalScore >= threshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// ApplyMMRDiversity MMR 去重：避免前 K 名充滿近似重複的記憶
// 使用向量餘弦相似度判斷。超過 threshold 的候選者會被降級到尾端。
func ApplyMMRDiversity(results []SearchResult, threshold float64) []SearchResult {
	if len(results) <= 1 || threshold <= 0 {
		return results
	}

	selected := make([]SearchResult, 0, len(results))
	deferred := make([]SearchResult, 0)

	for _, candidate := range results {
		tooSimilar := false

		// 檢查候選者是否與已選結果過於相似
		for _, s := range selected {
			sVec := s.Chunk.Embedding
			cVec := candidate.Chunk.Embedding
			if len(sVec) == 0 || len(cVec) == 0 || len(sVec) != len(cVec) {
				continue
			}
			sim := cosineSimilarity(sVec, cVec)
			if sim > threshold {
				tooSimilar = true
				break
			}
		}

		if tooSimilar {
			deferred = append(deferred, candidate)
		} else {
			selected = append(selected, candidate)
		}
	}

	// 降級的結果附加在尾端（仍可使用但優先級低）
	return append(selected, deferred...)
}

// RunScoringPipeline 執行完整的多階段評分管線
// 在 mergeResults 之後呼叫此函式
func RunScoringPipeline(results []SearchResult, cfg RetrievalConfig) []SearchResult {
	// 1. 新鮮度加成
	results = ApplyRecencyBoost(results, cfg)

	// 2. 重要度加權
	results = ApplyImportanceWeight(results)

	// 3. 長度正規化
	results = ApplyLengthNormalization(results, cfg)

	// 4. 時間衰減
	results = ApplyTimeDecay(results, cfg)

	// 5. 硬性最低分數
	results = ApplyHardMinScore(results, cfg.HardMinScore)

	// 6. 噪音過濾
	if cfg.FilterNoise {
		results = FilterNoiseFromResults(results)
	}

	// 7. MMR 去重
	results = ApplyMMRDiversity(results, cfg.MMRThreshold)

	return results
}
