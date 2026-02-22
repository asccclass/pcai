package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/asccclass/pcai/internal/agent"
	"github.com/asccclass/pcai/internal/core"
	"github.com/asccclass/pcai/internal/database"
	"github.com/asccclass/pcai/internal/history"
	"github.com/asccclass/pcai/internal/notify"
	"github.com/asccclass/pcai/llms/ollama"
	"github.com/asccclass/pcai/skills"
	"github.com/go-resty/resty/v2"
	"github.com/ollama/ollama/api"
	// 假設你的專案名稱為 pcai
)

// 內部定義優先級
const (
	PriorityUrgent = "URGENT" // 立即通知（如 Boss、家人、警報）
	PriorityNormal = "NORMAL" // 存入記憶，下次對話提醒
	PriorityIgnore = "IGNORE" // 廣告、驗證碼、垃圾訊息
)

type HeartbeatDecision struct {
	Decision string `json:"decision"` // ACTION: NOTIFY_USER, STATUS: IDLE, STATUS: LOGGED
	Reason   string `json:"reason"`   // 為什麼做出這個決定
	Score    int    `json:"score"`    // 0-100 的信心分數
}

type IntentResponse struct {
	Intent string                 `json:"intent"` // 例如: SET_FILTER, CHAT, UNKNOWN
	Params map[string]interface{} `json:"params"` // 提取出的參數，如 pattern, action
	Reply  string                 `json:"reply"`  // AI 給用戶的直接回覆內容
}

type ContactInfo struct {
	Name     string
	Relation string // 關係：Boss, Family, Friend, Unknown
	Priority string
}

// ToolExecutor 定義執行工具的介面
type ToolExecutor interface {
	CallTool(name string, argsJSON string) (string, error)
	GetToolPrompt() string
	GetDefinitions() []api.Tool
}

// PCAIBrain 實作 scheduler.HeartbeatBrain 介面
// 這裡可以放入你的 Ollama 客戶端、記憶管理器、Signal 客戶端等
type PCAIBrain struct {
	DB          *database.DB
	httpClient  *resty.Client
	ollamaURL   string
	filterSkill *skills.FilterSkill
	dispatcher  *notify.Dispatcher
	modelName   string
	tools       ToolExecutor // 加入工具執行器
	tgToken     string
	tgChatID    string
	lineToken   string
}

func (b *PCAIBrain) SetTools(executor ToolExecutor) {
	b.tools = executor
}

func NewPCAIBrain(db *database.DB, ollamaURL, modelName, tgToken, tgChatID, lineToken string) *PCAIBrain {
	brain := &PCAIBrain{
		DB:          db,
		httpClient:  resty.New().SetTimeout(100 * time.Second).SetRetryCount(2),
		ollamaURL:   ollamaURL,
		modelName:   modelName,
		filterSkill: skills.NewFilterSkill(db),
		tgToken:     tgToken,
		tgChatID:    tgChatID,
		lineToken:   lineToken,
	}
	brain.SetupDispatcher()
	return brain
}

// 這是 Heartbeat 決策系統 的「信任名單」— 讓 AI 判斷收到訊息時，哪些人需要緊急處理、哪些可以忽略。
func (b *PCAIBrain) getTrustList() map[string]ContactInfo {
	// 實務上這應該從你的 SQLite 或設定檔讀取
	return map[string]ContactInfo{
		"+886912345678": {Name: "老闆", Relation: "Boss", Priority: PriorityUrgent},
		"+886987654321": {Name: "老婆", Relation: "Family", Priority: PriorityUrgent},
	}
}

// 定義 LLM 回傳的結構與 Prompt
func (b *PCAIBrain) analyzeIntentWithOllama(ctx context.Context, userInput string) (*IntentResponse, error) {
	systemPrompt := `
你是 PCAI 意圖解析助理。請分析用戶輸入並回傳 JSON 格式。
當前作業系統: %s

支援的意圖 (Intent)：
1. SET_FILTER: 當用戶想忽略、過濾、或標記某號碼/關鍵字為重要時。
   - params 需包含: "pattern" (號碼或關鍵字), "action" (URGENT, NORMAL, IGNORE)
2. CHAT: 一般閒聊（**若用戶是在詢問事實、回憶、或查詢具體資訊，請務必使用 TOOL_USE**）。
3. TOOL_USE: 當用戶要求執行特定任務（如列出檔案、讀取網頁），或**查詢記憶/知識庫/人事物資訊**。
   - params 需包含: "tool" (工具名稱), "args" (JSON 物件或 JSON 字串)

   - params 需包含: "tool" (工具名稱), "args" (JSON 物件或 JSON 字串)
   - 重要：列出檔案請優先使用 fs_list_dir (跨平台)，而非 shell_exec。
   - 若必須使用 shell_exec，請根據作業系統選擇正確的指令 (Windows: dir, del, copy; Linux/Mac: ls, rm, cp)。
   - 支援工具列表與詳細參數定義如下:
%s

範例輸入：「請幫我列出當前目錄的檔案」
範例輸出：{"intent": "TOOL_USE", "params": {"tool": "fs_list_dir", "args": {"path": "."}}, "reply": "好的，正在為您列出檔案。"}

用戶輸入："%s"
`
	// 組合完整的 Prompt
	toolPrompt := ""
	if b.tools != nil {
		toolPrompt = b.tools.GetToolPrompt()
	}
	formattedPrompt := fmt.Sprintf(systemPrompt, runtime.GOOS, toolPrompt, userInput)

	// 呼叫 Ollama API (使用 go-resty)
	var result struct {
		Response string `json:"response"`
	}

	resp, err := b.httpClient.R().
		SetContext(ctx).
		SetBody(map[string]interface{}{
			"model":  b.modelName,
			"prompt": formattedPrompt,
			"stream": false,
			"format": "json", // 強制 Ollama 回傳 JSON 格式
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", b.ollamaURL))

	if err != nil {
		return nil, err
	}
	// 使用 resp 來檢查狀態碼
	if resp.IsError() {
		return nil, fmt.Errorf("Ollama 回傳錯誤狀態: %s (代碼: %d)", resp.Status(), resp.StatusCode())
	}

	// 解析 LLM 的 JSON 回覆
	var intent IntentResponse
	if err := json.Unmarshal([]byte(result.Response), &intent); err != nil {
		fmt.Printf("⚠️ 解析意圖失敗，原始回覆:\n%s\n", result.Response)
		return nil, fmt.Errorf("解析意圖失敗: %v", err)
	}

	return &intent, nil
}

// ---------------------------------------------------------
// 1. 環境感知 (Heartbeat Path)
// ---------------------------------------------------------
func (b *PCAIBrain) CollectEnv(ctx context.Context) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("當前時間: %s\n", time.Now().Format("15:04")))

	// A. 載入資料庫中的自訂過濾規則 (自我學習的成果)
	rules, _ := b.DB.GetFilters(ctx)
	if len(rules) > 0 {
		sb.WriteString("### 自訂過濾規則 ###\n")
		for _, r := range rules {
			sb.WriteString(fmt.Sprintf("- 模式: %s -> 處理: %s\n", r["pattern"], r["action"]))
		}
	}

	/*
		// B. 抓取 Signal 訊息
		sb.WriteString("\n### 待處理訊息 ###\n")
		msgs, err := b.fetchSignalMessages(ctx)
		if err != nil {
			sb.WriteString(fmt.Sprintf("錯誤: 無法抓取訊息 (%v)\n", err))
		} else if len(msgs) == 0 {
			return "" // 如果完全沒訊息，回傳空字串讓 Scheduler 跳過這次 Think
		} else {
			for _, m := range msgs {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Source, m.Content))
			}
		}
	*/

	// C. 檢查是否需要執行定期自檢 (Self-Test at 00:00 and 12:00)
	lastTest, err := b.DB.GetLastHeartbeatAction(ctx, "ACTION: SELF_TEST")
	if err != nil {
		fmt.Printf("⚠️ Check last test failed: %v\n", err)
	}
	// 如果在此 12 小時區間 (00:00-11:59 或 12:00-23:59) 尚未執行過
	now := time.Now()
	hour := 0
	if now.Hour() >= 12 {
		hour = 12
	}
	windowStart := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())

	if lastTest.IsZero() || lastTest.Before(windowStart) {
		sb.WriteString("\n### SYSTEM ALERT: DAILY_SELF_TEST_DUE ###\n(Scheduled self-test is due. Please execute SELF_TEST.)\n")
	}

	return sb.String()
}

// ---------------------------------------------------------
// 2. 決策與自我學習 (Logic Path)
// ---------------------------------------------------------
func (b *PCAIBrain) Think(ctx context.Context, snapshot string) (string, error) {
	// 心跳邏輯的 Prompt
	prompt := fmt.Sprintf(`
你現在是 PCAI 自動化決策大腦。請分析以下環境快照並給出 JSON 格式的決策。
%s

規則：
1. 若符合過濾規則且為 IGNORE，回覆 "STATUS: IDLE"。
2. 若訊息包含緊急內容或來自重要人物，回覆 "ACTION: NOTIFY_USER"。
3. 若看見 "SYSTEM ALERT: DAILY_SELF_TEST_DUE"，除非有更緊急的訊息，否則請回覆 "ACTION: SELF_TEST"。

請在 JSON 中加入 "score" 欄位，代表你對此判斷的信心指數 (0-100)：
- 100: 完全確定（如：符合明確的過濾模式）。
- 60 以下: 不太確定（如：內容語意模糊、未見過的號碼但內容像廣告）。
- 90: 系統自檢請求。

請嚴格回覆：
{"decision": "...", "reason": "...", "score": 85}
`, snapshot)

	fmt.Printf("[Brain] 正在思考決策... \n內容:\n%s\n", snapshot)

	// 真正呼叫 Ollama (複用之前的 HTTP 請求結構)
	var result struct {
		Response string `json:"response"`
	}

	resp, err := b.httpClient.R().
		SetContext(ctx).
		SetBody(map[string]interface{}{
			"model":  b.modelName,
			"prompt": prompt,
			"stream": false,
			"format": "json",
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", b.ollamaURL))

	if err != nil {
		return "", fmt.Errorf("Ollama 連線失敗: %w", err)
	}
	// 使用 resp 來檢查狀態碼
	if resp.IsError() {
		return "", fmt.Errorf("Ollama 回傳錯誤狀態: %s (代碼: %d)", resp.Status(), resp.StatusCode())
	}

	// 3. 清理回傳字串（移除 AI 可能多加的空格或換行）
	decision := strings.TrimSpace(result.Response)
	if decision == "" {
		return "", fmt.Errorf("Ollama 回傳內容為空")
	}

	// 解析 JSON 結果
	var dec HeartbeatDecision
	if err := json.Unmarshal([]byte(decision), &dec); err != nil {
		// 容錯：如果是因為超時或其他原因導致回傳了非 JSON 字串 (例如 HTML 錯誤頁面)
		// 我們記錄錯誤但不讓程式崩潰 (雖然這裡 return err 會被上層 recover，或是 log print)
		return "", fmt.Errorf("解析決策 JSON 失敗: %v (原始內容: %.20s...)", err, decision)
	}

	// 核心：將思考過程存入資料庫
	b.DB.CreateHeartbeatLog(ctx, snapshot, dec.Decision, dec.Reason, dec.Score, result.Response)

	// 我們將決策與理由組合成一個字串回傳給 ExecuteDecision，或者修改 interface 傳遞 struct
	// 這裡採用簡單的格式化回傳，方便 ExecuteDecision 處理
	return fmt.Sprintf("%s|%s", dec.Decision, dec.Reason), nil
}

// HandleUserChat 處理用戶的主動指令（自我學習入口）
func (b *PCAIBrain) HandleUserChat(ctx context.Context, sessionID string, userInput string) (string, error) {
	fmt.Printf("[Agent] 正在解析用戶意圖 (Session: %s): %s\n", sessionID, userInput)

	// 嘗試載入對話歷史 (雖然目前 analyzeIntentWithOllama 還沒完全利用它，但先載入以備未來擴充)
	// sess := history.LoadSession(sessionID)
	// TODO: 將 sess.Messages 傳入 analyzeIntentWithOllama 或新的 ChatWithHistory 函式
	// 目前先保持既有邏輯，但已具備 Session 識別能力

	// 讓 Ollama 告訴我們用戶想做什麼
	intentResp, err := b.analyzeIntentWithOllama(ctx, userInput)
	if err != nil {
		return "抱歉，我的大腦現在有點混亂，請稍後再試。", err
	}
	// 根據解析出的意圖執行動作
	switch intentResp.Intent {
	case "SET_FILTER":
		pattern, _ := intentResp.Params["pattern"].(string)
		action, _ := intentResp.Params["action"].(string)

		// 呼叫 Skill 寫入資料庫（實現自我學習）
		_, err := b.filterSkill.Execute(ctx, skills.FilterParams{
			Pattern:     pattern,
			Action:      action,
			Description: fmt.Sprintf("來自對話學習: %s", userInput),
		})
		if err != nil {
			return "設定過濾器時發生資料庫錯誤。", err
		}
		return intentResp.Reply, nil

	case "TOOL_USE":
		// 如果大腦判斷需要使用工具
		toolName, _ := intentResp.Params["tool"].(string)

		// 處理 args: 可能是 string (JSON encoded) 或 map[string]interface{}
		var toolArgs string
		if rawArgs, ok := intentResp.Params["args"]; ok {
			switch v := rawArgs.(type) {
			case string:
				toolArgs = v
			default:
				// 嘗試將物件轉回 JSON 字串
				if bytes, err := json.Marshal(v); err == nil {
					toolArgs = string(bytes)
				} else {
					fmt.Printf("⚠️ 無法將 args 轉為 JSON 字串: %v\n", err)
					toolArgs = "{}"
				}
			}
		} else {
			toolArgs = "{}"
		}

		fmt.Printf("[Agent] 嘗試使用工具: %s, 參數: %s\n", toolName, toolArgs)

		if b.tools == nil {
			return "⚠️ 抱歉，我現在無法使用工具（工具庫未初始化）。", nil
		}

		// 執行工具
		result, err := b.tools.CallTool(toolName, toolArgs)
		if err != nil {
			return fmt.Sprintf("工具執行失敗: %v", err), nil
		}

		return fmt.Sprintf("工具執行結果:\n%s", result), nil

	case "CHAT":
		return intentResp.Reply, nil

	default:
		return "我不確定這是否是一個指令，但我會把它當作一般聊天處理。", nil
	}
}

// ---------------------------------------------------------
// 3. 執行執行 (Action Path)
// ---------------------------------------------------------
func (b *PCAIBrain) SetupDispatcher() {
	// 如果 AI 偵測到同樣的訊息，只要你沒讀，它就不會再吵你；但如果過了一小時你還沒處理，它會再次發送一次提醒。
	dispatcher := notify.NewDispatcher(60 * time.Minute)
	commonClient := resty.New() // 複用同一個 HTTP Client

	// 1. 註冊 Telegram
	if b.tgToken != "" && b.tgChatID != "" {
		dispatcher.Register(&notify.TelegramNotifier{
			Token:  b.tgToken,
			ChatID: b.tgChatID,
			Client: commonClient,
		})
	}

	// 2. 註冊 LINE (僅當有 Token 時)
	if b.lineToken != "" {
		dispatcher.Register(&notify.LineNotifier{
			Token:  b.lineToken,
			Client: commonClient,
		})
	}

	b.dispatcher = dispatcher
}

func (b *PCAIBrain) ExecuteDecision(ctx context.Context, decisionStr string) error {
	if decisionStr == "STATUS: IDLE" || decisionStr == "" {
		return nil
	}

	// 拆分決策與理由
	parts := strings.SplitN(decisionStr, "|", 2)
	decision := parts[0]
	reason := ""
	if len(parts) > 1 {
		reason = parts[1]
	}

	if decision == "STATUS: IDLE" {
		// 你也可以選擇記錄到日誌，方便日後檢查 AI 是否過濾太嚴格
		// log.Printf("[Log] 保持沉默。原因: %s", reason)
		return nil
	}

	fmt.Printf("[Brain] 執行決策: %s\n", decision)
	fmt.Printf("[Reason] AI 判斷理由: %s\n", reason)

	if decision == "ACTION: NOTIFY_USER" {
		msg := fmt.Sprintf("🚨 重要通知！\n理由: %s\n內容: %s", reason, decision)
		// 這裡串接你的 Signal 送信工具或系統通知
		b.dispatcher.Dispatch(ctx, "URGENT", msg)
	}

	if decision == "ACTION: SELF_TEST" {
		return b.RunSelfTest(ctx)
	}

	return nil
}

// AskOllama 是一個通用的輔助方法，用於傳送 Prompt 並獲取純文字回覆
func (b *PCAIBrain) AskOllama(ctx context.Context, prompt string) (string, error) {
	var result struct {
		Response string `json:"response"`
	}

	// 使用我們之前初始化的 httpClient (resty)
	resp, err := b.httpClient.R().
		SetContext(ctx).
		SetBody(map[string]interface{}{
			"model":  b.modelName, // 確保與你本地的模型名稱一致
			"prompt": prompt,
			"stream": false, // 簡報通常較長，關閉 stream 以一次性獲取內容
		}).
		SetResult(&result).
		Post(fmt.Sprintf("%s/api/generate", b.ollamaURL))

	if err != nil {
		return "", fmt.Errorf("Ollama 請求失敗: %w", err)
	}

	if resp.IsError() {
		return "", fmt.Errorf("Ollama 回傳錯誤狀態: %d, 內容: %s", resp.StatusCode(), resp.String())
	}

	// 回傳過濾掉前後空格的純文字結果
	return strings.TrimSpace(result.Response), nil
}

func (b *PCAIBrain) GenerateMorningBriefing(ctx context.Context) error {
	// 1. 撈取昨晚 23:00 以後的日誌
	// 這裡建議在資料庫增加一個 is_briefed 欄位來過濾
	query := `SELECT id, snapshot, reason FROM heartbeat_logs 
	          WHERE created_at > date('now', '-1 day') || ' 23:00:00' 
	          AND is_briefed = 0`

	rows, err := b.DB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query heartbeat logs: %w", err)
	}
	defer rows.Close()

	var logs []string
	var ids []int
	for rows.Next() {
		var id int
		var snp, reas string
		if err := rows.Scan(&id, &snp, &reas); err != nil {
			fmt.Printf("⚠️ 掃描日誌失敗: %v\n", err)
			continue
		}
		logs = append(logs, fmt.Sprintf("- 訊息摘要: %s (判斷理由: %s)", snp, reas))
		ids = append(ids, id)
	}

	if len(logs) == 0 {
		return nil
	}

	// 2. 呼叫我們剛剛寫好的 AskOllama
	prompt := fmt.Sprintf(`
你現在是我的數位管家。昨晚我在睡覺時，你幫我過濾了以下訊息：
%s

請幫我寫一份親切的「晨間簡報」。
要求：
1. 語氣溫暖，像真正的管家。
2. 條列式總結重點，不要逐字念。
3. 告訴我是否有我需要特別留意的趨勢。
`, strings.Join(logs, "\n"))

	briefing, err := b.AskOllama(ctx, prompt)
	if err != nil {
		return err
	}

	// 3. 發送簡報
	b.dispatcher.Dispatch(ctx, "URGENT", "☀️ 早安！昨晚我為您處理了以下事務：\n\n"+briefing)

	// --- 將簡報內容存入日誌資料庫 決策標記為 "REPORT: MORNING"，理由放簡報內容
	err = b.DB.CreateHeartbeatLog(
		ctx,
		"SYSTEM: MORNING_BRIEFING_TRIGGER", // 快照內容標記為系統觸發
		"REPORT: MORNING",                  // 決策類型
		briefing,                           // 將生成的簡報內容存在理由欄位
		100,                                // 信心指數 100
		fmt.Sprintf("Summarized %d logs", len(ids)), // 原始回覆紀錄
	)
	if err != nil {
		fmt.Printf("⚠️ 無法儲存簡報日誌: %v\n", err)
	}

	// 4. 更新舊日誌的標記
	for _, id := range ids {
		b.DB.ExecContext(ctx, "UPDATE heartbeat_logs SET is_briefed = 1 WHERE id = ?", id)
	}

	return nil
}

// RunSelfTest 執行系統自我檢測
func (b *PCAIBrain) RunSelfTest(ctx context.Context) error {
	fmt.Println("🛠️ [SelfTest] Starting daily system self-test...")

	// 1. Database Check
	dbStatus := "✅ PASS"
	if err := b.DB.Ping(); err != nil {
		dbStatus = fmt.Sprintf("❌ FAIL (%v)", err)
	}

	// 2. Internet Check
	netStatus := "✅ PASS"
	if _, err := b.httpClient.R().Get("https://www.google.com"); err != nil {
		netStatus = fmt.Sprintf("❌ FAIL (%v)", err)
	}

	// 3. LLM Check
	llmStatus := "✅ PASS"
	// 給一個簡單的 Ping
	llmResp, err := b.AskOllama(ctx, "Ping. Reply with 'Pong'.")
	if err != nil {
		llmStatus = fmt.Sprintf("❌ FAIL (%v)", err)
	} else if llmResp == "" {
		llmStatus = "❌ FAIL (Empty Response)"
	}

	// 4. Tools Check
	toolStatus := "UNKNOWN"
	var toolDetails strings.Builder
	if b.tools != nil {
		toolStatus = "✅ PASS (Registry Connected)"
		toolDetails.WriteString("\n## 🛠️ Tools & Skills Status\n")
		defs := b.tools.GetDefinitions()
		for _, tool := range defs {
			toolDetails.WriteString(fmt.Sprintf("- **%s**: ✅ Available (%s)\n", tool.Function.Name, tool.Function.Description))
		}
	} else {
		toolStatus = "❌ FAIL (No Tool Executor)"
		toolDetails.WriteString("\n## 🛠️ Tools & Skills Status\n- ❌ Registry Not Connected\n")
	}

	// 產生完整報告 (存檔用)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fullReport := fmt.Sprintf("# Daily System Self-Test Report\nDate: %s\n\n- **Database**: %s\n- **Internet**: %s\n- **LLM**: %s\n- **Tools**: %s\n%s",
		timestamp, dbStatus, netStatus, llmStatus, toolStatus, toolDetails.String())

	// 產生簡短通知 (Telegram用)
	summary := fmt.Sprintf("🛠️ [System] Daily Self-Test Completed.\n\n- **Database**: %s\n- **Internet**: %s\n- **LLM**: %s\n- **Tools**: %s\n\n(See `botmemory/self_test_reports/` for full details)",
		dbStatus, netStatus, llmStatus, toolStatus)

	// 判斷是否需要通知
	shouldNotify := false
	hasError := !strings.Contains(dbStatus, "PASS") || !strings.Contains(netStatus, "PASS") || !strings.Contains(llmStatus, "PASS") || !strings.Contains(toolStatus, "PASS")

	// 檢查此區間 (00:00-11:59 或 12:00-23:59) 是否已執行過
	// GetLastHeartbeatAction returns the *previous* run time since we haven't logged this one yet.
	lastTest, err := b.DB.GetLastHeartbeatAction(ctx, "ACTION: SELF_TEST")
	isFirstTestInWindow := false

	now := time.Now()
	hour := 0
	if now.Hour() >= 12 {
		hour = 12
	}
	windowStart := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())

	if err != nil || lastTest.IsZero() || lastTest.Before(windowStart) {
		isFirstTestInWindow = true
	}

	if hasError || isFirstTestInWindow {
		shouldNotify = true
	} else {
		fmt.Println("ℹ️ [SelfTest] Notification skipped (Not first test in window & no errors).")
	}

	// 儲存完整報告到檔案
	home, _ := os.Getwd()
	reportDir := filepath.Join(home, "botmemory", "self_test_reports")
	_ = os.MkdirAll(reportDir, 0755)

	reportPath := filepath.Join(reportDir, fmt.Sprintf("report_%s.md", time.Now().Format("20060102_150405")))
	if err := os.WriteFile(reportPath, []byte(fullReport), 0644); err != nil {
		fmt.Printf("⚠️ Write report failed: %v\n", err)
	} else {
		fmt.Printf("✅ Report saved to: %s\n", reportPath)
	}

	// 發送通知 (使用簡短摘要)
	if shouldNotify {
		b.dispatcher.Dispatch(ctx, "NORMAL", summary)
	}

	// 寫入 Heartbeat Log (重置計時器)
	err = b.DB.CreateHeartbeatLog(ctx, "SYSTEM: AUTO_TEST", "ACTION: SELF_TEST", "Daily Check Completed", 100, summary)
	return err
}

// RunPatrol 執行閒置時的背景巡邏，讀取 HEARTBEAT.md 的指令並啟動一個 Agent 流程來執行 Tool Calls
func (b *PCAIBrain) RunPatrol(ctx context.Context) error {
	home, _ := os.Getwd()
	data, err := os.ReadFile(filepath.Join(home, "botcharacter", "HEARTBEAT.md"))
	if err != nil {
		fmt.Printf("⚠️ [Heartbeat] 找不到 HEARTBEAT.md，略過背景巡邏 (%v)\n", err)
		return nil
	}

	systemPrompt := string(data)

	// 確保能轉為核心工具註冊表
	registry, ok := b.tools.(*core.Registry)
	if !ok {
		return fmt.Errorf("無法取得工具註冊表")
	}

	// 建立專用的暫時 Session 供背景 Agent 使用，不與主要輸入混淆
	sess := history.NewSession()
	sess.ID = "session_patrol_" + fmt.Sprint(time.Now().Unix()) // 特殊 ID，避免被一般讀取覆蓋
	sess.Messages = append(sess.Messages, ollama.Message{Role: "system", Content: systemPrompt})

	// 建立背景 Agent (不需 Logger 避免洗版)
	myAgent := agent.NewAgent(b.modelName, systemPrompt, sess, registry, nil)

	fmt.Println("🕵️ [Heartbeat] 啟動背景巡邏 (Patrol)...")

	// 我們加上 "SILENT" 短語的預防針在輸入中，這樣如果 AI 決定不要回報任何事情，它就只會輸出 SILENT
	input := fmt.Sprintf("開始執行 Heartbeat 巡邏指令。現在時間是: %s。\n請嚴格遵守執行原則。如果你判斷不需要主動通知我任何事（例如現在是深夜勿擾時間，或者無任何異常），請只回答 'SILENT'。", time.Now().Format("2006-01-02 15:04:05"))

	response, err := myAgent.Chat(input, nil)
	if err != nil {
		return fmt.Errorf("巡邏執行錯誤: %w", err)
	}

	response = strings.TrimSpace(response)

	// 若內容並非宣告安靜，就發送通知給使用者
	if response != "" && !strings.Contains(response, "SILENT") && !strings.Contains(response, "無異常") && !strings.Contains(response, "綠燈") {
		fmt.Printf("🕵️ [Heartbeat] 巡邏回報: 發送通知...\n")
		b.dispatcher.Dispatch(ctx, "NORMAL", response)
	} else {
		fmt.Printf("🕵️ [Heartbeat] 巡邏完畢: 狀態靜默。\n")
	}

	return nil
}
