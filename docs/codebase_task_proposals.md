# 程式碼庫巡檢：優先修正任務提案

本文件整理一次快速巡檢後建議優先處理的 4 類任務：拼字、程式錯誤、文件差異與測試改進。

## 1) 拼字修正任務：`cient.go` 檔名更正為 `client.go`

- **問題觀察**：`llms/ollama/` 目錄下存在 `cient.go`，檔名明顯為 `client.go` 的拼字錯誤。
- **風險/影響**：
  - 降低可讀性與可搜尋性（新進開發者不易定位「client」職責）。
  - 工具鏈（例如以檔名規則做靜態分析）可能出現例外或漏掃。
- **任務內容**：
  1. 將 `llms/ollama/cient.go` 重新命名為 `llms/ollama/client.go`。
  2. 確認沒有任何腳本或文件硬編碼引用舊檔名。
  3. 執行 `go test ./...` 驗證重命名未造成建置或測試問題。
- **完成定義（DoD）**：
  - Git 歷史清楚顯示 rename。
  - 全專案測試可通過。

## 2) 錯誤修正任務：Signal HTTP 呼叫未完整處理錯誤且忽略 context

- **問題觀察**：`internal/singal/client.go.go` 中：
  - `fetchSignalMessages` 接收 `context.Context`，但使用 `http.Get`，未將 context 傳入 request。
  - `SendNotification` 忽略 `json.Marshal` 錯誤（`jsonData, _ := ...`）。
- **風險/影響**：
  - 逾時/取消訊號無法中斷請求，可能導致阻塞。
  - 序列化失敗時靜默吞錯，除錯困難且可能送出錯誤 payload。
- **任務內容**：
  1. 改為 `http.NewRequestWithContext` + `http.DefaultClient.Do`。
  2. 完整回傳 `json.Marshal` 錯誤。
  3. 補上對非 2xx 狀態碼的回應 body 訊息（利於除錯）。
- **完成定義（DoD）**：
  - 所有 HTTP 請求可被 context 取消。
  - 不再存在被忽略的序列化錯誤。

## 3) 文件差異修正任務：清理 README 中殘留 diff 標記

- **問題觀察**：`README.md` 的 Signal/Gmail/影片轉檔段落前保留了 `+` diff 前綴。
- **風險/影響**：
  - 直接影響 README 呈現品質與專業度。
  - 讀者可能誤判文件為未完成合併狀態。
- **任務內容**：
  1. 移除該段落每行行首 `+` 字元。
  2. 檢查 Markdown 標題層級與程式碼區塊排版是否正常。
  3. 若有 CI 文件檢查，加入 markdown lint 流程。
- **完成定義（DoD）**：
  - README 在 GitHub 預覽渲染正確。
  - 無明顯 diff 殘留符號。

## 4) 測試改進任務：補強 Tool Hint 負向案例與邊界測試

- **問題觀察**：`systemtesting/01_tool_hint_test.go` 目前偏重正向匹配，對「多意圖衝突」與「誤觸發」覆蓋不足。
- **風險/影響**：
  - 關鍵字路由可能在多語句或混合意圖時誤選工具。
  - 回歸時較難及早發現提示注入邏輯退化。
- **任務內容**：
  1. 新增負向案例：例如「今天天氣如何」不應觸發 `manage_calendar`。
  2. 新增多意圖案例：同時出現「行事曆 + 郵件」時，驗證排序/優先權符合設計。
  3. 新增表格驅動測試，集中維護關鍵字與預期工具映射。
- **完成定義（DoD）**：
  - 新增測試可穩定重現並防止誤觸發。
  - 對核心路由規則有明確且可讀的測試名稱與案例。
