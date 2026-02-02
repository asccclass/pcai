// pkg/memory/vector.go (數學工具) 這裡放置純粹的數學運算，不涉及業務邏輯。

package memory

import "math"

// cosineSimilarity 計算兩個向量的餘弦相似度
// 這是 private 函式，因為只有這個 package 內部需要用到
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
