# Project Comparison: PCAI vs OpenClaw

這份文件詳細比較了本專案 (PCAI) 與開源專案 [OpenClaw](https://github.com/openclaw/openclaw) 的功能、架構差異及執行方式。

## 1. 專案概觀 (Overview)

| 特性 | **PCAI (本專案)** | **OpenClaw** |
| :--- | :--- | :--- |
| **核心定位** | 個人電腦伴侶 (Personal Companion)，強調單一執行檔、輕量化與本地優先。 | AI 作業系統 (AI OS)，強調作為連接多種服務的 Gateway，架構較為龐大。 |
| **程式語言** | Go (Golang) | TypeScript / Node.js |
| **執行方式** | 單一執行檔 (`pcai.exe`)，直接在終端機運行。 | 容器化服務 (Docker Compose) 或 Node.js Runtime。 |
| **部署難度** | 低 (下載即用，依賴少) | 中 (需設定 Docker、Node 環境、Redis 等) |

## 2. 功能對照 (Feature Mapping)

下表列出 OpenClaw 的核心功能與 PCAI 的對應實作：

| 功能領域 | **OpenClaw 功能** | **PCAI 對應實作** | **狀態/差異** |
| :--- | :--- | :--- | :--- |
| **訊息通訊** | 支援 WhatsApp, Telegram, Discord, Slack, Teams, Signal 等多種平台。 | 支援 **Telegram** (`telego`) 與 **WhatsApp** (`whatsmeow`)。 | ⚠️ PCAI 支援平台較少，主要集中在個人常用通訊軟體。 |
| **LLM 支援** | 模型無關 (Model Agnostic)，支援 OpenAI, Anthropic, Local (Ollama) 等。 | 支援 **Ollama** (本地) 與 OpenAI 介面。 | ✅ 兩者皆支援本地模型，PCAI 更深度整合 Ollama。 |
| **系統權限** | 完整的系統存取權 (Shell, File, Browser)。 | 具備 **Shell Tool**, **Filesystem Tool**, **Google Tool** 等。 | ✅ 功能相當，PCAI 針對 Windows/Linux 有適配。 |
| **網頁瀏覽** | 專屬 Browser Layer，使用 "Semantic Snapshots" 解析網頁。 | 使用 **Chromedp** 與 **Rod** (曾用) 進行無頭瀏覽器控制。 | ⚠️ OpenClaw 的語意快照技術可能在 token 效率上較優。 |
| **記憶系統** | 長期記憶 (Long-term Memory)，跨對話與平台。 | **BotMemory** 模組，包含向量搜尋 (Vector) 與混合檢索。 | ✅ 兩者皆具備 RAG 能力。 |
| **自動化** | Heartbeat 機制 (主動觸發任務)。 | **Heartbeat Processor** (`internal/heartbeat`) 與 **Cron Tool**。 | ✅ 兩者皆能主動發起任務，而非僅被動回應。 |
| **技能擴充** | 自定義 Skills，具備寫程式擴充能力。 | **Skills Directory** 與 **Skill Generator Tool** (自動寫 code)。 | ✅ PCAI 具備 `generate_skill` 工具，可讓 AI 自行撰寫新技能。 |
| **多代理人** | Multi-Agent Routing (路由至不同 Agent)。 | 內部有 `agent`, `advisor` 模組，但主要為單一 Agent 架構。 | ⚠️ PCAI 為單一強大 Agent，OpenClaw 支援多 Agent 協作。 |
| **介面 (UI)** | Live Canvas (視覺化介面)。 | **CLI (Glamour)** 與 簡單 Web 管理介面 (`cmd/server.go`)。 | ⚠️ PCAI 缺乏複雜的圖形化互動介面 (Canvas)。 |

## 3. 本專案缺少的地方 (Missing Features)

相較於 OpenClaw，PCAI 目前在以下方面有所欠缺：

1.  **多平台支援廣度 (Channel Breadth)**:
    *   OpenClaw 是一個通訊樞紐，支援 Slack, Discord, Teams 等企業級通訊軟體。
    *   PCAI 目前僅專注於 Telegram 與 WhatsApp，缺乏企業協作平台的整合。

2.  **視覺化互動介面 (UI/UX)**:
    *   OpenClaw 提供了 "Live Canvas" 讓 Agent 可以繪製圖表或呈現複雜資訊。
    *   PCAI 主要依賴終端機 (CLI) 的文字渲染 (Glamour)，雖然美觀但互動性有限。

3.  **瀏覽器控制的細緻度**:
    *   OpenClaw 強調 "Semantic Snapshots" (語意快照) 來解決 LLM 瀏覽網頁的 Token 消耗與準確度問題。
    *   PCAI 使用標準的 DOM 抓取或截圖，對於動態複雜網頁的處理可能不如 OpenClaw 的架構最佳化。

4.  **沙箱安全性 (Sandboxing)**:
    *   OpenClaw 強調工具執行的沙箱化 (Docker 內)。
    *   PCAI 直接在 Host 系統執行 (Shell Exec)，雖然方便但風險較高，依賴使用者審核指令。

## 4. 執行方式與架構差異 (Execution & Architecture)

### OpenClaw 架構
*   **Hub-and-Spoke**: 是一個中央服務器 (Gateway)，所有訊息平台都連線到這個 Gateway。
*   **微服務化**: 需要 Redis (Queue), Postgres (DB), Sandbox (Docker) 等多個組件配合。
*   **適用場景**: 適合部署在伺服器上，作為一個長駐的服務，服務多個使用者或是作為多個平台的統一窗口。

### PCAI 架構
*   **Monolithic (單體)**: 所有功能 (HTTP Server, Telegram Bot, Agent Logic, Vector DB) 都編譯在一個 Go Binary 中。
*   **Embedded**: 資料庫使用 SQLite (Embedded)，向量庫可能也是本地實作或依賴 Ollama。
*   **適用場景**: 適合個人使用者 (Single User)，跑在筆電或桌機 (Windows/Mac/Linux) 背景，作為個人的數位助理。
*   **特色**: 啟動快，無須架設複雜環境，資料完全本地化。

## 5. 總結

**OpenClaw** 是一個強大的 **AI 基礎設施**，適合想要架設自己的 "Jarvis 伺服器" 並串接各種服務的進階用戶或開發者。

**PCAI** 則是一個 **輕量級的個人代理**，專注於 "帶著走" 的能力，直接在你的工作電腦上運行，幫助你操作本機檔案、排程與查詢資訊。它的優勢在於**部署極簡**與**系統整合深度** (直接控制本機 OS)。
