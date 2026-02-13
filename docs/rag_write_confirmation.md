# RAG 寫入確認機制

## 概述

`memory_save` 工具不再直接寫入記憶庫，改為暫存到 `PendingStore`，等使用者確認後才永久寫入。

## 相關檔案

| 檔案 | 類型 | 說明 |
|------|------|------|
| `internal/memory/pending_store.go` | 新增 | 暫存機制，30 分鐘自動過期 |
| `tools/memory_save.go` | 改寫 | 暫存到 PendingStore，不直接寫入 |
| `tools/memory_confirm.go` | 新增 | `memory_confirm` 工具 (確認/拒絕) |
| `tools/init.go` | 修改 | 建立 PendingStore 並注入工具 |

## PendingStore API

| 方法 | 說明 |
|------|------|
| `Add(content, tags)` | 暫存記憶，回傳 pending ID |
| `Confirm(id)` | 取出並確認單筆 |
| `ConfirmAll()` | 確認所有待處理項目 |
| `Reject(id)` | 丟棄單筆 |
| `RejectAll()` | 丟棄所有待處理項目 |
| `List()` | 列出所有待確認項目 |
| `Count()` | 回傳待確認數量 |

## memory_confirm 工具操作

| Action | 說明 |
|--------|------|
| `confirm` | 確認單筆（需 `pending_id`） |
| `reject` | 拒絕單筆（需 `pending_id`） |
| `confirm_all` | 批次確認全部 |
| `reject_all` | 批次拒絕全部 |

## 互動流程

```
使用者: 請記住我喜歡喝咖啡
   ↓
AI 呼叫 memory_save("使用者喜歡喝咖啡")
   ↓
PendingStore.Add() → pending_123
   ↓
AI: 我準備記住「使用者喜歡喝咖啡」，確認嗎？
   ↓
使用者: 確認
   ↓
AI 呼叫 memory_confirm(action: "confirm", pending_id: "pending_123")
   ↓
PendingStore.Confirm() → Manager.Add() → 寫入 Vector DB + knowledge.md
   ↓
AI: 好的，已經記住了！
```

## 啟動載入

程式啟動時自動載入知識庫（無需額外設定）：

1. `memory.NewManager(jsonPath, embedder)` → 載入 `memory_store.json`
2. `SyncMemory(memManager, mdPath)` → 同步 `knowledge.md` 新內容到向量庫
