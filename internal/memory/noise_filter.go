// Package memory — 噪音過濾器
// 移植自 memory-lancedb-pro (noise-filter.ts)
// 過濾低品質記憶：AI 拒絕回應、後設問題、打招呼/心跳
package memory

import (
	"regexp"
	"strings"
)

// ─────────────────────────────────────────────────────────────
// 噪音模式定義
// ─────────────────────────────────────────────────────────────

// AI 端拒絕模式
var denialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)i don'?t have (any )?(information|data|memory|record)`),
	regexp.MustCompile(`(?i)i'?m not sure about`),
	regexp.MustCompile(`(?i)i don'?t recall`),
	regexp.MustCompile(`(?i)i don'?t remember`),
	regexp.MustCompile(`(?i)it looks like i don'?t`),
	regexp.MustCompile(`(?i)i wasn'?t able to find`),
	regexp.MustCompile(`(?i)no (relevant )?memories found`),
	regexp.MustCompile(`(?i)i don'?t have access to`),
	// 中文版本
	regexp.MustCompile(`我(沒有|找不到|無法)(相關的?)?資(料|訊)`),
	regexp.MustCompile(`我不(確定|記得|清楚)`),
	regexp.MustCompile(`(沒有|未)(找到|搜到)(相關的?)?記憶`),
}

// 使用者端後設問題模式
var metaQuestionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bdo you (remember|recall|know about)\b`),
	regexp.MustCompile(`(?i)\bcan you (remember|recall)\b`),
	regexp.MustCompile(`(?i)\bdid i (tell|mention|say|share)\b`),
	regexp.MustCompile(`(?i)\bhave i (told|mentioned|said)\b`),
	regexp.MustCompile(`(?i)\bwhat did i (tell|say|mention)\b`),
	// 中文版本
	regexp.MustCompile(`你(記得|還記得|記不記得)`),
	regexp.MustCompile(`我(有沒有|是否)(跟你)?(說過|提過|講過)`),
}

// 打招呼/心跳模式
var boilerplatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(hi|hello|hey|good morning|good evening|greetings)`),
	regexp.MustCompile(`(?i)^fresh session`),
	regexp.MustCompile(`(?i)^new session`),
	regexp.MustCompile(`(?i)^HEARTBEAT`),
	// 中文版本
	regexp.MustCompile(`^(你好|嗨|早安|晚安|哈囉)`),
}

// ─────────────────────────────────────────────────────────────
// 噪音判定函式
// ─────────────────────────────────────────────────────────────

// NoiseFilterOptions 噪音過濾選項
type NoiseFilterOptions struct {
	FilterDenials       bool // 過濾 AI 拒絕回應 (預設 true)
	FilterMetaQuestions bool // 過濾後設問題 (預設 true)
	FilterBoilerplate   bool // 過濾打招呼 (預設 true)
}

// DefaultNoiseFilterOptions 回傳預設選項
func DefaultNoiseFilterOptions() NoiseFilterOptions {
	return NoiseFilterOptions{
		FilterDenials:       true,
		FilterMetaQuestions: true,
		FilterBoilerplate:   true,
	}
}

// IsNoise 判斷文本是否為噪音，回傳 true 表示應該被過濾
func IsNoise(text string, opts NoiseFilterOptions) bool {
	trimmed := strings.TrimSpace(text)

	// 太短的文本視為噪音
	if len([]rune(trimmed)) < 5 {
		return true
	}

	if opts.FilterDenials {
		for _, p := range denialPatterns {
			if p.MatchString(trimmed) {
				return true
			}
		}
	}

	if opts.FilterMetaQuestions {
		for _, p := range metaQuestionPatterns {
			if p.MatchString(trimmed) {
				return true
			}
		}
	}

	if opts.FilterBoilerplate {
		for _, p := range boilerplatePatterns {
			if p.MatchString(trimmed) {
				return true
			}
		}
	}

	return false
}

// FilterNoiseFromResults 從搜尋結果中過濾噪音條目
func FilterNoiseFromResults(results []SearchResult) []SearchResult {
	opts := DefaultNoiseFilterOptions()
	filtered := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if !IsNoise(r.Chunk.Content, opts) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
