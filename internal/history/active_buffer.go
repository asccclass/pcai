package history

import (
	"fmt"
	"strings"
	"sync"

	"github.com/asccclass/pcai/llms/ollama"
)

// ActiveBuffer 負責維護當前對話的 Active Context
type ActiveBuffer struct {
	mu           sync.RWMutex
	Messages     []ollama.Message
	TokenLimit   int
	Summarized30 bool
	Logger       *DailyLogger
}

// NewActiveBuffer 建立一個新的 Active Buffer
func NewActiveBuffer(limit int, logger *DailyLogger) *ActiveBuffer {
	if limit <= 0 {
		limit = 4000 // 預設 4000 tokens
	}
	return &ActiveBuffer{
		Messages:   []ollama.Message{},
		TokenLimit: limit,
		Logger:     logger,
	}
}

// Add 加入訊息到 Buffer
func (b *ActiveBuffer) Add(msg ollama.Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Messages = append(b.Messages, msg)
}

// GetMessages 取得所有訊息
func (b *ActiveBuffer) GetMessages() []ollama.Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Messages
}

// Clear 清空 Buffer
func (b *ActiveBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Messages = []ollama.Message{}
}

// EstimateTokens 估算當前 Buffer 的 Token 數 (簡單估算法：字元數 / 4)
func (b *ActiveBuffer) EstimateTokens() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	totalChars := 0
	for _, msg := range b.Messages {
		totalChars += len(msg.Content)
	}
	return totalChars / 4
}

// ShouldSummarize 判斷是否需要進行自動歸納
func (b *ActiveBuffer) ShouldSummarize() bool {
	return b.EstimateTokens() >= b.TokenLimit
}

// TriggerSummarization 執行歸納邏輯
// modelName: 用於歸納的模型
// summarizeFunc: 呼叫 LLM 進行歸納的函式
func (b *ActiveBuffer) TriggerSummarization(modelName string, summarizeFunc func(model string, prompt string) (string, error)) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	total := len(b.Messages)
	if total < 5 {
		return nil // 訊息太少不歸納
	}

	// 取出最舊的 30%
	count := int(float64(total) * 0.3)
	if count == 0 {
		count = 1
	}

	toSummarize := b.Messages[:count]
	remaining := b.Messages[count:]

	// 準備歸納 Prompt
	var sb strings.Builder
	sb.WriteString("請幫我精煉以下對話內容，保留核心資訊與結論：\n\n")
	for _, m := range toSummarize {
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	summary, err := summarizeFunc(modelName, sb.String())
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}

	// 建立 Summary 訊息
	summaryMsg := ollama.Message{
		Role:    "system",
		Content: fmt.Sprintf("【對話歸納】\n%s", summary),
	}

	// 更新 Messages: [Summary] + [Remaining]
	b.Messages = append([]ollama.Message{summaryMsg}, remaining...)

	// 記錄總結到日誌
	if b.Logger != nil {
		_ = b.Logger.Record(summaryMsg)
	}

	return nil
}
