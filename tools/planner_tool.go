package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/core"
	"github.com/ollama/ollama/api"
)

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

func (t *PlannerTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "task_planner",
			Description: "用於管理長期或複雜任務的規劃工具。可以建立計畫(create)、讀取當前計畫(get)、更新步驟狀態(update)。通常在面對需要多個步驟才能完成的複雜請求時使用。",
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
		Action string `json:"action"`
		Goal   string `json:"goal"`
		Steps  string `json:"steps"`
		StepID int    `json:"step_id"`
		Status string `json:"status"`
		Result string `json:"result"`
	}

	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析參數失敗: %v", err)
	}

	planFile := getPlanFilePath()

	switch args.Action {
	case "create":
		return t.createPlan(planFile, args.Goal, args.Steps)
	case "get":
		return t.getPlan(planFile)
	case "update":
		return t.updateStep(planFile, args.StepID, args.Status, args.Result)
	case "finish":
		return t.finishPlan(planFile)
	default:
		return "未知的動作: " + args.Action, nil
	}
}

func getPlanFilePath() string {
	home, _ := os.Getwd()
	// 確保目錄存在
	dir := filepath.Join(home, "botmemory")
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
		if s.Status == "completed" {
			icon = "✅"
		} else if s.Status == "in_progress" {
			icon = "🔄"
		} else if s.Status == "failed" {
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
	// 直接刪除檔案或標記為完成
	// 這裡選擇歸檔並刪除當前檔案
	plan, err := loadPlan(path)
	if err == nil {
		plan.Status = "completed"
		// TODO: 可以選擇歸檔到 history
	}
	if err := os.Remove(path); err != nil {
		return "移除計畫檔失敗: " + err.Error(), nil
	}
	return "目前計畫已結束並清除。", nil
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

// 確保 PlannerTool 實作 AgentTool 介面
var _ core.AgentTool = (*PlannerTool)(nil)
