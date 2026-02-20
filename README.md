# PCAI (Personalized Contextual AI)

**PCAI** 是一個強大的命令行 AI 助手，整合了 Ollama 本地模型、RAG 長期記憶、工具呼叫與 PDF 匯出功能。專為開發者與進階使用者設計，讓 AI 能直接與你的本地環境互動。

## ✨ 主要功能

- **🤖 本地模型整合**: 透過 [Ollama](https://ollama.ai/) 執行各種開源 LLM (如 Llama 3, Mistral 等)。
- **🧠 雙層記憶系統 (Dual Memory)**:
    - **永久記憶**: 透過 `knowledge_append` 將重要資訊存入 `knowledge.md`，永久保存。
    - **短期記憶**: 工具回應 (天氣/行事曆/郵件) 與對話內容自動存入 SQLite，預設 7 天後自動清理。
    - **RAG 長期記憶**: 閒置時自動歸納重點，具備 `knowledge_search` 向量搜尋。
- **🛠️ 工具呼叫 (Function Calling)**:
    - `shell_exec`: 讓 AI 執行本地 Shell 指令，**自動偵測 OS** 翻譯指令 (如 `ls` → `dir`)。
    - `fetch_url`: 爬取網頁內容，讓 AI 讀取最新網路資訊。
    - `fs_list_dir`: 跨平台檔案列表 (Go 原生 API，不依賴 Shell)。
    - `manage_cron_job`: 設定定時任務 (如定期讀取 Gmail、自動備份)。
    - `convert_videos`: 批量影片轉檔工具 (支援多執行緒與智慧複製)。
    - `knowledge_append`: 主動將重要資訊寫入永久記憶。
    - `skill_scaffold`: 自動建立新 Skill 骨架目錄 (含 SKILL.md 範本)。
    - `skill_validate`: 驗證 Skill 是否符合 agentskills.io 規格。
- **🎯 智慧工具路由 (Tool Hints)**:
    - 自動偵測使用者意圖，導向正確工具（如「記住」→ `knowledge_append`、「列出檔案」→ `fs_list_dir`）。
    - 處理 LLM 工具名稱幻覺（自動修正錯誤工具名稱）。
    - 全域 JSON 參數清理（處理 LLM 產生的巢狀格式）。
- **☀️ 每日晨間簡報**:
    - 每天 06:30 自動執行，整合 Email + 行事曆 + 天氣。
    - 透過 LLM 摘要後發送到 Telegram。
- **📡 擴充整合**:
    - **Telegram 聊天**: 支援透過 Telegram Bot 與 AI 對話，並自動存入短期記憶。
    - **Gmail 助手**: 可排程讀取重要郵件並自動摘要。
    - **自動備份**: 支援 `backup_knowledge` 任務，定期備份長期記憶庫。
- **📄 PDF 匯出**: 支援將對話紀錄匯出為 PDF 文件 (自動適配系統字型)。
- **💻 跨平台支援**: 支援 Windows, macOS 與 Linux (含 ARM64 架構)。

## 🚀 快速開始

### 前置需求

1. 安裝 [Go 1.25+](https://go.dev/dl/)
2. 安裝並啟動 [Ollama](https://ollama.ai/)
3. 下載模型 (例如 `llama3`):
   ```bash
   ollama pull llama3
   ```

### 安裝與編譯

```bash
# 下載專案
git clone https://github.com/asccclass/pcai.git
cd pcai

# 編譯 (Windows)
make build-win

# 編譯 (Linux/Mac)
make build
```

### 使用方式

啟動對話模式：

```bash
# 預設啟動
./pcai.exe chat

# 指定模型與系統提示詞
./pcai.exe chat -m llama3 -s "你是一個資深 Golang 工程師"
```

## ⚙️ 進階功能設定

### 🧩 Skill 開發工具

PCAI 內建兩個 Skill 開發工具，使用者或 LLM 皆可直接呼叫：

| 工具 | 說明 | 參數 |
|------|------|------|
| `skill_scaffold` | 建立新 Skill 骨架 | `skill_name` (必填), `description` (必填), `command` (選填) |
| `skill_validate` | 驗證 Skill 規格 | `skill_name` (必填) |

使用範例：
```
> 幫我建立一個圖片搜尋技能
→ PCAI 呼叫 skill_scaffold(skill_name="image_search", description="搜尋網路圖片")

> 檢查 weather 技能有沒有問題
→ PCAI 呼叫 skill_validate(skill_name="weather")
```

骨架結構遵循 [agentskills.io](https://agentskills.io) 開源規格，詳見 `skills/skill-creator/`。

+### 💬 Signal 整合
+
+若要啟用 Signal 聊天功能，請先安裝 `signal-cli-rest-api` 並設定環境變數：
+
+1. 啟動 `signal-cli-rest-api` 服務。
+2. 設定環境變數 (或建立 `.env` 檔案)：
+   ```ini
+   SignalHost=localhost:8080
+   SignalNumber=+886912345678  # 你的 Signal 號碼
+   ```
+3. 啟動 PCAI 後，系統會自動連線並監聽訊息。
+
+### 📧 Gmail 助手
+
+欲使用 Gmail 排程讀取功能：
+
+1. 至 Google Cloud Console 啟用 **Gmail API**。
+2. 下載 `credentials.json` 並放置於專案根目錄。
+3. 首次執行 `read_email` 任務時，終端機將顯示授權連結，請點擊並輸入驗證碼。
+
+### 🎥 影片轉檔
+
+直接呼叫 AI 即可：
+> "幫我把 assets 資料夾裡的影片轉成 mp4，使用 4 個執行緒"
+
+AI 會自動呼叫 `convert_videos` 工具，並利用 `ffmpeg` 進行高效轉檔 (支援 Smart Copy 模式)。
+
+## 🛠️ 開發指南

專案結構說明：

- `cmd/`: CLI 指令入口 (Cobra 框架)。
- `internal/history/`: 對話紀錄與 RAG 邏輯。
- `llms/ollama/`: Ollama API 客戶端串接。
- `tools/`: 各式 AI 工具實作 (Shell, Web Scraper 等)。
- `export/`: PDF 匯出邏輯。
- `assets/`: 字型與靜態資源。

## 功能測試

```
go test ./systemtesting/... -v
```



## 🤝 貢獻

歡迎提交 Pull Request 或 Issue！

## 📄 授權

MIT License