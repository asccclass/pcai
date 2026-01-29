package aicontext

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/asccclass/pcai/tools"
	"github.com/google/uuid"
	"github.com/ollama/ollama/api"
)

// =======================
// 1. 資料結構定義
// =======================

type Topic struct {
	ID         string        `json:"id"`
	Summary    string        `json:"summary"`     // AI 生成的主題摘要
	History    []api.Message `json:"history"`     // 對話內容 (使用 ollama api.Message)
	LastActive time.Time     `json:"last_active"` // 用於 LRU 排序
}

type TopicManager struct {
	Topics      map[string]*Topic
	LastTopicID string // 紀錄最後一次活躍的主題 ID
	DataDir     string // 資料儲存目錄
	MaxTopics   int
	Model       string
	Client      *api.Client         // Ollama Client
	Registry    *tools.ToolRegistry // 工具註冊表
	mu          sync.Mutex          // 確保併發安全
}

// =======================
// 2. 初始化與持久化邏輯
// =======================

func NewTopicManager(dataDir, model string, max int, registry *tools.ToolRegistry) (*TopicManager, error) {
	// 建立 Client
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, fmt.Errorf("無法連接 Ollama: %v", err)
	}

	// 確保資料夾存在
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		os.Mkdir(dataDir, 0755)
	}

	tm := &TopicManager{
		Topics:    make(map[string]*Topic),
		DataDir:   dataDir,
		MaxTopics: max,
		Model:     model,
		Client:    client,
		Registry:  registry,
	}

	// 啟動時載入硬碟中的舊資料
	tm.loadFromDisk()
	return tm, nil
}

// loadFromDisk 從 JSON 檔案載入對話
func (tm *TopicManager) loadFromDisk() {
	files, err := os.ReadDir(tm.DataDir)
	if err != nil {
		log.Println("讀取資料夾失敗:", err)
		return
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			data, err := os.ReadFile(filepath.Join(tm.DataDir, f.Name()))
			if err == nil {
				var t Topic
				if err := json.Unmarshal(data, &t); err == nil {
					tm.Topics[t.ID] = &t
				}
			}
		}
	}
	// 設定 LastTopicID 為時間最近的一個
	tm.updateLastActiveTracker()
	fmt.Printf(" [系統] 已載入 %d 個歷史主題。\n", len(tm.Topics))
}

// saveTopic 儲存單一主題到硬碟
func (tm *TopicManager) saveTopic(t *Topic) {
	filename := filepath.Join(tm.DataDir, t.ID+".json")
	data, _ := json.MarshalIndent(t, "", "  ")
	os.WriteFile(filename, data, 0644)
}

// deleteTopic 刪除主題與檔案
func (tm *TopicManager) deleteTopic(id string) {
	delete(tm.Topics, id)
	os.Remove(filepath.Join(tm.DataDir, id+".json"))
}

// =======================
// 3. 核心邏輯：路由與管理
// =======================

// HandleInput 處理使用者輸入的主入口
func (tm *TopicManager) HandleInput(input string) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 步驟 A: 模糊語句過濾 (Heuristic Check)
	if tm.isAmbiguousInput(input) && tm.LastTopicID != "" {
		fmt.Println(" [路由] 偵測到模糊語句，沿用上一主題。")
		return tm.continueTopic(tm.LastTopicID, input)
	}

	// 步驟 B: AI 語義路由 (Semantic Routing)
	topicID := tm.findMatchingTopic(input)

	if topicID == "NEW" || topicID == "" {
		// 建立新主題
		return tm.createNewTopic(input)
	}

	// 步驟 C: 沿用現有主題
	return tm.continueTopic(topicID, input)
}

// Chat satisfies the ChatAgent interface
func (tm *TopicManager) Chat(input string) (string, error) {
	return tm.HandleInput(input)
}

// createNewTopic 建立新主題
func (tm *TopicManager) createNewTopic(input string) (string, error) {
	// 1. 檢查是否超過上限，執行 LRU 清理
	if len(tm.Topics) >= tm.MaxTopics {
		tm.evictOldest()
	}

	// 2. 產生 ID
	newID := uuid.New().String()

	// 3. 呼叫 AI 生成摘要
	summary := tm.generateSummary(input)

	// 4. 設定 System Prompt (這裡可以設定全域 Prompt)
	systemPrompt := "你是一個專業的繁體中文 AI 助手。無論使用者用什麼語言提問，你都必須使用台灣繁體中文（Traditional Chinese, Taiwan）進行回答。請語氣親切、專業。"

	topic := &Topic{
		ID:      newID,
		Summary: summary,
		History: []api.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input},
		},
		LastActive: time.Now(),
	}

	tm.Topics[newID] = topic
	tm.LastTopicID = newID
	tm.saveTopic(topic)

	fmt.Printf(" [系統] 新建主題: %s\n", summary)

	// 5. 執行對話並回應
	return tm.runChatLoop(topic)
}

// continueTopic 繼續現有主題
func (tm *TopicManager) continueTopic(id, input string) (string, error) {
	topic, exists := tm.Topics[id]
	if !exists {
		return tm.createNewTopic(input) // 防呆
	}

	// 更新狀態
	topic.LastActive = time.Now()
	topic.History = append(topic.History, api.Message{Role: "user", Content: input})
	tm.LastTopicID = id
	tm.saveTopic(topic)

	fmt.Printf(" [系統] 延續主題: %s\n", topic.Summary)

	// 執行對話並回應
	return tm.runChatLoop(topic)
}

// runChatLoop 執行對話迴圈 (包含 Tool Execution Logic)
func (tm *TopicManager) runChatLoop(topic *Topic) (string, error) {
	ctx := context.Background()
	stream := false

	// 使用迴圈處理可能的多次 Tool Call
	for {
		// 每次請求都傳送完整的 History (注意：若 History 過長可能需要修剪，這裡暫時保留 Memory.go 的 Prune 概念但由 TopicManager 管理比較複雜，這裡先全部送出或後續實作 Prune)
		// 這裡建議加上 Prune 機制，避免 Token 過多
		tm.pruneHistory(topic)

		req := &api.ChatRequest{
			Model:    tm.Model,
			Messages: topic.History,
			Tools:    tm.Registry.GetDefinitions(),
			Stream:   &stream,
		}

		var resp api.ChatResponse
		err := tm.Client.Chat(ctx, req, func(r api.ChatResponse) error {
			resp = r
			return nil
		})
		if err != nil {
			return "", err
		}

		// 將 AI 的回答加入歷史紀錄
		topic.History = append(topic.History, resp.Message)
		tm.saveTopic(topic) // 保存狀態

		// 檢查是否有 Tool Calls
		if len(resp.Message.ToolCalls) > 0 {
			fmt.Println("🤖 AI Using Tools...")
			for _, toolCall := range resp.Message.ToolCalls {
				fmt.Printf("   -> %s\n", toolCall.Function.Name)

				// 準備參數
				argsBytes, err := json.Marshal(toolCall.Function.Arguments)
				if err != nil {
					fmt.Printf("Error marshaling args: %v\n", err)
					continue
				}

				// 執行工具
				result, err := tm.Registry.Execute(toolCall.Function.Name, string(argsBytes))
				if err != nil {
					result = fmt.Sprintf("Error executing tool %s: %v", toolCall.Function.Name, err)
				}
				fmt.Printf("      Result: %s\n", result)

				// 將工具執行結果加入歷史紀錄
				topic.History = append(topic.History, api.Message{
					Role:    "tool",
					Content: result,
				})
			}
			tm.saveTopic(topic)
			// 執行完工具後，迴圈繼續
			continue
		}

		// AI 回答完畢
		return resp.Message.Content, nil
	}
}

// pruneHistory 修剪歷史紀錄 (保留 System Prompt + 最近 N 則)
func (tm *TopicManager) pruneHistory(t *Topic) {
	const MaxWindowSize = 20 // 調整視窗大小

	if len(t.History) <= MaxWindowSize+1 {
		return
	}

	systemPrompt := t.History[0] // 假設第一則是 System
	// 如果第一則不是 System，可能要另外處理，這裡假設初始化都有 System

	startIndex := len(t.History) - MaxWindowSize
	// 確保 startIndex 不會切到 System Prompt 之後
	if startIndex < 1 {
		startIndex = 1
	}

	recentMessages := t.History[startIndex:]

	newHistory := make([]api.Message, 0, len(recentMessages)+1)
	if systemPrompt.Role == "system" {
		newHistory = append(newHistory, systemPrompt)
	}
	newHistory = append(newHistory, recentMessages...)

	t.History = newHistory
	// fmt.Printf("🧹 History pruned. Current size: %d\n", len(t.History))
}

// =======================
// 4. AI 互動層 (Ollama API)
// =======================

// findMatchingTopic 請求 AI 判斷歸類
func (tm *TopicManager) findMatchingTopic(input string) string {
	if len(tm.Topics) == 0 {
		return "NEW"
	}

	var activeTopics []*Topic
	for _, t := range tm.Topics {
		activeTopics = append(activeTopics, t)
	}

	sort.Slice(activeTopics, func(i, j int) bool {
		return activeTopics[i].LastActive.After(activeTopics[j].LastActive)
	})

	var sb strings.Builder
	for _, t := range activeTopics {
		sb.WriteString(fmt.Sprintf(`{"id": "%s", "summary": "%s", "last_active": "%s"}`+"\n",
			t.ID, t.Summary, t.LastActive.Format("15:04:05")))
	}

	systemPrompt := `You are a conversation router. Match the USER_INPUT to the most relevant Existing Topic.
The topics are listed from most recent to oldest.

Current Topics (JSON format):
` + sb.String() + `

Rules:
1. If the input is logically connected to a topic, return {"id": "UUID"}.
2. If it's a new topic, return {"id": "NEW"}.
3. Return JSON ONLY.`

	// 使用 Ollama JSON mode
	respJSON := tm.callOllamaSimple(systemPrompt, input, true)

	var result struct {
		ID string `json:"id"`
	}
	// 清理 Markdown
	cleanJSON := strings.Trim(respJSON, "```json\n ")
	cleanJSON = strings.Trim(cleanJSON, "`")
	if err := json.Unmarshal([]byte(cleanJSON), &result); err != nil {
		// fmt.Println(" [警告] 路由解析失敗，預設為 NEW:", err)
		return "NEW"
	}
	return result.ID
}

// generateSummary 生成摘要
func (tm *TopicManager) generateSummary(input string) string {
	sys := "Summarize the user input into a short topic title (max 10 words). Output text only."
	summary := tm.callOllamaSimple(sys, input, false)
	return strings.Trim(summary, `"`)
}

// callOllamaSimple 簡單的 Ollama 呼叫 (用於 Router 和 Summary，不涉及工具)
func (tm *TopicManager) callOllamaSimple(sys, user string, jsonMode bool) string {
	ctx := context.Background()
	stream := false
	req := &api.ChatRequest{
		Model: tm.Model,
		Messages: []api.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
		Stream: &stream,
	}
	if jsonMode {
		req.Format = json.RawMessage(`"json"`)
	}

	var content string
	tm.Client.Chat(ctx, req, func(r api.ChatResponse) error {
		content = r.Message.Content
		return nil
	})
	return content
}

// =======================
// 5. 輔助函數 (Utils)
// =======================

func (tm *TopicManager) isAmbiguousInput(input string) bool {
	if len([]rune(input)) < 2 {
		return true
	}
	keywords := []string{"好的", "謝謝", "了解", "繼續", "然後呢", "OK", "Thanks", "Yes", "No", "好"}
	for _, kw := range keywords {
		if strings.EqualFold(input, kw) {
			return true
		}
	}
	return false
}

func (tm *TopicManager) evictOldest() {
	var oldestID string
	var oldestTime time.Time
	first := true

	for id, t := range tm.Topics {
		if first || t.LastActive.Before(oldestTime) {
			oldestTime = t.LastActive
			oldestID = id
			first = false
		}
	}
	if oldestID != "" {
		fmt.Printf(" [清理] 移除最舊主題: %s\n", tm.Topics[oldestID].Summary)
		tm.deleteTopic(oldestID)
	}
}

func (tm *TopicManager) updateLastActiveTracker() {
	var newestTime time.Time
	for id, t := range tm.Topics {
		if t.LastActive.After(newestTime) {
			newestTime = t.LastActive
			tm.LastTopicID = id
		}
	}
}
