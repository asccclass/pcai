---
name: read_calendars
description: Read and list user's calendar events from Google Calendar using the gog tool.
metadata:
  pcai:
    requires:
      bins: ["gog"]
---

# Calendar Reader Skill

此技能允許 Agent 透過 `gog` CLI 工具讀取使用者的 Google 行事曆。

## Capabilities

1.  **List Events**: 列出特定時間範圍內的行事曆活動。
2.  **Flexible Date Ranges**: 支援「今天」、「本週」、「下週」或特定日期的查詢。

## Instructions for the Agent

當使用者要求讀取行事曆時，請遵循以下步驟：

1.  **解析時間範圍**：
    - 根據使用者的自然語言（例如「本週」、「今天」、「因為下禮拜要...」）計算出 `Start Date` 和 `End Date`。
    - 格式必須為 `YYYY-MM-DD`。
    - 常用對照：
        - **今天**: `Start` = Today, `End` = Today (or Today+1 for full coverage)
        - **本週**: `Start` = Today (or Monday/Sunday of this week), `End` = End of this week (Sunday/Saturday)
        - **未來 7 天**: `Start` = Today, `End` = Today + 7 days
    - 如果使用者未指定，預設查詢 **未來 7 天**。

2.  **執行查詢**：
    - 使用指令 `gog calendar events --all --from <START_DATE> --to <END_DATE> --json`。
    - `--all` 參數確保讀取所有行事曆（包含訂閱的）。
    - 務必使用 `--json` 以便程式解析，但在摘要給使用者時請轉換為自然語言。

3.  **摘要內容**：
    - 收到 JSON 回應後，請整理並列出重點行程。
    - 包含：時間 (Start Time)、活動名稱 (Summary)、地點 (Location, if any)。
    - 如果沒有活動，請明確告知使用者「這段時間沒有行程」。

## Examples

User: "看看我這禮拜有什麼行程"
(假設今天是 2026-02-12 星期四)
Agent Action:
1. Identify intent: Read calendar.
2. Calculate dates: "This week" -> From 2026-02-12 (Today) to 2026-02-15 (Sunday) OR 2026-02-14 (Saturday). Let's use Today to Sunday.
3. run `gog calendar events --all --from 2026-02-12 --to 2026-02-15 --json`

User: "檢查明天的行事曆"
(假設今天是 2026-02-12)
Agent Action:
1. Identify intent: Read calendar for tomorrow.
2. Calculate dates: Tomorrow is 2026-02-13.
3. run `gog calendar events --all --from 2026-02-13 --to 2026-02-13 --json` (Note: gog might require end date to be inclusive or +1 day depending on implementation, usually same day works for 'events on that day', or safely use +1 day 2026-02-14 to cover full 24h)
   *Code Hint*: `gog` 的 `--to` 通常是 inclusive 或者截止點。為了保險，查詢單日建議 `to` 設為同一天或隔天。

User: "取得所有行事曆"
Agent Action:
1. Default to next 7 days or ask clarification.
2. run `gog calendar events --all --from <TODAY> --to <TODAY+7> --json`