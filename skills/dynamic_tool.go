// skills/dynamic_tool.go 應該移到 internal/skillloader 目錄下
package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/ollama/ollama/api"
	"gopkg.in/yaml.v3"
)

// SkillDefinition 代表從 Markdown 解析出來的技能
type SkillDefinition struct {
	Name          string                       `yaml:"name"`
	Description   string                       `yaml:"description"`
	Command       string                       `yaml:"command"`        // 入口指令 (e.g. "python main.py {{args}}")
	Image         string                       `yaml:"image"`          // Docker Image (e.g. "python:3.9-slim"), 若為空則使用本地 Shell
	CacheDuration string                       `yaml:"cache_duration"` // 支援快取時間設定 (e.g. "3h", "10m")
	Options       map[string][]string          `yaml:"options"`        // 參數選項 (param -> [option1, option2])
	OptionAliases map[string]map[string]string `yaml:"option_aliases"` // 參數別名 (param -> {alias: canonical_value})
	Params        []string                     `yaml:"-"`              // 從 Command 解析出的參數參數名 (e.g. "query", "args")
	RepoPath      string                       `yaml:"-"`              // 本地代碼路徑 (包含 SKILL.md 的目錄)
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

	// 設定 RepoPath (SKILL.md 所在的目錄)
	skill.RepoPath = filepath.Dir(path)

	// 解析參數
	skill.Params = ParseParams(skill.Command)
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

// ParseParams 解析 {{param}} 或 {{func:param}} 形式的參數
func ParseParams(cmd string) []string {
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

	// 設定閾值：對於短字串(如地名)，允許編輯距離
	// 例如：台北市 (3 chars) -> 臺北市 (3 chars), dist=1 (台!=臺)
	// 林口 (2 chars) -> 宜蘭 (2 chars), dist=2 -> 不應匹配
	threshold := 2
	inputLen := len([]rune(input))
	if inputLen <= 2 {
		threshold = 1 // 2字以下只允許錯1字 (e.g. 台南->臺南)
	} else if inputLen > 4 {
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
	Def          *SkillDefinition
	Registry     *core.Registry
	DockerClient *client.Client // 支援 Docker 執行

	// 快取相關
	Cache      map[string]CacheEntry
	CacheMutex sync.RWMutex
}

func NewDynamicTool(def *SkillDefinition, registry *core.Registry, dockerCli *client.Client) *DynamicTool {
	return &DynamicTool{
		Def:          def,
		Registry:     registry,
		DockerClient: dockerCli,
		Cache:        make(map[string]CacheEntry),
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

// getStringValue extracts string from potential complex structures
func getStringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		// Check for {"value": "..."} or {"type": "...", "value": "..."}
		if v, ok := val["value"]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	// Fallback to default string representation
	return fmt.Sprintf("%v", v)
}

func (t *DynamicTool) Run(argsJSON string) (string, error) {
	// 1. 解析參數
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 1.5 [VALIDATION & AUTO-CORRECTION] 參數驗證與自動校正
	for k, v := range args {
		// valStr := fmt.Sprintf("%v", v)
		// [FIX] Handle complex value types (e.g. {"type":"string", "value":"..."})
		valStr := getStringValue(v)

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

		// Update args with normalized string anyway, so subsequent logic sees string
		args[k] = valStr

		// 檢查該參數是否有定義選項
		if opts, ok := t.Def.Options[k]; ok && len(opts) > 0 {
			// 嘗試尋找最佳匹配
			match, found := findClosestMatch(valStr, opts)
			if found {
				if match != valStr {
					fmt.Printf("⚠️ [DynamicTool] Auto-correction: '%s' -> '%s' (param: %s)\n", valStr, match, k)
					args[k] = match // 更新為正確的值
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
		valStr := getStringValue(v)

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

	// 3. 判斷是否為內部工具呼叫 (Registry Call)
	// 如果 Registry 存在且命令以 http_get 等開頭，則走內部工具
	// 規則：如果 Def.Image 有設定，則強制走 Docker。否則嘗試 Registry -> Shell。
	useDocker := t.DockerClient != nil && t.Def.Image != ""

	if !useDocker {
		// 簡單啟發式：取得第一個單詞作為工具名稱
		parts := strings.SplitN(finalCmd, " ", 2)
		toolName := parts[0]
		toolArgs := ""
		if len(parts) > 1 {
			toolArgs = parts[1]
		}

		// 嘗試從 Registry 查找工具
		if t.Registry != nil {
			// ALIAS: http_get -> web_fetch (fetch_url is deprecated name)
			if toolName == "http_get" || toolName == "fetch_url" || toolName == "web_fetch" {
				targetURL := strings.Trim(toolArgs, "\"'")
				// 支援帶有參數的 url (e.g. web_fetch "http://..." --extractMode text)
				// 但這裡簡單處理，假設只有 URL
				// 如果 toolArgs 包含空白，可能需要更複雜的解析。
				// 目前 SKILL.md 中是 dynamic replacement，所以通常是一長串 URL。

				// 移除可能的 ExtractMode 參數如果混在裡面?
				// 暫時假設 SKILL.md 只傳 URL

				jsonParams := fmt.Sprintf(`{"url": "%s"}`, targetURL)
				// Call 'web_fetch' tool (registered name)
				result, executionErr = t.Registry.CallTool("web_fetch", jsonParams)
			}
		}
	} else {
		// --- Docker Execution (Sidecar Mode) ---
		fmt.Printf("🚀 [DynamicSkill] Executing %s in Docker (Image: %s)...\n", t.Name(), t.Def.Image)
		ctx := context.Background()

		// 1. 檢查映像檔
		_, _, err := t.DockerClient.ImageInspectWithRaw(ctx, t.Def.Image)
		if client.IsErrNotFound(err) {
			fmt.Printf("Image %s not found, pulling...\n", t.Def.Image)
			reader, err := t.DockerClient.ImagePull(ctx, t.Def.Image, types.ImagePullOptions{})
			if err != nil {
				return "", fmt.Errorf("pull image failed: %v", err)
			}
			defer reader.Close()
			io.Copy(io.Discard, reader)
		} else if err != nil {
			return "", fmt.Errorf("inspect image failed: %v", err)
		}

		// 2. 配置容器
		repoAbsPath, _ := filepath.Abs(t.Def.RepoPath)

		// 修正 Command: finalCmd 是替換過變數的完整指令字串 (e.g. "python main.py arg1")
		// Docker Cmd 預期是 []string。我們用 sh -c 來執行複雜指令
		cmdSlice := []string{"sh", "-c", finalCmd}

		containerConfig := &container.Config{
			Image:           t.Def.Image,
			Cmd:             cmdSlice,
			NetworkDisabled: false, // 允許聯網
			WorkingDir:      "/app",
		}

		hostConfig := &container.HostConfig{
			Binds: []string{
				fmt.Sprintf("%s:/app:ro", repoAbsPath), // 掛載 RepoPath 到 /app (Read-Only)
			},
			AutoRemove: false,                                          // 必須設為 false，否則執行完瞬間就被刪除，讀不到 logs
			Resources:  container.Resources{Memory: 256 * 1024 * 1024}, // 256MB
		}

		// 3. 建立並啟動
		resp, err := t.DockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
		if err != nil {
			return "", fmt.Errorf("container create failed: %v", err)
		}

		// 確保清理
		defer func() {
			_ = t.DockerClient.ContainerRemove(context.Background(), resp.ID, types.ContainerRemoveOptions{Force: true})
		}()

		if err := t.DockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
			return "", fmt.Errorf("container start failed: %v", err)
		}

		// 4. 等待結果
		statusCh, errCh := t.DockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			executionErr = fmt.Errorf("container error: %v", err)
		case <-statusCh:
			// Success, read logs
		case <-time.After(60 * time.Second): // Timeout
			_ = t.DockerClient.ContainerKill(ctx, resp.ID, "SIGKILL")
			executionErr = fmt.Errorf("timeout")
		}

		// 5. 讀取 Logs
		if executionErr == nil || executionErr.Error() == "timeout" {
			out, err := t.DockerClient.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
			if err == nil {
				defer out.Close()
				var stdout, stderr bytes.Buffer
				// 使用 stdcopy 區分 stdout/stderr
				stdcopy.StdCopy(&stdout, &stderr, out)

				resOut := stdout.String()
				resErr := stderr.String()

				if resErr != "" {
					result = fmt.Sprintf("%s\n(Stderr: %s)", resOut, resErr)
				} else {
					result = resOut
				}
			}
		}
	}

	// 4. 背景執行 (Fallback to Shell)
	if result == "" && executionErr == nil && !useDocker {
		// 使用開頭的字作為執行檔，後面的作為參數
		// 注意：這裡直接執行可能會有安全風險
		cmd := exec.Command("cmd", "/C", finalCmd)
		out, err := cmd.CombinedOutput()
		output := string(out)
		if err != nil {
			output += fmt.Sprintf("\nErrors: %v", err)
			executionErr = err
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

	// [POST-PROCESS] 針對特定 Skill 進行輸出後處理
	if t.Def.Name == "read_calendars" {
		result = postProcessCalendarOutput(result)
	}

	return result, nil
}
