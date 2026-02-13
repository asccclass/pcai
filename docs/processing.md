# PCAI 系統啟動與執行流程文件

本文件詳細說明 PCAI (Personal Cyber AI) 程式從啟動到執行的完整流程，包括初始化各個組件、載入工具與技能、以及進入互動模式的步驟。

## 1. 程式入口 (Entry Point)

執行檔入口位於專案根目錄的 `main.go`。

- **`main.main()`**: 呼叫 `cmd.Execute()`，將控制權交給 Cobra CLI 框架。

## 2. 命令列解析 (CLI Initialization)

- **`cmd/root.go`**:
    - 定義全域 Flags (如設定檔路徑)。
    - `Execute()` 根據使用者輸入的子命令 (如 `chat`, `server`, `health`) 導向對應的處理函式。

- **`cmd/chat.go`** (`runChat`):
    - 這是主要的互動模式入口 (也是 Telegram Bot 的宿主)。
    - 初始化 UI 渲染器 (Glamour)。
    - 準備背景任務管理器 (`tools.NewBackgroundManager()`)。

## 3. 系統初始化 (System Initialization)

在 `runChat` 中，最關鍵的步驟是呼叫 `tools.InitRegistry` (位於 `tools/init.go`)。

### 3.1 基礎設施與日誌
1. **System Logger**: 在 `cmd/chat.go` 中初始化 `agent.NewSystemLogger("botmemory")`，負責記錄系統操作至 `botmemory/system.log`。
2. **Config**: `config.LoadConfig()` 讀取 `.env` 與設定檔。

### 3.2 核心組件 (Within InitRegistry)
`tools.InitRegistry` 依序初始化以下組件：

1. **Ollama Client**: 建立與本地 LLM 的連線。
2. **Database**: `database.NewSQLiteDB()` 初始化 SQLite 資料庫 (儲存對話歷史與 Metadata)。
3. **Memory Manager**: 
    - 初始化 `memory.NewManager`。
    - 連線至 ChromaDB (Vector Store) 負責 RAG 長期記憶。
    - 建立 `PendingStore` (負責 `memory_save` 的暫存確認機制)。
4. **Scheduler**: `scheduler.NewManager()` 啟動排程器 (Cron Jobs)，並載入已儲存的任務。

### 3.3 工具與技能註冊 (Tool & Skill Registration)
系統建立 `core.Registry` 並註冊能力：

1. **Skill Manager**: 掃描 `skills/` 目錄，解析 `SKILL.md` 定義，將外部腳本轉換為可呼叫的 Prompt-based Skills。
2. **Built-in Tools**: 註冊 Go 語言原生實作的工具 (如 `file_read`, `google_search` 等)。
3. **Skill-First Priority**: 設定 Skills 的優先級為 10 (高於內建工具)，確保 LLM 優先使用專門技能。

### 3.4 Telegram 整合 (Gateway)
若設定檔中包含 Telegram Token：
1. **Agent Adapter**: `gateway.NewAgentAdapter(...)` 初始化。
    - 這是 Telegram 與 Agent 之間的橋樑。
    - 每個 Telegram User 擁有獨立的 `Session` 與 `Agent` 實例。
    - 共用 `SystemLogger` 進行日誌記錄。
2. **Dispatcher**: 負責分派訊息。
3. **Telegram Channel**: 啟動 Polling (Long Polling) 接收訊息。

## 4. Agent 建構與啟動

回到 `cmd/chat.go`：

1. **載入 Session**: `history.LoadLatestSession()` 讀取最近一次的 CLI 對話紀錄。
2. **RAG Summarization**: `history.CheckAndSummarize()`Check 檢查是否需要對過往對話進行歸納 (Summary)，以減少 Context Window 佔用。
3. **Agent 實例化**: `agent.NewAgent(...)`。
    - 注入 `Registry` (工具庫)、`Session` (記憶)、`SystemLogger` (日誌)。
    - 設定 UI Callbacks (`OnGenerateStart`, `OnToolCall`, `OnToolResult`) 以便在終端機顯示即時狀態。

## 5. 主執行循環 (Main Execution Loop)

程式進入 `runChat` 的 `for` 迴圈，等待使用者輸入：

1. **User Input**: 讀取標準輸入 (Stdin)。
2. **Agent Chat**: 呼叫 `myAgent.Chat(input)`。
    - **Log**: 記錄 `user_input`。
    - **Tool Hint Injection**: 根據輸入關鍵字 (如 "commit") 注入工具提示。
    - **Thought Loop (LLM 思考循環)**:
        - LLM 輸出思考過程或工具呼叫請求。
        - **Tool Execution**: 若涉及工具，Agent 透過 Registry 執行工具。
        - **Log**: 記錄 `tool_call` 與 `tool_result` (+ Success/Error)。
        - 將結果回傳給 LLM，繼續生成。
    - **Final Response**: LLM 產生最終回答。
    - **Log**: 記錄 `ai_response`。
3. **UI Rendering**: 使用 Glamour 渲染 Markdown 回應。
4. **Auto-Save**: `history.SaveSession()` 儲存當前對話狀態。

## 6. 程式結束 (Shutdown)

當使用者輸入 `exit` 或收到中斷訊號：
- 觸發 `defer cleanup()`。
- 停止 Telegram Polling。
- 關閉資料庫連線與 Logger。
