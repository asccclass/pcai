---
name: read_email
description: Read, search and summarize user's emails from Gmail using the gog tool.
metadata:
  pcai:
    requires:
      bins: ["gog"]
---

# Email Reader Skill
此技能允許 Agent 透過 `gog` CLI 工具讀取使用者的 Gmail。

## Capabilities

1.  **List Emails**: 列出最近的郵件。
2.  **Read Email**: 讀取特定郵件的詳細內容。
3.  **Search Emails**: 搜尋特定郵件。

## Instructions for the Agent

當使用者要求讀取 Email 時，請遵循以下步驟：

1.  **檢查狀態**：確認 `gog` 工具是否可用。
2.  **列出郵件**：
    - 使用指令 `gog gmail list --limit 10` 來獲取最近的 10 封郵件列表（包含 ID 和標題）。
    - **注意**：不要一次讀取「所有」內容，這會超出 Context Window (記憶體限制)。先列出標題讓使用者選，或只讀取最新的。
3.  **讀取內容**：
    - 如果使用者想看特定郵件，使用 `gog gmail get <message_id>`。
    - 讀取後，請幫使用者**摘要**內容，除非使用者要求顯示全文。
    - 如果執行指令返回 'Unauthorized' 或 'Token expired'，請提示使用者重新執行登入指令 (例如 gog auth login)。

## Examples

User: "看看我有什麼新信"
Agent Action: run `gog gmail list --limit 5`

User: "讀取關於 '發票' 的那封信"
Agent Action:
1. run `gog gmail search "發票"`
2. (Find ID)
3. run `gog gmail get <ID>`