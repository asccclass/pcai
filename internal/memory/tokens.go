package memory

import (
	"sync"
	"unicode/utf8"

	"github.com/pkoukk/tiktoken-go"
)

var (
	tkm      *tiktoken.Tiktoken
	initOnce sync.Once
)

// getTiktoken 獲取 tiktoken 編碼器 (Lazy Init)
func getTiktoken() *tiktoken.Tiktoken {
	initOnce.Do(func() {
		// OpenAI 的 GPT-3.5/GPT-4 模型通常使用 cl100k_base
		enc, err := tiktoken.GetEncoding("cl100k_base")
		if err == nil {
			tkm = enc
		}
	})
	return tkm
}

// CountTokens 精準計算文本 Token 數量
func CountTokens(text string) int {
	if text == "" {
		return 0
	}
	enc := getTiktoken()
	if enc != nil {
		tokens := enc.Encode(text, nil, nil)
		return len(tokens)
	}

	// Fallback 若無法載入編碼器
	n := utf8.RuneCountInString(text)
	if n == 0 {
		return 0
	}
	return n/2 + 1
}

// TruncateByTokens 根據 Token 數量截斷文本
func TruncateByTokens(text string, maxTokens int) string {
	if text == "" || maxTokens <= 0 {
		return ""
	}
	enc := getTiktoken()
	if enc != nil {
		tokens := enc.Encode(text, nil, nil)
		if len(tokens) > maxTokens {
			tokens = tokens[:maxTokens]
			return enc.Decode(tokens) + "...«已截斷»"
		}
		return text
	}

	// Fallback 若無法載入編碼器
	runes := []rune(text)
	// 粗略估算 1 Token ≈ 2 Runes (對於中文)
	maxRunes := maxTokens * 2
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "...«已截斷»"
	}
	return text
}
