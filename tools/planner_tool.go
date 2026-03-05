package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/core"
	"github.com/ollama/ollama/api"
)

// ─────────────────────────────────────────────────────────────
// 任務鎖定 (File-based Task Lock) — 確保同一時間只有一個任務在執行
// ─────────────────────────────────────────────────────────────

func getTaskLockPath() string {
	workspace := os.Getenv("WORKSPACE_PATH")
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return filepath.Join(workspace, "tasks", "task.lock")
}

// IsTaskLocked 檢查是否有任務正在執行
func IsTaskLocked() bool {
	_, err := os.Stat(getTaskLockPath())
	return err == nil
}

// AcquireTaskLock 嘗試獲取任務鎖
func AcquireTaskLock() bool {
	if IsTaskLocked() {
		return false // 已有任務在執行
	}
	lockPath := getTaskLockPath()
	_ = os.MkdirAll(filepath.Dir(lockPath), 0755)
	content := fmt.Sprintf(`{"locked_at": "%s", "pid": %d}`, time.Now().Format(time.RFC3339), os.Getpid())
	return os.WriteFile(lockPath, []byte(content), 0644) == nil
}

// ReleaseTaskLock 釋放任務鎖
func ReleaseTaskLock() {
	_ = os.Remove(getTaskLockPath())
}

// CheckPendingPlan 檢查是否有未完成的計畫，若有則回傳恢復提示
// 可由 agent.Chat 或 heartbeat.RunPatrol 呼叫
func CheckPendingPlan() string {
	planFile := getPlanFilePath()
	plan, err := loadPlan(planFile)
	if err != nil {
		return "" // 無計畫檔或無法讀取
	}

	// 檢查是否有未完成的步驟
	if plan.Status == "completed" {
		return ""
	}

	// 計算進度
	total := len(plan.Steps)
	completed := 0
	nextPending := ""
	for _, s := range plan.Steps {
		if s.Status == "completed" {
			completed++
		} else if nextPending == "" && (s.Status == "pending" || s.Status == "in_progress") {
			nextPending = fmt.Sprintf("步驟 %d: %s", s.ID, s.Description)
		}
	}

	if completed >= total {
		return "" // 全部完成，不需恢復
	}

	// 生成計畫狀態摘要
	var sb strings.Builder
	sb.WriteString("[SYSTEM INSTRUCTION — 任務恢復模式]\n")
	sb.WriteString("⚠️ 系統偵測到一個未完成的任務計畫需要恢復執行。\n\n")
	sb.WriteString(fmt.Sprintf("📋 任務目標: %s\n", plan.Goal))
	sb.WriteString(fmt.Sprintf("📊 進度: %d/%d 步驟已完成\n\n", completed, total))

	// 已完成步驟的結果（供 LLM 直接引用，不需要重新執行）
	hasCompletedResults := false
	for _, s := range plan.Steps {
		if s.Status == "completed" && s.Result != "" {
			if !hasCompletedResults {
				sb.WriteString("📦 已完成步驟的快取結果（請直接引用，不要重新執行）:\n")
				hasCompletedResults = true
			}
			sb.WriteString(fmt.Sprintf("  ✅ 步驟 %d [%s]: %s\n", s.ID, s.Description, s.Result))
		}
	}
	if hasCompletedResults {
		sb.WriteString("\n")
	}

	// 待執行步驟
	sb.WriteString("📝 待執行步驟:\n")
	for _, s := range plan.Steps {
		if s.Status == "completed" || s.Status == "skipped" {
			continue
		}
		icon := "⬜"
		switch s.Status {
		case "in_progress":
			icon = "🔄"
		case "failed":
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("  %s 步驟 %d: %s [%s]\n", icon, s.ID, s.Description, s.Status))
	}

	sb.WriteString(fmt.Sprintf("\n🔜 下一步: %s\n\n", nextPending))
	sb.WriteString("執行規範:\n")
	sb.WriteString("1. ❌ 禁止重新執行已完成的步驟，直接引用上方快取結果\n")
	sb.WriteString("2. ✅ 從第一個未完成的步驟開始依序執行\n")
	sb.WriteString("3. 📝 每完成一步，必須呼叫 task_planner(action=\"update\", step_id=N, status=\"completed\", result=\"執行結果摘要\") 記錄結果\n")
	sb.WriteString("4. 🏁 所有步驟完成後，呼叫 task_planner(action=\"finish\") 結束計畫\n")
	sb.WriteString("5. ⚠️ 若步驟失敗，使用 status=\"failed\" 並記錄失敗原因到 result，然後繼續下一步")

	return sb.String()
}

// TaskPlan 代表一個完整的任務計畫
type TaskPlan struct {
	ID        string     `json:"id"`
	Goal      string     `json:"goal"`
	Steps     []TaskStep `json:"steps"`
	CreatedAt time.Time  `json:"created_at"`
	Status    string     `json:"status"` // "planning", "in_progress", "completed", "failed"
}

// TaskStep 代表計畫中的單一步驟
type TaskStep struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"` // "pending", "in_progress", "completed", "skipped", "failed"
	Result      string `json:"result,omitempty"`
}

type PlannerTool struct{}

func NewPlannerTool() *PlannerTool {
	return &PlannerTool{}
}

func (t *PlannerTool) Name() string {
	return "task_planner"
}

func (t *PlannerTool) IsSkill() bool {
	return false
}

func (t *PlannerTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "task_planner",
			Description: "用於管理長期或複雜任務的規劃工具。可以建立計畫(create)、讀取當前計畫(get)、更新步驟狀態(update)、追加步驟(append)、結束計畫(finish)。注意：update 時 result 為必填，用來記錄該步驟的執行結果以供恢復時使用。finish 只有在所有步驟都完成時才能呼叫。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"action": {
						"type": "string",
						"description": "執行動作：'create' (建立新計畫), 'get' (讀取當前計畫), 'update' (更新步驟狀態), 'append' (新增步驟), 'finish' (結束計畫)",
						"enum": ["create", "get", "update", "append", "finish"]
					},
					"goal": {
						"type": "string",
						"description": "任務總目標 (僅用於 create 動作)"
					},
					"steps": {
						"type": "string",
						"description": "分號分隔的步驟列表 (僅用於 create 動作)，例如 '步驟1;步驟2;步驟3'"
					},
					"step_id": {
						"type": "integer",
						"description": "要更新的步驟 ID (僅用於 update 動作)"
					},
					"status": {
						"type": "string",
						"description": "新的狀態 (僅用於 update 動作): 'in_progress', 'completed', 'failed', 'skipped'",
						"enum": ["in_progress", "completed", "failed", "skipped"]
					},
					"result": {
						"type": "string",
						"description": "執行的結果或註解 (僅用於 update 動作)"
					}
				}`
				_ = json.Unmarshal([]byte(js), &props)
				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"action"},
				}
			}(),
		},
	}
}

func (t *PlannerTool) Run(argsJSON string) (string, error) {
	var args struct {
		Action string      `json:"action"`
		Goal   string      `json:"goal"`
		Steps  string      `json:"steps"`
		StepID interface{} `json:"step_id"` // 容錯：接受 int 或 string
		Status string      `json:"status"`
		Result string      `json:"result"`
	}

	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	// 解析 step_id（容錯處理 LLM 可能傳入 string 或帶逗號的數字）
	stepID := 0
	if args.StepID != nil {
		switch v := args.StepID.(type) {
		case float64:
			stepID = int(v)
		case string:
			// 清理非數字字元（如 "1," → "1"）
			cleaned := strings.TrimRight(strings.TrimSpace(v), ",;. ")
			if n, err := strconv.Atoi(cleaned); err == nil {
				stepID = n
			}
		}
	}

	planFile := getPlanFilePath()

	switch args.Action {
	case "create":
		return t.createPlan(planFile, args.Goal, args.Steps)
	case "get":
		return t.getPlan(planFile)
	case "update":
		return t.updateStep(planFile, stepID, args.Status, args.Result)
	case "append":
		return t.appendStep(planFile, args.Steps)
	case "finish":
		return t.finishPlan(planFile)
	default:
		return "未知的動作: " + args.Action, nil
	}
}

func getPlanFilePath() string {
	workspace := os.Getenv("WORKSPACE_PATH")
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	// 確保目錄存在
	dir := filepath.Join(workspace, "tasks")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "current_plan.json")
}

func (t *PlannerTool) createPlan(path, goal, stepsStr string) (string, error) {
	if goal == "" || stepsStr == "" {
		return "建立計畫失敗: 必須提供 goal 和 steps", nil
	}

	rawSteps := strings.Split(stepsStr, ";")
	var steps []TaskStep
	for i, s := range rawSteps {
		desc := strings.TrimSpace(s)
		if desc != "" {
			steps = append(steps, TaskStep{
				ID:          i + 1,
				Description: desc,
				Status:      "pending",
			})
		}
	}

	plan := TaskPlan{
		ID:        fmt.Sprintf("%d", time.Now().Unix()),
		Goal:      goal,
		Steps:     steps,
		CreatedAt: time.Now(),
		Status:    "in_progress",
	}

	return savePlan(path, &plan, "計畫已建立。請遵循此計畫執行。")
}

func (t *PlannerTool) getPlan(path string) (string, error) {
	plan, err := loadPlan(path)
	if err != nil {
		return "目前沒有執行中的計畫。", nil
	}
	// 格式化輸出
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 當前任務: %s (Status: %s)\n", plan.Goal, plan.Status))
	for _, s := range plan.Steps {
		icon := "⬜"
		switch s.Status {
		case "completed":
			icon = "✅"
		case "in_progress":
			icon = "🔄"
		case "failed":
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %d. %s [%s]\n", icon, s.ID, s.Description, s.Status))
		if s.Result != "" {
			sb.WriteString(fmt.Sprintf("   └ 結果: %s\n", s.Result))
		}
	}
	return sb.String(), nil
}

func (t *PlannerTool) updateStep(path string, stepID int, status, result string) (string, error) {
	plan, err := loadPlan(path)
	if err != nil {
		return "無法更新: 沒有執行中的計畫", nil
	}

	found := false
	for i, s := range plan.Steps {
		if s.ID == stepID {
			if status != "" {
				plan.Steps[i].Status = status
			}
			if result != "" {
				// Append result if exists
				if plan.Steps[i].Result != "" {
					plan.Steps[i].Result += "; " + result
				} else {
					plan.Steps[i].Result = result
				}
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Sprintf("找不到步驟 ID: %d", stepID), nil
	}

	return savePlan(path, plan, fmt.Sprintf("步驟 %d 已更新為 %s", stepID, status))
}

func (t *PlannerTool) finishPlan(path string) (string, error) {
	plan, err := loadPlan(path)
	if err != nil {
		return "無法讀取計畫檔", nil
	}

	// 護欄：檢查是否所有步驟都已完成或跳過
	pendingSteps := []string{}
	for _, s := range plan.Steps {
		if s.Status != "completed" && s.Status != "skipped" && s.Status != "failed" {
			pendingSteps = append(pendingSteps, fmt.Sprintf("步驟 %d: %s [%s]", s.ID, s.Description, s.Status))
		}
	}

	if len(pendingSteps) > 0 {
		return fmt.Sprintf("⚠️ 無法結束計畫：仍有 %d 個步驟未完成。請先完成以下步驟：\n%s",
			len(pendingSteps), strings.Join(pendingSteps, "\n")), nil
	}

	// 所有步驟已完成，標記為 completed 並刪除檔案
	plan.Status = "completed"

	// 歸檔到 tasks/archive/ 目錄
	archiveDir := filepath.Join(filepath.Dir(path), "archive")
	_ = os.MkdirAll(archiveDir, 0755)
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("plan_%s_%s.json", plan.ID, time.Now().Format("20060102_150405")))
	if data, err := json.MarshalIndent(plan, "", "  "); err == nil {
		_ = os.WriteFile(archivePath, data, 0644)
	}

	// 刪除當前計畫檔
	if err := os.Remove(path); err != nil {
		ReleaseTaskLock() // 釋放任務鎖
		return "計畫已完成但清除失敗: " + err.Error(), nil
	}
	ReleaseTaskLock() // 釋放任務鎖
	return "✅ 計畫已全部完成！已歸檔至 " + archivePath, nil
}

func loadPlan(path string) (*TaskPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plan TaskPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func savePlan(path string, plan *TaskPlan, successMsg string) (string, error) {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return successMsg, nil
}

// appendStep 動態追加新步驟到現有計畫
func (t *PlannerTool) appendStep(path, stepsStr string) (string, error) {
	if stepsStr == "" {
		return "追加步驟失敗: 必須提供 steps", nil
	}

	plan, err := loadPlan(path)
	if err != nil {
		return "無法追加: 沒有執行中的計畫", nil
	}

	nextID := len(plan.Steps) + 1
	rawSteps := strings.Split(stepsStr, ";")
	for _, s := range rawSteps {
		desc := strings.TrimSpace(s)
		if desc != "" {
			plan.Steps = append(plan.Steps, TaskStep{
				ID:          nextID,
				Description: desc,
				Status:      "pending",
			})
			nextID++
		}
	}

	return savePlan(path, plan, fmt.Sprintf("已追加 %d 個步驟到計畫中。", len(rawSteps)))
}

// 確保 PlannerTool 實作 AgentTool 介面
var _ core.AgentTool = (*PlannerTool)(nil)
