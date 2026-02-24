// Package memory — 自適應檢索
// 移植自 memory-lancedb-pro (adaptive-retrieval.ts)
// 決定是否跳過記憶搜尋：過濾寒暄、系統指令、純表情等
package memory

import (
	"regexp"
	"strings"
	"unicode"
)

// ─────────────────────────────────────────────────────────────
// 跳過模式（明確不需要記憶檢索的查詢）
// ─────────────────────────────────────────────────────────────

var skipPatterns = []*regexp.Regexp{
	// 英文寒暄
	regexp.MustCompile(`(?i)^(hi|hello|hey|good\s*(morning|afternoon|evening|night)|greetings|yo|sup|howdy|what'?s up)\b`),
	// 系統/命令
	regexp.MustCompile(`^/`), // slash commands
	regexp.MustCompile(`(?i)^(run|build|test|ls|cd|git|npm|pip|docker|curl|cat|grep|find|make|sudo)\b`),
	// 簡短確認/否定
	regexp.MustCompile(`(?i)^(yes|no|yep|nope|ok|okay|sure|fine|thanks|thank you|thx|ty|got it|understood|cool|nice|great|good|perfect|awesome)\s*[.!]?$`),
	// 繼續指令
	regexp.MustCompile(`(?i)^(go ahead|continue|proceed|do it|start|begin|next)\s*[.!]?$`),
	// 中文寒暄
	regexp.MustCompile(`^(你好|嗨|哈囉|早安|午安|晚安)\s*[。！]?$`),
	// 中文確認/繼續
	regexp.MustCompile(`^(好的?|可以|行|是|對|沒問題|了解|知道了|實施|開始|繼續)\s*[。！]?$`),
	// HEARTBEAT / 系統
	regexp.MustCompile(`(?i)^HEARTBEAT`),
	regexp.MustCompile(`(?i)^\[System`),
}

// ─────────────────────────────────────────────────────────────
// 強制檢索模式（即使查詢很短也應觸發記憶檢索）
// ─────────────────────────────────────────────────────────────

var forceRetrievePatterns = []*regexp.Regexp{
	// 英文記憶關鍵字
	regexp.MustCompile(`(?i)\b(remember|recall|forgot|memory|memories)\b`),
	regexp.MustCompile(`(?i)\b(last time|before|previously|earlier|yesterday|ago)\b`),
	regexp.MustCompile(`(?i)\b(my (name|email|phone|address|birthday|preference))\b`),
	regexp.MustCompile(`(?i)\b(what did (i|we)|did i (tell|say|mention))\b`),
	// 中文記憶關鍵字
	regexp.MustCompile(`(你記得|之前|上次|以前|還記得|提到過|說過|記住|我叫|我的名字)`),
}

// ─────────────────────────────────────────────────────────────
// CJK 判定
// ─────────────────────────────────────────────────────────────

// hasCJK 判斷字串中是否包含 CJK 字元
func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// isPureEmoji 判斷是否為純 Emoji 字串（不含字母或數字）
func isPureEmoji(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return true
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
		// 只允許空白和非字母數字字元（含 emoji）
	}
	return true
}

// ─────────────────────────────────────────────────────────────
// ShouldSkipRetrieval — 主要判定函式
// ─────────────────────────────────────────────────────────────

// ShouldSkipRetrieval 判斷是否應跳過記憶檢索
// 回傳 true 表示此查詢不需要記憶搜尋
func ShouldSkipRetrieval(query string) bool {
	trimmed := strings.TrimSpace(query)

	// 優先檢查強制檢索模式（比長度檢查更早，確保短 CJK 如「你記得嗎」不被跳過）
	for _, p := range forceRetrievePatterns {
		if p.MatchString(trimmed) {
			return false
		}
	}

	// 太短無意義
	if len([]rune(trimmed)) < 5 {
		return true
	}

	// 匹配跳過模式
	for _, p := range skipPatterns {
		if p.MatchString(trimmed) {
			return true
		}
	}

	// 純 Emoji
	if isPureEmoji(trimmed) {
		return true
	}

	// CJK 感知長度閾值：CJK 字元每字承載更多語義
	runeLen := len([]rune(trimmed))
	minLen := 15
	if hasCJK(trimmed) {
		minLen = 6
	}

	// 非提問的短訊息很可能是指令或確認
	hasQuestion := strings.Contains(trimmed, "?") || strings.Contains(trimmed, "？")
	if runeLen < minLen && !hasQuestion {
		return true
	}

	// 預設：執行檢索
	return false
}
