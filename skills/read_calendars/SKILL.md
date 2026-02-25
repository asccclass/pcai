---
name: read_calendars
description: 讀取所有存取權限內的 Google 行事曆（含主要、自建、他人分享）。此工具會先列出所有行事曆及其權限類別（如 owner, reader），再抓取指定時間範圍內的所有事件，供 LLM 進行整合分析。
command: |
  echo "--- CALENDAR LIST ---" && \
  gog calendar calendars --json && \
  echo "--- ALL EVENTS ---" && \
  gog calendar events --all --from {{from}} --to {{to}} --json
options:
  from:
    description: "開始時間 (例如: today 或 2026-02-26)"
    default: "today"
  to:
    description: "結束時間 (例如: today 或 2026-02-28)"
    default: "today"

---
name: check_availability_all
description: 檢查所有行事曆的空閒/忙碌狀態 (Free/Busy) 以偵測潛在衝突。此工具會先獲取行事曆清單，並同時對所有行事曆發出可用性查詢。
command: |
  echo "--- CALENDAR LIST ---" && \
  gog calendar calendars --json && \
  echo "--- GLOBAL AVAILABILITY ---" && \
  gog calendar freebusy --all --from {{from}} --to {{to}} --json
options:
  from:
    description: "查詢開始時間 (RFC3339 或 relative)"
    default: "today"
  to:
    description: "查詢結束時間 (RFC3339 或 relative)"
    default: "today"

---
name: create_event_smart
description: 智慧新增行程。LLM 會先讀取行事曆清單以判斷最適合的 calendar_id（例如：工作相關行程選擇工作行事曆），再新增行程。
command: |
  echo "--- AVAILABLE CALENDARS ---" && \
  gog calendar calendars --json && \
  echo "--- CREATING EVENT ---" && \
  gog calendar create {{calendar_id}} --summary "{{summary}}" --from {{from}} --to {{to}} --json
options:
  calendar_id:
    description: "由 LLM 從行事曆清單中選出的 ID (例如: primary 或 email 地址)"
  summary:
    description: "行程標題"
  from:
    description: "行程開始時間"
  to:
    description: "行程結束時間"
---

# Google Calendar (gog) Skill - 全域智慧版

## 技能概覽
本技能解決了 `gog` 預設僅操作主要行事曆的限制。透過自動化指令組合，確保 LLM 在每次操作前都能獲取最新的行事曆清單，實現「智慧選取」與「全域讀取」。

## 核心功能

### 1. 全域讀取與衝突檢查
- **read_all_calendar_data**: 同時獲取行事曆屬性與具體事件。
- **check_availability_all**: 跨行事曆檢查忙碌區間，預設查詢當天狀態。

### 2. 智慧創建事件 (`create_event_smart`)
- **自動導航**：在執行新增動作前，工具會強制輸出 `CALENDAR LIST`。LLM 必須對照 `summary` 與 `accessRole` 來決定目標行事曆。
- **寫入權限檢查**：LLM 應避開 `accessRole: reader` 的行事曆（僅唯讀），並優先選擇 `owner` 或 `writer` 權限的行事曆進行寫入。
- **預設與彈性**：若使用者未指定行事曆，LLM 應根據行程內容判斷；若內容模糊，則預設寫入 `primary`。

---

## 指令參考

### 智慧新增行程範例
```bash
# LLM 將依據情境自動填入參數
gog calendar create "work_email@gmail.com" --summary "專案週報" --from "2026-02-26T14:00:00Z" --to "2026-02-26T15:00:00Z" --json