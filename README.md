# PCAI (Personalized Contextual AI)

**PCAI** 是一個強大的命令行 AI 助手，整合了 Ollama 本地模型、RAG 長期記憶、工具呼叫與 PDF 匯出功能。專為開發者與進階使用者設計，讓 AI 能直接與你的本地環境互動。

## ✨ 主要功能

- **🤖 本地模型整合**: 透過 [Ollama](https://ollama.ai/) 執行各種開源 LLM (如 Llama 3, Mistral 等)。
- **🧠  RAG 長期記憶 (Long-term Memory)**:
    - 自動記錄對話歷史至 `~/botmemory/`。
    - 閒置時自動歸納重點並存入 `knowledge.md`。
    - 具備 `knowledge_search` 工具，讓 AI 能回憶起過去的知識。
- **🛠️ 工具呼叫 (Function Calling)**:
    - `shell_exec`: 讓 AI 執行本地 Shell 指令 (如 `ls`, `git status`)。
    - `fetch_url`: 爬取網頁內容，讓 AI 讀取最新網路資訊。
    - `list_files`: 檢視專案目錄結構。
    - `manage_cron_job`: 設定定時任務 (如定期讀取 Gmail、自動備份)。
    - `convert_videos`: 批量影片轉檔工具 (支援多執行緒與智慧複製)。
    - `knowledge_append`: 主動將重要資訊寫入長期記憶。
- **📡 擴充整合**:
    - **Signal 聊天**: 支援透過 `signal-cli` 將 AI 串接到 Signal App，隨時隨地對話。
    - **Gmail 助手**: 可排程讀取重要郵件 (如 `edu.tw`) 並自動摘要。
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
+
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

## 🤝 貢獻

歡迎提交 Pull Request 或 Issue！

## 📄 授權

MIT License