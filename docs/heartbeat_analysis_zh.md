# PCAI HeartBeat 機制分析

## 1. 核心概念 (Core Concept)

PCAI 的 HeartBeat 是一個基於 "Sense-Think-Act" (感知-思考-行動) 迴圈的自動化決策系統。它的主要目的是讓 AI 能夠主動感知環境變化、進行邏輯判斷，並執行相應的動作，而不僅僅是被動等待使用者的指令。

## 2. 架構組成 (Architecture)

主要涉及以下幾個模組：

*   **`internal/heartbeat/processor.go` (核心邏輯)**: 定義了 `PCAIBrain`，實作了感知、思考和行動的具體邏輯。
*   **`internal/scheduler/manager.go` (驅動引擎)**: 負責排程和觸發 HeartBeat，以及管理其他 Cron 任務。
*   **`tools/init.go` (初始化)**: 負責組裝 `PCAIBrain` 和 `Manager`，並註冊相關任務。

## 3. 工作流程 (Workflow)

HeartBeat 的運作流程如下：

### 3.1. 觸發 (Trigger)
由 `scheduler.Manager` 中的 Cron 排程觸發。
- 預設頻率：每 20 分鐘 (`*/20 * * * *`) 呼叫一次 `runHeartbeat()`。
- 並發控制：使用 `atomic.CompareAndSwapInt32` 確保同一時間只有一個 HeartBeat 在執行 (`isThinking`)。

### 3.2. 感知 (Sense) - `CollectEnv`
`PCAIBrain.CollectEnv` 收集當前的環境快照 (Snapshot)，包含：
1.  **當前時間**: 判斷是否為工作時間或休息時間。
2.  **資料庫過濾規則**: 載入使用者設定或自我學習的 `Filter` 規則。
3.  **系統狀態**: 檢查上次執行 `SELF_TEST` 的時間，若超過 24 小時則發出警報 (`SYSTEM ALERT: DAILY_SELF_TEST_DUE`)。
*(註：程式碼中也有預留 Signal 訊息的讀取邏輯，目前被註解掉)*

### 3.3. 思考 (Think) - `Think`
`PCAIBrain.Think` 將 `CollectEnv` 產生的快照發送給 LLM (Ollama) 進行決策。
- **Prompt 設計**: 要求 LLM 扮演「自動化決策大腦」，根據規則 (如 Ignore, Notify, Self-Test) 輸出 JSON 格式的決策。
- **決策結構 (`HeartbeatDecision`)**:
    - `decision`: 動作代碼 (如 `STATUS: IDLE`, `ACTION: NOTIFY_USER`, `ACTION: SELF_TEST`)。
    - `reason`: 決策理由。
    - `score`: 信心分數 (0-100)。
- **記錄**: 將思考過程和決策結果存入資料庫 (`heartbeat_logs`)。

### 3.4. 行動 (Act) - `ExecuteDecision`
根據 LLM 的決策執行對應動作：
- **`STATUS: IDLE`**: 什麼都不做 (AI 決定保持沉默)。
- **`ACTION: NOTIFY_USER`**: 透過 `dispatcher` (Telegram/Line) 發送重要通知給使用者。
- **`ACTION: SELF_TEST`**: 執行系統自我檢測 (`RunSelfTest`)。

## 4. 特殊功能 (Key Features)

### 4.1. 系統自我檢測 (Self-Test)
- **觸發**: 當 HeartBeat 發現超過 24 小時未執行自檢時，LLM 會決定執行 `ACTION: SELF_TEST`。
- **內容**: 檢查資料庫連線、網際網路連線、LLM 回應能力、工具庫狀態。
- **報告**: 產生詳細報告存檔 (`botmemory/self_test_reports/`)，並發送簡報給使用者。

### 4.2. 晨間簡報 (Morning Briefing)
- **觸發**: 透過 `scheduler` 獨立排程 (每天 07:00)。
- **內容**: 彙整昨晚的 HeartBeat 日誌 (過濾掉的訊息)，並使用 LLM 生成一份溫暖的晨間簡報發送給使用者。

### 4.3. 信任名單與意圖分析
- **信任名單**: 內建 `ContactInfo` (如 Boss, Family)，用於判斷訊息優先級。
- **意圖分析**: `HandleUserChat` 處理使用者主動對話，區分 `CHAT` (閒聊)、`TOOL_USE` (使用工具) 和 `SET_FILTER` (設定過濾規則)。

## 5. 總結

HeartBeat 賦予了 PCAI **主動性**。它不再只是一個被動的問答機器人，而是一個能夠：
1.  **定期巡邏** (Cron Schedule)。
2.  **感知環境** (CollectEnv)。
3.  **自主判斷** (LLM Decision)。
4.  **維護自身健康** (Self-Test)。
5.  **主動回報** (Notify/Briefing) 的智慧代理人。
