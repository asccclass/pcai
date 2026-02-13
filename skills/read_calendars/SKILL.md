---
name: read_calendars
description: BEST tool for reading/checking user's daily schedule or calendar events from ALL accounts.
command: bin\gog.exe calendar events --all --from {{from}} --to {{to}} --json
metadata:
  pcai:
    requires:
      bins: ["gog"]
      env: ["GOG_PATH"]
---

# Calendar Reader Skill
此技能允許 Agent 透過 `gog` CLI 工具讀取使用者的 Google 行事曆。

## Capabilities
1.  **Read Events**: 讀取所有行事曆的活動 (包含私人、工作、訂閱)。
2.  **Flexible Date Ranges**: 支援「今天」、「本週」、「下週」或特定日期的查詢。

## Instructions for the Agent
當使用者要求讀取行事曆時，請遵循以下步驟：

1.  **解析時間範圍 (Required)**：
    - **必須** 計算並提供 `from` 和 `to` 參數 (Format: YYYY-MM-DD)。
    - 若使用者未指定，預設查詢 **和今天 (Today)**。
    - 範例:
        - "今天": `from`="2026-02-13", `to`="2026-02-13" (假設今天是 13 號)
        - "未來 7 天": `from`="2026-02-13", `to`="2026-02-20"

2.  **執行查詢**：
    - 工具會自動執行: `bin\gog.exe calendar list --all --from {{from}} --to {{to}}`
    - 請確保 `from` 和 `to` 參數已正確填入。

3.  **摘要內容**：
    - 整理並列出重點行程，包含時間、活動名稱、地點。
    - 若無活動，請明確告知使用者。

## Examples
User: "看看我這禮拜有什麼行程"
Agent Action:
1. Dates: Today to Sunday.
2. Run: `gog calendar list --all --from 2026-02-12 --to 2026-02-15`

User: "列出今日的行程"
Agent Action:
1. Dates: Today.
2. Run: `gog calendar list --all --from 2026-02-12 --to 2026-02-12`