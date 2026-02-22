# PCAI 系統架構與運作流程總覽

## 1. 專案概觀

**PCAI (Personal AI)** 是一個以 CLI (命令列介面) 為主的 AI 代理系統。它不僅是一個聊天機器人，更具備「主動感知」、「長期記憶 (RAG)」與「自我學習」的能力。系統核心整合了 **Ollama** 本地大語言模型，並透過 **Go** 語言實作了強大的工俱調度與背景任務管理系統。

## 2. 核心架構 (Core Architecture)

系統主要由以下四大層級組成：

### 2.1 對話交互層 (Interaction Layer)
- **入口**: `cmd/chat.go`
- **職責**: 處理使用者輸入、顯示 AI 回應、管理當前對話 Session。
- **特點**:
    - 支援 **Rich UI** (使用 Glamour/Lipgloss 渲染 Markdown)。
    - 實作了 **Tool-Calling  State Machine** (思考 -> 呼叫工具 -> 再次思考 -> 回應)。
    - 自動注入 RAG 長期記憶至 System Prompt。

### 2.2 記憶與認知層 (Memory & Cognition Layer)
- **路徑**: `internal/history`, `botmemory/`
- **職責**: 讓 AI 擁有跨 Session 的記憶能力。
- **機制**:
    - **Short-term**: 記憶當前 Session 的對話內容。
    - **Long-term (RAG)**: 
        - 系統會在對話結束或閒置時，自動呼叫 AI 將對話歸納為重點。
        - 歸納結果存入 `botmemory/knowledge/knowledge.md`。
        - 每次新對話開始時，讀取 `knowledge.md` 的最後 2000 字元注入 System Prompt，讓 AI「想起」之前的知識。

### 2.3 心跳與排程層 (Heartbeat & Scheduler Layer) (獨特亮點 ✨)
- **路徑**: `internal/heartbeat`, `internal/scheduler`
- **職責**: 賦予 AI 「時間感」與「主動性」，使其不僅是被動回應，還能主動執行任務。
- **機制**:
    - **Scheduler**: 基於 Cron 的排程器，每 20 分鐘 (預設) 觸發一次 Heartbeat。
    - **Heartbeat Brain (`PCAIBrain`)**: 
        - **Sensing (感知)**: 收集環境資訊 (時間、DB 過濾規則、Signal 未讀訊息等)。
        - **Thinking (思考)**: 將環境快照丟給 LLM，詢問「現在該做什麼？」。
        - **Action (行動)**: 根據 LLM 的決策執行動作 (如發送通知、忽略訊息、寫入日誌)。

### 2.4 工具與技能層 (Tools & Skills Layer)
- **路徑**: `tools/`, `skills/`
- **職責**: 擴充 AI 的能力邊界。
- **主要工具**:
    - `shell_exec`: 執行系統命令。
    - `fetch_url`: 爬取網頁。
    - `video_converter`: 影片轉檔 (Worker Pool 處理)。
    - `knowledge_search/append`: 存取長期記憶。
    - `scheduler`: 管理定時任務。

---

## 3. 關鍵運作流程 (Key Workflows)

### 3.1 互動對話流程 (Interactive Chat Flow)

1. **啟動**: 使用者執行 `./pcai chat`。
2. **記憶載入**: 
   - 讀取 `botmemory/knowledge/knowledge.md`。
   - 組合 System Prompt: `(User Defined System Prompt) + (RAG Knowledge)`.
3. **對話循環**:
   - 使用者輸入 -> Append to History.
   - 呼叫 LLM (Ollama).
   - **工具判斷**: 
     - 若 LLM 決定呼叫工具 (e.g., `shell_exec`) -> 執行工具 -> 將結果回傳給 LLM (Role: Tool)。
     - 重複此步驟直到 LLM 產出最終文字回應。
4. **記憶歸納**:
   - 對話過程中或結束時，觸發 `CheckAndSummarize`。
   - 將繁瑣的對話紀錄精煉為「知識點」並存回 `knowledge.md`。

### 3.2 系統心跳流程 (System Heartbeat Flow)

這是一個在背景運行的無限循環 (由 `scheduler` 管理)：

1. **觸發**: Cron 定時器 (e.g., 每 20 分)。
2. **感知 (CollectEnv)**:
   - 抓取系統時間。
   - 從 SQLite 讀取 `filters` (AI 自己設定的規則)。
   - (未來) 檢查 Signal/Gmail 內容。
3. **思考 (Think)**:
   - Prompt: "你是 PCAI 的大腦，當前環境是 [Snapshot]，請決定動作 (IGNORE / NOTIFY / ...)"。
   - LLM 回傳 JSON 決策。
4. **決策執行 (Execute)**:
   - 若決策為 `NOTIFY` -> 發送通知給使用者。
   - 紀錄決策過程至 SQLite `heartbeat_logs` (供除錯與未來分析)。

### 3.3 自我學習機制 (Self-Learning Flow)

AI 如何透過對話變聰明？

1. **使用者指令**: 用戶在對話中說：「以後看到 +8869xx 的電話都不要理它」。
2. **意圖識別 (`HandleUserChat`)**: 
   - LLM 分析意圖為 `SET_FILTER`。
   - 提取參數: `pattern="+8869xx"`, `action="IGNORE"`.
3. **寫入資料庫**: 呼叫 `skills.FilterSkill` 將規則寫入 SQLite `filters` 表。
4. **生效**: 下次 Heartbeat 運作時，`CollectEnv` 會讀取這條新規則，AI 就能根據這條規則自動過濾訊息。

---

## 4. 專案目錄結構

```
pcai/
├── cmd/                # CLI 命令入口 (chat, health 等)
│   └── chat.go       # 核心對話邏輯
├── internal/
│   ├── heartbeat/    # AI 大腦與心跳邏輯 (PCAIBrain)
│   ├── scheduler/    # 排程與 Worker Pool 管理
│   ├── history/      # RAG 與 Session 管理
│   ├── database/     # SQLite 連線與 Schema
│   └── config/       # 設定檔讀取
├── tools/              # AI 可呼叫的工具集 (Shell, Video, Web...)
├── skills/             # 高階技能 (如 FilterSkill，整合 DB 操作)
├── botmemory/          # 長期記憶儲存區 (Markdown 檔案)
└── main.go             # 程式入口
```

## 5. 技術重點

- **LLM 整合**: 透過 REST API 與 **Ollama** 溝通 (支援 Llama 3, Mistral 等)。
- **資料庫**: 使用 **SQLite** (Pure Go `modernc.org/sqlite`)，無需 CGO，易於跨平台編譯。
- **並發模型**: 
    - 使用 **Go Rutoutines** 與 **Channels** 處理背景任務 (如影片轉檔)。
    - 使用 `sync.Atomic` 保護 Heartbeat 狀態，避免重複執行。
- **CLI UI**: 使用 `charmbracelet` 生態系 (`lipgloss`, `glamour`) 打造現代化終端介面。
