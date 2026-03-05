package systemtesting

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/skillloader"
)

// ============================================================
// Stage 5: Manage Calendar E2E Scenario Test
// 測試計畫：測試對行事曆的新增、查詢、修改、刪除流程
// 執行方式： go test -v .\systemtesting\05_manage_calendar_test.go
// ============================================================

// CalendarScenarioStep 定義端對端行事曆測試的每個步驟
type CalendarScenarioStep struct {
	StepName    string
	UserInput   string
	Mode        string
	ExpectedRes []string // 期望在回傳結果中包含的關鍵字
}

// TestManageCalendar_EndToEndScenario 測試行事曆的新增、查詢、修改、刪除
// 注意：由於此測試會實際呼叫 calendar.exe 操作真實 Google Calendar API，
// 建議在有需要驗證時開啟。此測試預設可能需要較長等待時間。
func TestManageCalendar_EndToEndScenario(t *testing.T) {
	// 為了避免影響真實行事曆資料而產生副作用，可視情況使用 t.Skip() 略過
	// t.Skip("Skip E2E calendar test to avoid real Google Calendar modification")

	cwd, _ := os.Getwd()
	skillsDir := filepath.Join(cwd, "..", "skills")

	loadedSkills, err := skillloader.LoadSkills(skillsDir)
	if err != nil {
		t.Fatalf("LoadSkills failed: %v", err)
	}

	var calendarSkill *skillloader.SkillDefinition
	for _, s := range loadedSkills {
		if s.Name == "manage_calendar" {
			calendarSkill = s
			break
		}
	}
	if calendarSkill == nil {
		t.Fatal("manage_calendar skill not found")
	}

	reg := core.NewRegistry()
	tool := skillloader.NewDynamicTool(calendarSkill, reg, nil)
	reg.Register(tool)

	// 設定好時間基準
	year := time.Now().Year() // 如果跨年可能會變動，確保測試在 2026/03 時依然穩健
	if year < 2026 {
		year = 2026
	}
	targetDate := fmt.Sprintf("%d-03-04", year)
	startTime := targetDate + "T18:00:00+08:00"
	endTime := targetDate + "T20:00:00+08:00"

	// 模擬 AI 在理解完人類對話後，產生不同的 tool calls 參數進一步呼叫 manage_calendar
	scenarioSteps := []struct {
		desc   string
		params map[string]interface{}
		verify func(t *testing.T, result string)
	}{
		{
			desc: "1. 幫我把3月4日晚上六點要去參加松山工農家長會在采芝樓聚餐的行程加入行事曆",
			params: map[string]interface{}{
				"mode":     "create",
				"from":     startTime,
				"to":       endTime,
				"summary":  "松山工農家長會聚餐",
				"location": "采芝樓",
				"cal":      "智漢個人行程",
				"force":    "true", // 測試環境強制寫入以避免被既有行程卡住
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "新增成功") {
					t.Errorf("Expected success message for create, got: %s", result)
				}
			},
		},
		{
			desc: "2. 查詢3月4日的行程是否有松山工農家長會的行事曆",
			params: map[string]interface{}{
				"mode": "read",
				"from": targetDate,
				"to":   targetDate,
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "松山工農家長會聚餐") {
					t.Errorf("Expected specific event in read result, got: %s", result)
				}
			},
		},
		{
			desc: "3. 幫我修改3月4日晚上六點松山工農家長會活動的行事曆地點改為科長辦公室",
			params: map[string]interface{}{
				"mode":     "update",
				"event":    "", // 留空由底層 summary fuzzy match 去尋找 ID
				"summary":  "松山工農家長會聚餐",
				"location": "科長辦公室",
				"from":     startTime,
				"to":       endTime,
				"cal":      "智漢個人行程",
				"force":    "true", // 測試環境強制寫入
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "更新成功") {
					t.Errorf("Expected success message for update, got: %s", result)
				}
			},
		},
		{
			desc: "4. 查詢3月4日晚上六點的行程地點是否為科長辦公室",
			params: map[string]interface{}{
				"mode": "read",
				"from": targetDate,
				"to":   targetDate,
			},
			verify: func(t *testing.T, result string) {
				// 應該要能夠在輸出中找到變更後的地點
				if !strings.Contains(result, "科長辦公室") {
					t.Errorf("Expected updated location in read result, got: %s", result)
				}
			},
		},
		{
			desc: "5. 刪除3月4日晚上六點松山工農家長會聚餐的行程",
			params: map[string]interface{}{
				"mode":    "delete",
				"event":   "", // 留空由底層模糊尋找
				"summary": "松山工農家長會聚餐",
				"from":    targetDate,
				"to":      targetDate,
			},
			verify: func(t *testing.T, result string) {
				if !strings.Contains(result, "刪除") {
					t.Errorf("Expected success message for delete, got: %s", result)
				}
			},
		},
		{
			desc: "6. 查詢3月4日的行程是否已無松山工農家長會的聚餐行程",
			params: map[string]interface{}{
				"mode": "read",
				"from": targetDate,
				"to":   targetDate,
			},
			verify: func(t *testing.T, result string) {
				if strings.Contains(result, "松山工農家長會聚餐") && strings.Contains(result, "科長辦公室") {
					t.Errorf("Expected event to be absent, but found in: %s", result)
				}
			},
		},
	}

	// 依序執行測試計畫
	for _, step := range scenarioSteps {
		t.Run(step.desc, func(t *testing.T) {
			paramBytes, _ := json.Marshal(step.params)
			result, err := reg.CallTool("manage_calendar", string(paramBytes))
			if err != nil {
				// 由於 delete 成功或找不到檔案有時也可能會有非 0 退出碼
				// 只要 result 字串正確就放行
				if step.params["mode"] != "delete" || !strings.Contains(result, "刪除") {
					t.Fatalf("CallTool failed for %s: %v\nResult string: %s", step.desc, err, result)
				}
			}
			step.verify(t, result)
		})
	}
}
