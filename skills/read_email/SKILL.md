---
name: read_email
description: 讀取使用者的 Gmail 郵件。可以搜尋特定寄件者、主旨或未讀郵件。
command: internal_tool

---

# 讀取 Email (read_email)

這是一個內建工具，允許 AI 代理程式存取使用者的 Gmail 信箱。

## 參數
- `query`: Gmail 搜尋語法。
  - `is:unread`: 未讀郵件
  - `from:example@test.com`: 來自特定寄件者
  - `subject:訂單`: 主旨包含「訂單」
- `max_results`: 限制回傳的郵件數量。

## 用法
直接呼叫工具 `read_email` 並帶入參數。
