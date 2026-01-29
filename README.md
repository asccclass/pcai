# PCAI (個人電腦 AI)

PCAI 是一個用 Go 語言編寫的本地 AI 代理實作，它整合了 [Ollama](https://ollama.com/)，透過函數呼叫 (工具) 在您的本地機器上執行任務。

## 功能

- **Ollama 整合**：連接到本地或遠端的 Ollama 實例 (目前配置為使用 `llama3.3`)。
- **工具註冊表**：一個模組化的系統，用於註冊和管理 AI 工具。
- **內建工具**：
  - **影片轉檔器**：使用 FFmpeg 批次轉換影片檔案 (在適用的情況下支援硬體加速偵測)。
  - **檔案列表器**：列出目錄中的檔案。
  - **時間查詢**：獲取當前時間。

## 先決條件

在執行此專案之前，請確保您已安裝以下軟體：

1.  **Go** (建議使用 1.25 或更高版本)。
2.  **Ollama**：
    -   確保已下載 `llama3.3` 模型：`ollama pull llama3.3`。
    -   確保 Ollama 伺服器正在運行。
3.  **FFmpeg**：
    -   影片轉檔工具所需。
    -   必須已加入到您系統的 PATH 環境變數中。

## 安裝

1.  複製儲存庫：
    ```bash
    git clone https://github.com/asccclass/pcai.git
    cd pcai
    ```

2.  下載相依套件：
    ```bash
    go mod tidy
    ```

## 設定

專案使用 `envfile` 進行設定，不過為了演示目的，目前部分設定在 `pcai.go` 中有強制覆寫的程式碼。

-   **Ollama 主機**：目前在 `pcai.go` 中強制設定為 `http://172.18.124.210:11434`。您可能需要將此行修改為 `http://localhost:11434` 或您特定的 Ollama 主機位置。

## 使用方法

執行代理：

```bash
go run pcai.go
```

### 目前的演示行為
`main` 函數目前執行一個寫死的演示場景：
-   **使用者查詢**：「幫我把電腦桌面下的D:\Backup\Desktop\mkv目錄內的影音檔案都轉換為mp4格式，並放置於：D:\Backup\Desktop\mkv目錄下」
-   AI 會偵測意圖，呼叫 `convert_videos` 工具，並使用 FFmpeg 執行轉檔。

### Signal 整合

```
curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Sec-WebSocket-Key: SGVsbG8sIHdvcmxkIQ==" -H "Sec-WebSocket-Version: 13" https://msg.justdrink.com.tw/v1/receive/+886921609364
```

## 專案結構

-   `pcai.go`：主要程式進入點。設定客戶端、註冊表並執行聊天迴圈。
-   `tools/`：包含工具定義和實作。
    -   `registry.go`：管理工具註冊與執行。
    -   `video_converter.go`：影片轉檔的 FFmpeg 包裝器。
    -   `list_files.go`：檔案系統列表工具。
    -   `query_time.go`：時間查詢工具。