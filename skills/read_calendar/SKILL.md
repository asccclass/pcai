---
name: read_calendar
description: 讀取或列出 Google Calendar 行事曆活動。支援一次讀取多個行事曆。底層使用 `gog` CLI。
command: internal_tool
options:
  calendars:
    - primary
    - "example@gmail.com"
    - "id1,id2"
  max_results:
    - 10
    - 20
---

# 讀取行事曆 (read_calendar)

這是一個內建工具，允許 AI 代理程式存取使用者的 Google Calendar。底層使用 `gog` CLI (Google CLI) 進行資料存取，自動查詢從指定時間（若未指定則為現在）開始的**未來 7 天**。

## 參數描述
- `calendars`: (string, required) 要讀取的行事曆 ID (Email 格式)。
    - 若要讀取預設行事曆，可不填或填 `primary`。
    - 若使用者指定了特定行事曆 (例如「讀取 Andy 的行事曆」或「讀取 work@company.com」)，必須填入對應的 Email ID。
    - 支援讀取多個行事曆，請使用**逗號分隔的字串** (如 `"primary,work@example.com"`)。
    - ❌ 禁止使用 JSON Array (如 `["id1", "id2"]`)。
- `max_results`: (integer, optional) 每個行事曆要讀取的最大事件數量 (預設 10)。注意：即便設定了，查詢時間範圍仍固定為 7 天。

## 呼叫範例
read_calendar {"calendars": "primary"}
read_calendar {"calendars": "liuchengood@gmail.com"}
read_calendar {"calendars": "primary,liuchengood@gmail.com"}

## 回傳格式說明
回傳為純文字格式，包含該行事曆**目前時段及未來 7 天**的活動資訊：
- `ID`: 行事曆 ID
- `Summary`: 活動標題
- `Start`: 活動開始時間
- `End`: 活動結束時間
- `Location`: 地點
- `Status`: 狀態 (confirmed, tentative, cancelled)
- `HtmlLink`: 網頁連結
