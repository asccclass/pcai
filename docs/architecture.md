# PCAI 系統架構說明：記憶與對話 (Memory & Session)

本文件說明 PCAI 系統中關於對話狀態管理 (Session) 與長期/短期記憶 (Memory) 的核心架構。

## 1. 對話狀態 (Session)

Session 負責儲存當前的對話上下文 (Context Window)，讓 Agent 能夠進行連續對話。

### 核心組件
- **結構定義**: `internal/history/session.go`
    - `Session` struct 包含 `ID` (字串), `Messages` (對話陣列), `LastUpdate` (時間戳)。
- **儲存位置**: `botmemory/history/*.json`
    - 每個 Session 存為一個 JSON 檔案。
    - CLI 模式預設讀取最新修改的 Session (`LoadLatestSession`)。
    - Telegram 模式為每個 User ID 建立獨立 Session (`telegram_{UserID}.json`)。

### 運作流程 (Telegram 範例)
1.  **用戶傳訊**: `AgentAdapter` 接收訊息。
2.  **載入/建立**: 呼叫 `history.LoadSession("telegram_" + userID)`。若無則建立新檔。
3.  **對話**: 將用戶訊息 append 到 `Messages`，送入 LLM。
4.  **回應**: 將 LLM 回應 append 到 `Messages`。
5.  **存檔**: 呼叫 `history.SaveSession()` 寫回 JSON。
6.  **歸納 (RAG)**: 背景執行 `history.CheckAndSummarize`，若對話過長或閒置過久，會觸發歸納機制（目前主要依賴時間或手動觸發）。

---

## 2. 記憶系統 (Memory)

記憶系統負責長期保存知識、事實與重要資訊，並支援語意搜尋 (Semantic Search)。

### 核心組件
1.  **Manager (`internal/memory/manager.go`)**:
    - **Vector Store**: 維護一個記憶條目 (`Entry`) 的列表。
    - **Embedding**: 使用 Ollama (`mxbai-embed-large`) 將文字轉為向量。
    - **Persistence**: 存於 `botmemory/knowledge/memory_store.json` (JSON 格式的向量庫)。
    - **Search**: 支援 Cosine Similarity + 關鍵字加權混合搜尋。

2.  **Controller (`internal/memory/controller.go`)**:
    - **Router**: 負責協調記憶的寫入流程。
    - **Integration**: 整合 `Manager` (長期儲存) 與 `PendingStore` (待確認區)。

3.  **Pending Store (`internal/memory/pending_store.go`)**:
    - **Buffer**: 暫存 AI 提取的記憶，等待用戶確認。
    - **TTL**: 預設 24 小時過期。
    - **Confirmation**: 用戶透過 `memory_confirm` 工具批准後，才寫入 Manager。

4.  **Skills (`internal/memory/skills.go`)**:
    - **Extraction**: 定義如何從對話中提取記憶（雖程式碼有框架，目前主要依賴 LLM Tool Use 直接呼叫 `memory_save`）。

### 儲存位置
- **向量庫**: `botmemory/knowledge/memory_store.json` (程式讀取用)
- **可讀日誌**: `botmemory/knowledge/knowledge.md` (人類閱讀用，Markdown 格式)
- **短期記憶 (SQLite)**: `pcai.db` -> table `short_term_memory` (用於晨間簡報、暫存對話摘要)。

### 運作流程 (記憶寫入)
1.  **觸發**: 用戶說 "記住我喜歡藍色"。
2.  **工具呼叫**: Agent 呼叫 `memory_save` 工具。
3.  **暫存**: 內容存入 `PendingStore`，回傳 Pending ID。
4.  **通知**: Agent 告知用戶 "已暫存，請確認"。
5.  **確認**: 用戶說 "確認" 或呼叫 `memory_confirm`。
6.  **寫入**: Controller 將內容移入 `Manager` (計算向量並存檔) 並追加到 `knowledge.md`。

## 3. 架構圖示

```mermaid
graph TD
    User[User (Telegram/CLI)] --> Adapter[Agent Adapter]
    Adapter --> Session[Session Manager]
    Session <--> HistFile[(history/*.json)]
    
    Adapter --> Agent[AI Agent]
    Agent --> Tools[Tool Registry]
    
    subgraph Memory System
        Tools --> MemSave[memory_save Tool]
        Tools --> MemConfirm[memory_confirm Tool]
        Tools --> MemSearch[memory_search Tool]
        
        MemSave --> Pending[Pending Store (RAM)]
        MemConfirm --> Pending
        MemConfirm --> Controller
        
        Controller --> Manager[Memory Manager]
        Manager <--> VecDB[(memory_store.json)]
        Manager <--> Markdown[(knowledge.md)]
    end
    
    subgraph ShortTerm
        Adapter --> SQLite[(pcai.db)]
    end
```
