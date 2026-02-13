package calendar

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Event 簡化後的行事曆事件結構
type Event struct {
	ID          string
	Summary     string
	Description string
	Start       string
	End         string
	Location    string
	Status      string
	HtmlLink    string
}

// CalendarItem 簡化後的行事曆列表項目
type CalendarItem struct {
	ID         string
	Summary    string
	Primary    bool
	AccessRole string // e.g. "owner", "reader", "freeBusyReader"
}

// gogCalendarListEntry 對應 gog calendar calendars --json 的單項結構
type gogCalendarListEntry struct {
	ID         string `json:"id"`
	Summary    string `json:"summary"`
	Primary    bool   `json:"primary"`
	AccessRole string `json:"accessRole"`
	Selected   bool   `json:"selected"`
}

// gogCalendarListResponse 對應 gog calendar calendars --json 的回應
type gogCalendarListResponse struct {
	Calendars []gogCalendarListEntry `json:"calendars"`
}

// gogEventListResponse 對應 gog calendar events --json 的回應
type gogEventListResponse struct {
	Events []gogEvent `json:"events"`
}

type gogEvent struct {
	ID          string       `json:"id"`
	Summary     string       `json:"summary"`
	Status      string       `json:"status"`
	HtmlLink    string       `json:"htmlLink"`
	Description string       `json:"description"`
	Location    string       `json:"location"`
	Start       gogEventTime `json:"start"`
	End         gogEventTime `json:"end"`
}

type gogEventTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

// ListCalendars 列出所有可用的行事曆 (使用 gog CLI)
func ListCalendars() ([]CalendarItem, error) {
	// exec gog calendar calendars --json
	gogPath := os.Getenv("GOG_PATH")
	if gogPath == "" {
		gogPath = "gog"
	}
	cmd := exec.Command(gogPath, "calendar", "calendars", "--json")
	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("執行 gog calendar calendars 失敗: %v, Stderr: %s", err, string(exitError.Stderr))
		}
		return nil, fmt.Errorf("執行 gog calendar calendars 失敗: %v", err)
	}

	var resp gogCalendarListResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("解析 gog 輸出失敗 (JSON格式不符?): %v, Output: %s", err, string(output))
	}

	var results []CalendarItem
	for _, item := range resp.Calendars {
		results = append(results, CalendarItem{
			ID:         item.ID,
			Summary:    item.Summary,
			Primary:    item.Primary,
			AccessRole: item.AccessRole,
		})
	}
	return results, nil
}

// FetchUpcomingEvents 抓取指定時間點後的行程 (使用 gog CLI)
// 根據使用者需求: 預設查詢未來 7 天
func FetchUpcomingEvents(calendarID string, timeMin string, maxResults int64) ([]Event, error) {
	if calendarID == "" {
		calendarID = "primary"
	}

	// 若未指定 timeMin，則預設為 Now (RFC3339)
	if timeMin == "" {
		timeMin = time.Now().Format(time.RFC3339)
	}

	// 根據使用者需求: 設定查詢範圍為 7 天
	// 注意: 這裡我們將 timeMin 作為起始點
	tMin, err := time.Parse(time.RFC3339, timeMin)
	if err != nil {
		// 若解析失敗，退回 Now
		tMin = time.Now()
		timeMin = tMin.Format(time.RFC3339)
	}
	tMax := tMin.AddDate(0, 0, 7)
	timeMax := tMax.Format(time.RFC3339)

	fmt.Printf("🔍 [DEBUG] 正在呼叫 gog 抓取行事曆資料...\n")
	fmt.Printf("🔍 [DEBUG] 查詢範圍: %s 到 %s\n", timeMin, timeMax)

	// gog calendar events [<calendarID>] --from ... --to ... --json [--all]
	// If calendarID is "all", use --all flag and omit calendarID arg.
	args := []string{"calendar", "events"}

	if calendarID == "all" {
		args = append(args, "--all")
	} else {
		args = append(args, calendarID)
	}

	args = append(args,
		"--from", timeMin,
		"--to", timeMax,
		"--json")

	if maxResults > 0 {
		args = append(args, "--max", fmt.Sprintf("%d", maxResults))
	}

	gogPath := os.Getenv("GOG_PATH")
	if gogPath == "" {
		gogPath = "gog"
	}
	cmd := exec.Command(gogPath, args...)
	fmt.Printf("🔍 [DEBUG] Executing: %s\n", cmd.String())

	output, err := cmd.Output()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gog 執行錯誤: %v, Stderr: %s", err, string(exitError.Stderr))
		}
		return nil, fmt.Errorf("gog 執行錯誤: %v", err)
	}

	var resp gogEventListResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("解析 gog 事件輸出失敗: %v", err)
	}

	var results []Event
	for _, item := range resp.Events {
		start := item.Start.DateTime
		if start == "" {
			start = item.Start.Date
		}
		end := item.End.DateTime
		if end == "" {
			end = item.End.Date
		}
		results = append(results, Event{
			ID:          item.ID,
			Summary:     item.Summary,
			Description: item.Description,
			Start:       start,
			End:         end,
			Location:    item.Location,
			Status:      item.Status,
			HtmlLink:    item.HtmlLink,
		})
	}

	fmt.Printf("\n=== 成功抓取到 %d 個行程 ===\n", len(results))
	return results, nil
}
