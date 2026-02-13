# Skill-First 執行優先權

## 概述

修改 `core.Registry` 支援優先級，讓 Skills 優先於 Tools 被 LLM 選用。

## 相關檔案

| 檔案 | 類型 | 說明 |
|------|------|------|
| `internal/core/definition.go` | 改寫 | Registry 支援 `RegisterWithPriority()` |
| `tools/init.go` | 修改 | Skills 以 priority 10 註冊 |
| `tools/skill_manager.go` | 修改 | 恢復的 Skills 也用 priority 10 |

## 優先級設計

| 類型 | Priority | 效果 |
|------|----------|------|
| Dynamic Skills | 10 | `GetDefinitions()` 排在前面 |
| Advisor Skill | 10 | `GetToolPrompt()` 標註 `[優先使用]` |
| Built-in Tools | 0 | 預設優先級，排在後面 |

## Registry API

```go
// 預設優先級 (0) 註冊
registry.Register(tool)

// 指定優先級註冊（數字越大越優先）
registry.RegisterWithPriority(tool, 10)
```

## 運作原理

1. `GetDefinitions()` 依 priority 降序排列工具定義
2. LLM 收到的工具清單中 Skills 排在最前面
3. `GetToolPrompt()` 為 priority > 0 的工具標註 `[優先使用]`
4. LLM 傾向選擇清單前面且有優先標記的工具
