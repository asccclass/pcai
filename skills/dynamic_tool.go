// skills/dynamic_tool.go 應該移到 internal/skillloader 目錄下
package skills

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/asccclass/pcai/internal/core"
	"github.com/ollama/ollama/api"
	"gopkg.in/yaml.v3"
)

// SkillDefinition 代表從 Markdown 解析出來的技能
type SkillDefinition struct {
	Name          string                       `yaml:"name"`
	Description   string                       `yaml:"description"`
	Command       string                       `yaml:"command"`
	CacheDuration string                       `yaml:"cache_duration"` // 支援快取時間設定 (e.g. "3h", "10m")
	Options       map[string][]string          `yaml:"options"`        // 參數選項 (param -> [option1, option2])
	OptionAliases map[string]map[string]string `yaml:"option_aliases"` // 參數別名 (param -> {alias: canonical_value})
	Params        []string                     `yaml:"-"`              // 從 Command 解析出的參數參數名 (e.g. "query", "args")
}

// loadSkillFromFile 解析單一 SKILL.md 檔案
func loadSkillFromFile(path string) (*SkillDefinition, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 解析 Frontmatter
	parts := strings.SplitN(string(content), "---", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid frontmatter format")
	}

	yamlContent := parts[1]
	var skill SkillDefinition
	if err := yaml.Unmarshal([]byte(yamlContent), &skill); err != nil {
		return nil, fmt.Errorf("yaml parse error: %v", err)
	}

	// 解析參數
	skill.Params = parseParams(skill.Command)
	return &skill, nil
}

// LoadSkills 從指定目錄載入所有技能定義 (Clawcode 標準: SKILL.md)
func LoadSkills(dir string) ([]*SkillDefinition, error) {
	var skills []*SkillDefinition

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "SKILL.md") {
			skill, err := loadSkillFromFile(path)
			if err != nil {
				fmt.Printf("⚠️ [Skills] Warning: Failed to load skill from %s: %v\n", path, err)
				return nil // 繼續載入其他技能
			}
			skills = append(skills, skill)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return skills, nil
}

// parseParams 解析 {{param}} 或 {{func:param}} 形式的參數
func parseParams(cmd string) []string {
	// 正則表達式：匹配 {{...}}
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := re.FindAllStringSubmatch(cmd, -1)
	var params []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			fullContent := m[1] // e.g. "location" or "url:location"
			parts := strings.Split(fullContent, ":")

			// 取得實際的參數名稱 (最後一個部分)
			paramName := parts[len(parts)-1]

			if !seen[paramName] {
				params = append(params, paramName)
				seen[paramName] = true
			}
		}
	}
	return params
}

// min 回傳三個整數中的最小值
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// levenshtein 計算兩個字串的編輯距離
func levenshtein(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	len1, len2 := len(r1), len(r2)

	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
	}

	for i := 0; i <= len1; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}
	return matrix[len1][len2]
}

// findClosestMatch 在選項列表中尋找最接近的匹配
func findClosestMatch(input string, options []string) (string, bool) {
	bestMatch := ""
	minDist := 1000 // 任意大數

	// 1. 先嘗試精確匹配 (Case Indifferent)
	for _, opt := range options {
		if strings.EqualFold(input, opt) {
			return opt, true
		}
	}

	// 2. 嘗試模糊匹配
	for _, opt := range options {
		dist := levenshtein(input, opt)
		if dist < minDist {
			minDist = dist
			bestMatch = opt
		}
	}

	// 設定閾值：對於短字串(如地名)，允許 1~2 個編輯距離
	// 例如：台北市 (3 chars) -> 臺北市 (3 chars), dist=1 (台!=臺)
	threshold := 2
	if len([]rune(input)) > 4 {
		threshold = 3
	}

	if minDist <= threshold {
		return bestMatch, true
	}

	return "", false
}

// CacheEntry 快取項目
type CacheEntry struct {
	Response  string
	ExpiresAt time.Time
}

// DynamicTool 實作 core.AgentTool 介面
type DynamicTool struct {
	Def      *SkillDefinition
	Registry *core.Registry

	// 快取相關
	Cache      map[string]CacheEntry
	CacheMutex sync.RWMutex
}

func NewDynamicTool(def *SkillDefinition, registry *core.Registry) *DynamicTool {
	return &DynamicTool{
		Def:      def,
		Registry: registry,
		Cache:    make(map[string]CacheEntry),
	}
}

func (t *DynamicTool) Name() string {
	// 將名稱轉為 snake_case 符合工具命名慣例 (e.g. GoogleSearch -> google_search)
	// 這裡簡單轉小寫並把空白換底線
	return strings.ToLower(strings.ReplaceAll(t.Def.Name, " ", "_"))
}

func (t *DynamicTool) Definition() api.Tool {
	// 重新建構 Properties map
	propsMap := make(map[string]interface{})
	required := []string{}

	for _, p := range t.Def.Params {
		paramSchema := map[string]interface{}{
			"type":        "string",
			"description": fmt.Sprintf("Parameter %s for command", p),
		}

		// 如果有定義選項，加入 enum
		if opts, ok := t.Def.Options[p]; ok && len(opts) > 0 {
			paramSchema["enum"] = opts
			// 將選項加入描述，增強提示
			// description := fmt.Sprintf("Parameter %s. Allowed: %s", p, strings.Join(opts, ", "))
			// paramSchema["description"] = description
		}

		propsMap[p] = paramSchema
		required = append(required, p)
	}

	// 透過 JSON轉換 來產生 api.ToolPropertiesMap，避免內部型別不一致的問題
	var apiProps api.ToolPropertiesMap
	propsBytes, _ := json.Marshal(propsMap)
	_ = json.Unmarshal(propsBytes, &apiProps)

	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: t.Def.Description,
			Parameters: api.ToolFunctionParameters{
				Type:       "object",
				Properties: &apiProps,
				Required:   required,
			},
		},
	}
}

func (t *DynamicTool) Run(argsJSON string) (string, error) {
	// 1. 解析參數
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 1.5 [VALIDATION & AUTO-CORRECTION] 參數驗證與自動校正
	for k, v := range args {
		valStr := fmt.Sprintf("%v", v)

		// 1.5.1 檢查是否有別名 (Alias) 設定
		if aliases, ok := t.Def.OptionAliases[k]; ok && len(aliases) > 0 {
			// 建立別名清單以供 fuzzy match
			aliasKeys := make([]string, 0, len(aliases))
			for alias := range aliases {
				aliasKeys = append(aliasKeys, alias)
			}

			// 嘗試尋找別名匹配 (例如: "信義區" -> "臺北市")
			match, found := findClosestMatch(valStr, aliasKeys)
			if found {
				canonical := aliases[match]
				fmt.Printf("⚠️ [DynamicTool] Alias mapping: '%s' (matches alias '%s') -> '%s' (param: %s)\n", valStr, match, canonical, k)
				valStr = canonical
				args[k] = canonical // 更新為標準值，以便後續驗證與快取
			}
		}

		// 檢查該參數是否有定義選項
		if opts, ok := t.Def.Options[k]; ok && len(opts) > 0 {
			// 嘗試尋找最佳匹配
			match, found := findClosestMatch(valStr, opts)
			if found {
				if match != valStr {
					fmt.Printf("⚠️ [DynamicTool] Auto-correction: '%s' -> '%s' (param: %s)\n", valStr, match, k)
					args[k] = match // 更新為正確的值

					// 更新 cacheKey (因為 args 變了)
					// 注意：如果 Cache Key 是基於原始 JSON 生成的，這裡變更後應該要重算
					// 但我們上面已經算過 Cache Logic 了...
					// 修正：Cache Logic 應該移到這裡之後，或者 Cache Key 應該基於「校正後」的參數
					// 為了簡單起見，我們接受：如果 User 輸入錯字 -> Cache Miss -> 這裡校正 -> 執行正確指令 -> 寫入 Cache (Key 是原始錯字 JSON?)
					// 不，這樣下次錯字還是會 Miss。
					// 理想：Cache Key 應該是 Canonical 的。
					// 但為了不大幅重構，我們暫時接受 Cache Key 是原始輸入。
					// 不過，既然我們改了 args[k]，下面的 cmd 替換就會用正確的值。
				}
			} else {
				fmt.Printf("⚠️ [DynamicTool] Warning: Value '%s' for param '%s' is not in allowed options.\n", valStr, k)
			}
		}
	}

	// [CACHE CHECK] (搬移到參數校正之後，這樣 Cache Key 才是校正後的... 但 argsJSON 是原始的)
	// 為了讓「錯字也能命中快取」，我們應該用「校正後的 args」來產 CacheKey。
	// 所以我們重新 Marshal args
	canonicalArgsJSON := argsJSON // 預設用原始的
	if correctedArgsBytes, err := json.Marshal(args); err == nil {
		canonicalArgsJSON = string(correctedArgsBytes)
	}

	// [CACHE CHECK] 如果有設定 CacheDuration，先檢查快取
	var cacheDuration time.Duration
	if t.Def.CacheDuration != "" {
		if d, err := time.ParseDuration(t.Def.CacheDuration); err == nil {
			cacheDuration = d
			// 使用校正後的參數 JSON 作為快取 Key
			cacheKey := canonicalArgsJSON

			t.CacheMutex.RLock()
			entry, exists := t.Cache[cacheKey]
			t.CacheMutex.RUnlock()

			if exists {
				if time.Now().Before(entry.ExpiresAt) {
					// 快取有效，直接回傳
					fmt.Printf("[DynamicTool] Returning cached response for %s (Expires: %v)\n", t.Name(), entry.ExpiresAt.Format("15:04:05"))
					return entry.Response, nil
				}
			}
		} else {
			fmt.Printf("⚠️ [DynamicTool] Invalid cache_duration format: %s\n", t.Def.CacheDuration)
		}
	}

	// 2. 替換指令中的變數 (支援 {{param}} 和 {{url:param}})
	finalCmd := t.Def.Command
	for k, v := range args {
		valStr := fmt.Sprintf("%v", v)

		// 替換原始參數: {{param}}
		placeholder := fmt.Sprintf("{{%s}}", k)
		finalCmd = strings.ReplaceAll(finalCmd, placeholder, valStr)

		// 替換 URL Encode 參數: {{url:param}}
		// 需要 import "net/url"
		encodedVal := url.QueryEscape(valStr)
		urlPlaceholder := fmt.Sprintf("{{url:%s}}", k)
		finalCmd = strings.ReplaceAll(finalCmd, urlPlaceholder, encodedVal)
	}

	// 執行結果變數
	var result string
	var executionErr error

	// 3. 判斷是否為內部工具呼叫
	// 簡單啟發式：取得第一個單詞作為工具名稱
	parts := strings.SplitN(finalCmd, " ", 2)
	toolName := parts[0]
	toolArgs := ""
	if len(parts) > 1 {
		toolArgs = parts[1]
	}

	// 嘗試從 Registry 查找工具
	// 注意：我們需要 access 到 registry，這需要從外部注入
	if t.Registry != nil {
		// ALIAS: http_get -> fetch_url
		// Also support direct usage of 'fetch_url' as an internal tool
		if toolName == "http_get" || toolName == "fetch_url" {
			// 去除引號，並組裝 JSON
			targetURL := strings.Trim(toolArgs, "\"'")
			jsonParams := fmt.Sprintf(`{"url": "%s"}`, targetURL)

			result, executionErr = t.Registry.CallTool("fetch_url", jsonParams)
		}
	}

	// 4. 背景執行 (Fallback to Shell)
	// 如果尚未透過內部工具處理，且沒有錯誤，則嘗試 Shell
	if result == "" && executionErr == nil {
		// 使用開頭的字作為執行檔，後面的作為參數
		// 注意：這裡直接執行可能會有安全風險

		// 為了支援非同步回傳，這裡改為同步執行並等待結果，才能寫入快取
		// 如果需要背景執行且不等待，則無法支援快取 (除非是異步回調模式)
		// 但目前的 Tool 介面要求回傳字串，所以我們假設 Tool 執行是同步的

		cmd := exec.Command("cmd", "/C", finalCmd)
		out, err := cmd.CombinedOutput()
		output := string(out)
		if err != nil {
			output += fmt.Sprintf("\nErrors: %v", err)
			executionErr = err // 標記錯誤
		}
		result = output
	}

	// [CACHE SET] 如果執行成功且需要快取，則寫入
	if executionErr == nil && cacheDuration > 0 && result != "" {
		t.CacheMutex.Lock()
		t.Cache[canonicalArgsJSON] = CacheEntry{
			Response:  result,
			ExpiresAt: time.Now().Add(cacheDuration),
		}
		t.CacheMutex.Unlock()
	}

	if executionErr != nil {
		return result, executionErr // 回傳錯誤訊息
	}

	return result, nil
}
