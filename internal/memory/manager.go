package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry 代表一條記憶
type Entry struct {
	ID      string `json:"id"`
	Content string `json:"content"`

	// 新增：儲存內容的 SHA256 Hash，用於快速比對
	ContentHash string `json:"content_hash"`

	Timestamp time.Time `json:"timestamp"`
	Tags      []string  `json:"tags"`
	Vector    []float32 `json:"vector"`
}

// SearchResult 搜尋結果
type SearchResult struct {
	Entry Entry
	Score float64
}

// Manager 記憶管理器
type Manager struct {
	filePath string
	entries  []Entry
	mu       sync.RWMutex
	embedder Embedder
}

// NewManager 初始化管理器
func NewManager(filePath string, embedder Embedder) *Manager {
	m := &Manager{
		filePath: filePath,
		entries:  []Entry{},
		embedder: embedder,
	}
	// 嘗試自動載入
	_ = m.Load()
	return m
}

// calculateHash 是一個私有輔助函式，用於計算字串的 SHA256
func calculateHash(content string) string {
	// 先去除前後空白，確保 " Hello " 和 "Hello" 視為相同
	trimmed := strings.TrimSpace(content)

	hash := sha256.New()
	hash.Write([]byte(trimmed))
	return hex.EncodeToString(hash.Sum(nil))
}

// Add 新增記憶 (已修改：加入 Hash 計算)
func (m *Manager) Add(content string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 在做昂貴的 Embedding 之前，先計算 Hash
	hash := calculateHash(content)

	// (選用優化) 您甚至可以在這裡再次檢查 Exists，防止重複 Add
	// 但為了保持方法單純，這裡我們只負責寫入

	// 2. 計算向量
	vec, err := m.embedder.GetEmbedding(content)
	if err != nil {
		return err
	}

	entry := Entry{
		ID:          fmt.Sprintf("mem_%d", time.Now().UnixNano()),
		Content:     content,
		ContentHash: hash, // 儲存計算好的 Hash
		Timestamp:   time.Now(),
		Tags:        tags,
		Vector:      vec,
	}

	m.entries = append(m.entries, entry)
	return m.save()
}

// Exists 檢查內容是否存在 (已修改：使用 SHA256 Hash 比對)
func (m *Manager) Exists(content string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. 計算傳入內容的 Hash
	targetHash := calculateHash(content)

	// 2. 比對 Hash (速度極快)
	for _, entry := range m.entries {
		// 如果舊資料沒有 ContentHash (可能是舊版程式產生的 json)，
		// 這裡會是不相符 (entry.ContentHash 為空字串)，
		// 這在過渡期是可以接受的，或者您可以加一個 fallback 比對 Content
		if entry.ContentHash == targetHash {
			return true
		}
	}
	return false
}

// Search 執行混合搜尋
func (m *Manager) Search(query string, topK int, minScore float64) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	queryVec, err := m.embedder.GetEmbedding(query)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	queryKeywords := strings.Fields(strings.ToLower(query))

	for _, entry := range m.entries {
		sim := cosineSimilarity(queryVec, entry.Vector)

		contentLower := strings.ToLower(entry.Content)
		keywordBoost := 0.0
		for _, kw := range queryKeywords {
			if strings.Contains(contentLower, kw) {
				keywordBoost += 0.05
			}
		}

		finalScore := sim + keywordBoost

		if finalScore >= minScore {
			results = append(results, SearchResult{
				Entry: entry,
				Score: finalScore,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// Count 回傳記憶總數
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// save 存檔
func (m *Manager) save() error {
	file, err := os.Create(m.filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(m.entries)
}

// Load 讀檔
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	file, err := os.Open(m.filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	// 使用 Decode 讀取 json，如果舊資料沒有 content_hash 欄位，它會預設為空字串
	return json.NewDecoder(file).Decode(&m.entries)
}

// DeleteByContent 根據內容刪除記憶 (使用 Hash 快速查找)
func (m *Manager) DeleteByContent(content string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	targetHash := calculateHash(content)
	index := -1

	// 1. 尋找目標索引
	for i, entry := range m.entries {
		// 如果有 Hash 就比對 Hash，沒有就比對內容 (兼容舊資料)
		if entry.ContentHash == targetHash || (entry.ContentHash == "" && entry.Content == content) {
			index = i
			break
		}
	}

	// 2. 如果沒找到
	if index == -1 {
		return false, nil // 回傳 false 代表沒東西可刪
	}

	// 3. 從 Slice 中移除 (標準 Golang 刪除 Slice 元素寫法)
	// 將 index 之後的所有元素往前移
	m.entries = append(m.entries[:index], m.entries[index+1:]...)

	// 4. 立即存檔
	return true, m.save()
}

// DeleteByID 根據 ID 刪除 (如果你知道 ID 的話)
func (m *Manager) DeleteByID(id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	index := -1
	for i, entry := range m.entries {
		if entry.ID == id {
			index = i
			break
		}
	}

	if index == -1 {
		return false, nil
	}

	m.entries = append(m.entries[:index], m.entries[index+1:]...)
	return true, m.save()
}
