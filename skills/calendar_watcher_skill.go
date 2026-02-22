package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/ollama/ollama/api"
)

// CalendarEvent 定義與 gogcli JSON 對應的結構
type CalendarEvent struct {
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       struct {
		DateTime string `json:"dateTime"` // ISO8601
		Date     string `json:"date"`     // YYYY-MM-DD (All day)
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		Date     string `json:"date"`
	} `json:"end"`
	Status string `json:"status"` // "confirmed", "tentative", "cancelled"
}

// CalendarWatcherSkill 監控行事曆變動
type CalendarWatcherSkill struct {
	StateFile      string
	KnowledgePath  string
	EventsPath     string
	TelegramToken  string
	TelegramChatID string
	GogPath        string
}

func NewCalendarWatcherSkill(tgToken, tgChatID string) *CalendarWatcherSkill {
	home, _ := os.Getwd()
	stateFile := filepath.Join(home, "botmemory", "calendar_state.json")
	knowledgePath := filepath.Join(home, "botmemory", "knowledge", "MEMORY.md")
	eventsPath := filepath.Join(home, "botmemory", "knowledge", "events.md")

	// 尋找 gog 執行檔
	gogPath := filepath.Join(home, "bin", "gog.exe")
	if _, err := os.Stat(gogPath); os.IsNotExist(err) {
		gogPath = "gog" // 嘗試從 PATH 找
	}

	return &CalendarWatcherSkill{
		StateFile:      stateFile,
		KnowledgePath:  knowledgePath,
		EventsPath:     eventsPath,
		TelegramToken:  tgToken,
		TelegramChatID: tgChatID,
		GogPath:        gogPath,
	}
}

// Execute 執行監控任務
// days: 監控未來幾天的事件 (例如 7 或 30)
func (s *CalendarWatcherSkill) Execute(days int) {
	log.Println("[CalendarWatcher] 開始檢查行事曆變動...")

	// 1. 取得當前所有行事曆事件
	now := time.Now()
	endDate := now.AddDate(0, 0, days)
	currentEvents, err := s.GetEvents(now, endDate)

	// 轉換成 map 以符合原有邏輯
	eventsMap := make(map[string]CalendarEvent)
	if err == nil {
		for _, e := range currentEvents {
			eventsMap[e.ID] = e
		}
	} else {
		log.Printf("[CalendarWatcher Error] 無法取得行事曆: %v", err)
		return
	}

	// 2. 讀取上次狀態
	lastEvents, err := s.loadState()
	if err != nil {
		log.Printf("[CalendarWatcher] 無法讀取上次狀態 (可能是初次執行): %v", err)
		// 初次執行，直接儲存狀態，不通知
		s.saveState(eventsMap)
		return
	}

	// 3. 比對差異
	added, removed, modified := s.diffEvents(lastEvents, eventsMap)

	if len(added) == 0 && len(removed) == 0 && len(modified) == 0 {
		log.Println("[CalendarWatcher] 行事曆無變動。")
		return
	}

	// 4. 通知與更新
	s.notifyChanges(added, removed, modified)
	s.updateKnowledge(added, removed, modified)
	s.saveState(eventsMap)
}

// GetEvents 取得指定時間範圍內的事件
func (s *CalendarWatcherSkill) GetEvents(from, to time.Time) ([]CalendarEvent, error) {
	// gog calendar events --all --from YYYY-MM-DD --to YYYY-MM-DD --json
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	cmd := exec.Command(s.GogPath, "calendar", "events", "--all", "--from", fromStr, "--to", toStr, "--json")
	cmd.Env = os.Environ()
	// [FIX] 注入 ZONEINFO 以修復 Windows 上的時區解析問題
	goroot := os.Getenv("GOROOT")
	if goroot != "" {
		zoneinfo := filepath.Join(goroot, "lib", "time", "zoneinfo.zip")
		cmd.Env = append(cmd.Env, "ZONEINFO="+zoneinfo)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gogcli error: %v, output: %s", err, string(output))
	}

	// 解析 JSON
	var container struct {
		Events []CalendarEvent `json:"events"`
	}
	if err := json.Unmarshal(output, &container); err != nil {
		// 嘗試直接 array? (為了容錯)
		var list []CalendarEvent
		if err2 := json.Unmarshal(output, &list); err2 == nil {
			container.Events = list
		} else {
			return nil, fmt.Errorf("json parse error: %v", err)
		}
	}

	// 過濾 cancelled
	var validEvents []CalendarEvent
	for _, e := range container.Events {
		if e.Status != "cancelled" {
			validEvents = append(validEvents, e)
		}
	}
	return validEvents, nil
}

// GenerateDailyBriefing 生成每日簡報並儲存
func (s *CalendarWatcherSkill) GenerateDailyBriefing(client *api.Client, model string) (string, error) {
	now := time.Now()
	// 取得今天 (00:00) 到明天 (00:00) 的事件
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 0, 1)

	events, err := s.GetEvents(from, to)
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "今日無行程，祝您有美好的一天！", nil
	}

	// 構建 Prompt
	var scheduleBuilder strings.Builder
	for _, e := range events {
		timeStr := s.formatTime(e)
		scheduleBuilder.WriteString(fmt.Sprintf("- %s: %s (地點: %s)\n  描述: %s\n", timeStr, e.Summary, e.Location, e.Description))
	}

	prompt := fmt.Sprintf(`
你是一個高效的貼身秘書。這是今天的行程表：
%s

請為我生成一份「每日簡報」，包含：
1. ☀️ 早安問候。
2. 📝 今日行程總覽 (條列式)。
3. ✅ 根據行程建議的待辦事項 (例如：如果有會議，提醒準備資料)。
4. 💡 溫馨提醒 (天氣或注意事項)。

請使用 Markdown 格式，與溫暖專業的語氣。
`, scheduleBuilder.String())

	// Call Ollama
	req := &api.GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: new(bool),
	}

	var briefing string
	ctx := context.Background()
	err = client.Generate(ctx, req, func(resp api.GenerateResponse) error {
		briefing = resp.Response
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("Ollama generation failed: %v", err)
	}

	// Save to events.md
	if err := s.appendToEvents("📅 每日簡報", briefing); err != nil {
		log.Printf("[CalendarBriefing] 無法寫入 events.md: %v", err)
	}

	return briefing, nil
}

// CheckUpcoming 檢查即將發生的事件 (例如 30 分鐘內)
func (s *CalendarWatcherSkill) CheckUpcoming(lookahead time.Duration) error {
	now := time.Now()
	// 為了避免漏掉，我們檢查 [now, now + lookahead]
	// 為了避免重複通知，我們需要一個簡單的機制 (例如檢查 events.md 是否最近寫過? 還是使用 StateFile?)
	// 簡單起見，我們只在事件開始前的 (lookahead - 5m) 到 (lookahead) 之間通知。
	// 假設 Cron 是每 5 分鐘跑一次。

	// 取得今天的事件
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := from.AddDate(0, 0, 1)

	events, err := s.GetEvents(from, to)
	if err != nil {
		return err
	}

	for _, e := range events {
		// 解析開始時間
		startTime, err := time.Parse(time.RFC3339, e.Start.DateTime)
		if err != nil {
			continue // 忽略全天事件或格式錯誤
		}

		timeUntil := startTime.Sub(now)
		// 如果在目標時間範圍內 (例如 25~30 分鐘後開始)
		// 假設 Cron 5 分鐘跑一次，我們檢查 30m >= timeUntil > 25m
		// 這樣只會觸發一次
		upperBound := lookahead
		lowerBound := lookahead - 10*time.Minute // 寬鬆一點，10分鐘區間

		if timeUntil <= upperBound && timeUntil > lowerBound {
			// 發送通知
			msg := fmt.Sprintf("🔔 **行程提醒**\n\n**%s** 即將在 %d 分鐘後開始 (%s)！\n地點: %s",
				e.Summary, int(timeUntil.Minutes()), startTime.Format("15:04"), e.Location)

			s.sendTelegram(msg)

			// 記錄到 events.md
			s.appendToEvents("🔔 行程提醒", fmt.Sprintf("已通知使用者: %s 即將開始", e.Summary))
		}
	}
	return nil
}

func (s *CalendarWatcherSkill) appendToEvents(title, content string) error {
	f, err := os.OpenFile(s.EventsPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04")
	entry := fmt.Sprintf("\n\n## %s: %s\n%s\n", title, timestamp, content)
	_, err = f.WriteString(entry)
	return err
}

func (s *CalendarWatcherSkill) sendTelegram(text string) {
	if s.TelegramToken == "" || s.TelegramChatID == "" {
		return
	}
	client := resty.New()
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.TelegramToken)
	client.R().
		SetBody(map[string]string{
			"chat_id":    s.TelegramChatID,
			"text":       text,
			"parse_mode": "Markdown",
		}).
		Post(url)
}

func (s *CalendarWatcherSkill) fetchEvents(days int) (map[string]CalendarEvent, error) {
	return nil, fmt.Errorf("deprecated, use GetEvents")
}

func (s *CalendarWatcherSkill) loadState() (map[string]CalendarEvent, error) {
	if _, err := os.Stat(s.StateFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("state file not found")
	}
	data, err := os.ReadFile(s.StateFile)
	if err != nil {
		return nil, err
	}
	var state map[string]CalendarEvent
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *CalendarWatcherSkill) saveState(state map[string]CalendarEvent) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.StateFile, data, 0644)
}

func (s *CalendarWatcherSkill) diffEvents(oldEvents, newEvents map[string]CalendarEvent) (added, removed, modified []CalendarEvent) {
	for id, newEv := range newEvents {
		if oldEv, exists := oldEvents[id]; !exists {
			added = append(added, newEv)
		} else {
			// 檢查是否修改 (比較特定欄位: Summary, Start, End, Location, Description)
			if !s.isEqual(oldEv, newEv) {
				modified = append(modified, newEv)
			}
		}
	}

	for id, oldEv := range oldEvents {
		if _, exists := newEvents[id]; !exists {
			// 如果在新列表不存在，可能是刪除，也可能是移出了時間範圍
			// 這裡簡單判定為刪除/移出
			removed = append(removed, oldEv)
		}
	}
	return
}

func (s *CalendarWatcherSkill) isEqual(a, b CalendarEvent) bool {
	// 忽略 ID, Status 等變動不大的欄位
	// 只關心核心內容
	if a.Summary != b.Summary {
		return false
	}
	if a.Description != b.Description {
		return false
	}
	if a.Location != b.Location {
		return false
	}

	// 時間比較
	if a.Start.DateTime != b.Start.DateTime {
		return false
	}
	if a.Start.Date != b.Start.Date {
		return false
	}
	if a.End.DateTime != b.End.DateTime {
		return false
	}
	if a.End.Date != b.End.Date {
		return false
	}

	return true
}

func (s *CalendarWatcherSkill) notifyChanges(added, removed, modified []CalendarEvent) {
	if s.TelegramToken == "" || s.TelegramChatID == "" {
		log.Println("⚠️ [CalendarWatcher] 未設定 Telegram Token/ChatID，略過通。")
		return
	}

	var sb strings.Builder
	sb.WriteString("📅 **行事曆變動通知**\n\n")

	if len(added) > 0 {
		sb.WriteString("🆕 **新增事件**:\n")
		for _, e := range added {
			timeStr := s.formatTime(e)
			sb.WriteString(fmt.Sprintf("- %s | %s\n", timeStr, e.Summary))
		}
		sb.WriteString("\n")
	}

	if len(modified) > 0 {
		sb.WriteString("✏️ **修改事件**:\n")
		for _, e := range modified {
			timeStr := s.formatTime(e)
			sb.WriteString(fmt.Sprintf("- %s | %s\n", timeStr, e.Summary))
		}
		sb.WriteString("\n")
	}

	if len(removed) > 0 {
		sb.WriteString("🗑️ **移除事件**:\n")
		for _, e := range removed {
			timeStr := s.formatTime(e)
			sb.WriteString(fmt.Sprintf("- %s | %s\n", timeStr, e.Summary))
		}
	}

	// 發送 Telegram
	client := resty.New()
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.TelegramToken)
	_, err := client.R().
		SetBody(map[string]string{
			"chat_id":    s.TelegramChatID,
			"text":       sb.String(),
			"parse_mode": "Markdown",
		}).
		Post(url)

	if err != nil {
		log.Printf("[CalendarWatcher Error] Telegram 發送失敗: %v", err)
	}
}

func (s *CalendarWatcherSkill) formatTime(e CalendarEvent) string {
	if e.Start.DateTime != "" {
		// 解析時間 (ISO8601)
		t, err := time.Parse(time.RFC3339, e.Start.DateTime)
		if err == nil {
			return t.Format("01/02 15:04")
		}
		return e.Start.DateTime
	}
	return e.Start.Date // 全天事件
}

func (s *CalendarWatcherSkill) updateKnowledge(added, removed, modified []CalendarEvent) {
	// 簡單將變動記錄到 Knowledge，讓 Agent 知道最近發生了什麼事
	f, err := os.OpenFile(s.KnowledgePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[CalendarWatcher Error] 無法寫入 Knowledge: %v", err)
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n\n## 📅 行事曆變動紀錄: %s\n", timestamp))

	// 簡化紀錄，不多佔用 Token
	count := len(added) + len(modified)
	if count > 0 {
		sb.WriteString(fmt.Sprintf("偵測到 %d 筆行程新增或修改。詳細內容已通知使用者。\n", count))
		for _, e := range added {
			sb.WriteString(fmt.Sprintf("- [NEW] %s %s\n", s.formatTime(e), e.Summary))
		}
		for _, e := range modified {
			sb.WriteString(fmt.Sprintf("- [MOD] %s %s\n", s.formatTime(e), e.Summary))
		}
	}
	// 移除的事件也稍微提一下
	if len(removed) > 0 {
		sb.WriteString(fmt.Sprintf("偵測到 %d 筆行程移除。\n", len(removed)))
	}

	if _, err := f.WriteString(sb.String()); err != nil {
		log.Printf("[CalendarWatcher Error] 寫入 Knowledge 失敗: %v", err)
	} else {
		log.Println("✅ [CalendarWatcher] 變動已記錄至 Knowledge")
	}
}
