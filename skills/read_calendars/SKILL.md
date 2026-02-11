---
name: fetch-all-calendars
description: 取得使用者所有 Google 行事曆分類的詳細活動內容 (包含私人、工作及訂閱的行事曆)
metadata:
  author: User
  version: 1.0
  requirements:
    - bin/gog
---

# Fetch All Calendars Skill

這個 Skill 用於讀取使用者所有的 Google 行事曆資料。
它會先列出所有可用的行事曆 ID，然後針對每一個 ID 抓取指定時間範圍內的活動。

## 使用方法 (Usage)

當使用者詢問「取得我所有行事曆的行程」或「檢查我所有行事曆」時，請執行以下步驟。

### 執行邏輯

請使用 `bash` 執行以下腳本。這個腳本會自動處理 ID 的列表與遍歷。

```bash
#!/bin/bash

# 1. 設定時間範圍 (預設未來 30 天，可根據需求調整)
START_DATE=$(date +%Y-%m-%d)
END_DATE=$(date -d "+7 days" +%Y-%m-%d)

echo "正在取得行事曆列表..."

# 2. 取得所有行事曆列表 (JSON 格式)
# 使用 jq 解析出 id 欄位 (假設 gog 回傳標準 JSON)
CAL_IDS=$(bin/gog calendar list --json | grep -o '"id": *"[^"]*"' | cut -d'"' -f4)

# 如果沒有 jq 或無法解析，這是備案：讓 Agent 根據實際輸出調整解析方式
if [ -z "$CAL_IDS" ]; then
    echo "無法自動解析 ID，嘗試直接顯示列表："
    gog calendar list
    exit 1
fi

echo "找到以下行事曆，開始讀取詳細內容..."
echo "$CAL_IDS"

# 3. 遍歷每個 ID 讀取事件
for id in $CAL_IDS; do
    echo "--------------------------------------------------"
    echo "正在讀取行事曆: $id"
    # 呼叫 gog 讀取該 ID 的事件
    # 注意：參數可能需要根據實際 gog 版本微調 (例如 --calendar 或 --id)
    bin/gog calendar events --calendar "$id" --from "$START_DATE" --to "$END_DATE" --json
done