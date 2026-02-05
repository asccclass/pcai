# 實作計畫 - 整合 CLI 與 Telegram 流程

## 目標說明
重構程式碼以統一訊息處理邏輯。目前的 CLI 使用 `cmd/chat.go` (Ollama 對話迴圈 + 歷史紀錄 + 工具)，而 Telegram 使用 `internal/heartbeat` (意圖分析 + 自訂邏輯)。本目標是讓 Telegram 使用與 CLI 相同的 "Agent" 邏輯。

## 需要使用者審閱
> [!IMPORTANT]
> 此變更將從根本上改變 Telegram 機器人的行為。它將從「Signal/心跳」類型的機器人（檢查意圖）轉變為完整的「Agent」機器人（具備對話能力、每個使用者的狀態歷史檔案、可使用工具）。
>
> **已知限制**：
> - CLI Session 目前只載入單一的「最新 Session」。對於 Telegram，我們需要**針對每個使用者 (Chat ID)** 管理 Session。
> - 目前的 `history` 套件似乎是為單一本機使用者設計的。我們可能需要調整 `history.LoadSession` 以接受識別符。

## 建議變更

### 1. 建立 `internal/agent` 套件
將 `cmd/chat.go` 中的核心迴圈提取為可重複使用的 `Agent` 結構。

#### [NEW] `internal/agent/agent.go`
- `type Agent struct`: 保存設定 (Model, SystemPrompt) 與狀態 (Session)。
- `func NewAgent(model, systemPrompt string, session *ollama.Session) *Agent`
- `func (a *Agent) Chat(input string, onStream func(string)) (string, error)`:
    - 封裝 `ollama.ChatStream` -> `Tool Call` -> `ollama.ChatStream` 的迴圈。
    - 處理工具輸出的格式化 (CLI 可能會做格式化，但 Agent 應回傳原始文字?)。
    - *決策*: 傳入 `PrintCallback` 進行即時輸出？或是為了 Telegram 使用阻塞式回傳？
    - *改進*: CLI 需要串流。Telegram 可以選擇串流或等待。`Chat` 函數應支援 callback 以處理串流區塊。

### 2. 重構 `internal/history`
如有需要，讓 Session 管理支援多使用者。
- 檢查 `history.LoadLatestSession()`。它可能只尋找特定的檔案模式。
- 我們需要 `history.LoadSession(id string)` 特別針對 Telegram 使用者 (例如使用 `telegram_<chat_id>`)。

### 3. 重構 `cmd/chat.go`
- 簡化 `runChat`，實例化 `agent.Agent` 並在 callback 中處理 UI (Glamour/Lipgloss)。

### 4. 建立 `internal/gateway/telegram_adapter.go` (或修改現有)
- 不再使用 `BrainAdapter` -> `PCAIBrain`，改為建立 `AgentAdapter`。
- `AgentAdapter` 維護一個 `ChatID -> *Agent` 的對照表 (Map)。
- 在 `HandleMessage` 時:
    - 載入/取得該 ChatID 的 Agent。
    - 呼叫 `agent.Chat(input, replyCallback)`。
    - `replyCallback` 將訊息送回 Telegram。

### 5. 更新 `tools/init.go`
- 更新連結 (Wiring)。不再將 Telegram 訊息送給 `PCAIBrain`，而是送給新的 `AgentAdapter`。

## 驗證計畫

### 自動化測試
- 目前無既存測試。將進行手動驗證。

### 手動驗證
1. **CLI 測試**: 執行 `go run main.go chat`。驗證對話歷史、工具使用 (例如 `list_files`) 以及 UI 渲染是否正常。
2. **邏輯檢查**: 追蹤 Telegram 分派的程式路徑，確保它呼叫相同的 `ollama` 函數。
