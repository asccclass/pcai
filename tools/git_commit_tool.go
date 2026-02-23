package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// GitAutoCommitTool 自動 git commit 工具
type GitAutoCommitTool struct{}

func (t *GitAutoCommitTool) Name() string {
	return "git_auto_commit"
}

func (t *GitAutoCommitTool) IsSkill() bool {
	return false
}

func (t *GitAutoCommitTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "git_auto_commit",
			Description: "Git 版本控制工具。支援三種操作：commit（分析變更並自動提交）、push（推送到遠端）、rollback（撤銷最後一次提交）。執行 commit 後，請務必詢問使用者是否要 push。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"action": {
						"type": "string",
						"description": "操作類型：commit (分析變更並提交), push (推送到遠端), rollback (撤銷最後一次 commit)",
						"enum": ["commit", "push", "rollback"]
					},
					"message": {
						"type": "string",
						"description": "自訂 commit 訊息。若留空，將自動根據 git diff 生成。"
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

func (t *GitAutoCommitTool) Run(argsJSON string) (string, error) {
	var args struct {
		Action  interface{} `json:"action"`
		Message interface{} `json:"message"`
	}
	cleanJSON := strings.Trim(argsJSON, "`json\n ")
	if err := json.Unmarshal([]byte(cleanJSON), &args); err != nil {
		return "", fmt.Errorf("參數錯誤: %w", err)
	}

	getString := func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		if m, ok := v.(map[string]interface{}); ok {
			if val, ok := m["value"].(string); ok {
				return val
			}
		}
		return ""
	}

	action := getString(args.Action)
	message := getString(args.Message)

	switch action {
	case "commit":
		return t.doCommit(message)
	case "push":
		return t.doPush()
	case "rollback":
		return t.doRollback()
	default:
		return fmt.Sprintf("不支援的操作: %s (支援: commit, push, rollback)", args.Action), nil
	}
}

// doCommit 分析變更、生成說明、自動 add + commit
func (t *GitAutoCommitTool) doCommit(customMessage string) (string, error) {
	// 1. 檢查是否在 git repo 中
	if _, err := runGit("rev-parse", "--git-dir"); err != nil {
		return "錯誤：當前目錄不是 Git 儲存庫。", nil
	}

	// 2. 取得變更狀態
	statusOutput, err := runGit("status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("git status 失敗: %w", err)
	}

	if strings.TrimSpace(statusOutput) == "" {
		return "目前沒有任何變更需要提交。", nil
	}

	// 3. 解析每個檔案的狀態
	fileDescriptions := parseGitStatus(statusOutput)

	// 4. 取得 diff 統計
	diffStat, _ := runGit("diff", "--stat", "HEAD")
	if diffStat == "" {
		// 可能有 untracked 檔案，先 add 再 diff
		diffStat, _ = runGit("diff", "--stat", "--cached")
	}

	// 5. 生成 commit message
	var commitMsg string
	if customMessage != "" {
		commitMsg = customMessage
	} else {
		commitMsg = generateCommitMessage(fileDescriptions)
	}

	// 6. git add -A
	if _, err := runGit("add", "-A"); err != nil {
		return "", fmt.Errorf("git add 失敗: %w", err)
	}

	// 7. git commit
	commitOutput, err := runGit("commit", "-m", commitMsg)
	if err != nil {
		return "", fmt.Errorf("git commit 失敗: %w\n%s", err, commitOutput)
	}

	// 8. 取得 commit hash
	hash, _ := runGit("rev-parse", "--short", "HEAD")
	hash = strings.TrimSpace(hash)

	// 9. 組裝回報
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ 已成功提交 (commit: %s)\n\n", hash))
	sb.WriteString("📋 變更檔案說明：\n")
	for _, fd := range fileDescriptions {
		sb.WriteString(fmt.Sprintf("  %s %s — %s\n", fd.StatusIcon, fd.FilePath, fd.Description))
	}
	sb.WriteString(fmt.Sprintf("\n📝 Commit Message:\n%s\n", commitMsg))
	sb.WriteString("\n⚠️ 請詢問使用者是否要 push 到遠端儲存庫。若使用者確認請呼叫 push，否則 commit 保留在本地。")

	return sb.String(), nil
}

// doPush 執行 git push
func (t *GitAutoCommitTool) doPush() (string, error) {
	output, err := runGit("push")
	if err != nil {
		return "", fmt.Errorf("git push 失敗: %w\n%s", err, output)
	}

	// 取得遠端資訊
	remote, _ := runGit("remote", "get-url", "origin")
	remote = strings.TrimSpace(remote)

	return fmt.Sprintf("✅ 已成功推送到遠端儲存庫。\n遠端: %s\n\n%s", remote, output), nil
}

// doRollback 撤銷最後一次 commit
func (t *GitAutoCommitTool) doRollback() (string, error) {
	// 取得將被撤銷的 commit 資訊
	logOutput, _ := runGit("log", "--oneline", "-1")
	logOutput = strings.TrimSpace(logOutput)

	output, err := runGit("reset", "HEAD~1")
	if err != nil {
		return "", fmt.Errorf("git reset 失敗: %w\n%s", err, output)
	}

	return fmt.Sprintf("↩️ 已撤銷最後一次提交: %s\n檔案仍保留在工作目錄中（未暫存狀態）。", logOutput), nil
}

// fileDescription 描述單個檔案的變更
type fileDescription struct {
	FilePath    string
	Status      string
	StatusIcon  string
	Description string
}

// parseGitStatus 解析 git status --porcelain 輸出
func parseGitStatus(output string) []fileDescription {
	var results []fileDescription
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		statusCode := strings.TrimSpace(line[:2])
		filePath := strings.TrimSpace(line[3:])

		// 處理重新命名 (R xxx -> yyy)
		if strings.Contains(filePath, " -> ") {
			parts := strings.Split(filePath, " -> ")
			filePath = parts[len(parts)-1]
		}

		fd := fileDescription{
			FilePath: filePath,
		}

		switch {
		case strings.Contains(statusCode, "A") || statusCode == "??":
			fd.Status = "新增"
			fd.StatusIcon = "🆕"
			fd.Description = "新增檔案"
		case strings.Contains(statusCode, "M"):
			fd.Status = "修改"
			fd.StatusIcon = "✏️"
			fd.Description = "修改內容"
		case strings.Contains(statusCode, "D"):
			fd.Status = "刪除"
			fd.StatusIcon = "🗑️"
			fd.Description = "刪除檔案"
		case strings.Contains(statusCode, "R"):
			fd.Status = "重新命名"
			fd.StatusIcon = "📝"
			fd.Description = "重新命名檔案"
		default:
			fd.Status = "變更"
			fd.StatusIcon = "📄"
			fd.Description = "檔案變更"
		}

		results = append(results, fd)
	}

	return results
}

// generateCommitMessage 根據檔案變更自動生成 commit message
func generateCommitMessage(files []fileDescription) string {
	now := time.Now().Format("2006-01-02 15:04")

	// 統計各類變更
	addCount, modCount, delCount := 0, 0, 0
	for _, f := range files {
		switch f.Status {
		case "新增":
			addCount++
		case "修改":
			modCount++
		case "刪除":
			delCount++
		}
	}

	// 生成標題
	var titleParts []string
	if addCount > 0 {
		titleParts = append(titleParts, fmt.Sprintf("新增 %d 檔", addCount))
	}
	if modCount > 0 {
		titleParts = append(titleParts, fmt.Sprintf("修改 %d 檔", modCount))
	}
	if delCount > 0 {
		titleParts = append(titleParts, fmt.Sprintf("刪除 %d 檔", delCount))
	}

	title := "chore: 自動提交"
	if len(titleParts) > 0 {
		title = fmt.Sprintf("chore: %s", strings.Join(titleParts, ", "))
	}

	// 組裝完整訊息
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s (%s)\n\n", title, now))
	sb.WriteString("變更檔案:\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- %s: [%s] %s\n", f.FilePath, f.Status, f.Description))
	}

	return sb.String()
}

// runGit 執行 git 指令並回傳輸出
func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
