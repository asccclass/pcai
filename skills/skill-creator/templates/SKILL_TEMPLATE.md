---
# [必填] 技能唯一識別名稱 (snake_case)
name: {{SKILL_NAME}}

# [必填] 技能描述 — Agent 會根據此描述判斷何時使用這個技能
description: {{DESCRIPTION}}

# [選填] 執行指令 — 支援 {{param}} 參數佔位符
# 省略此欄位則為 context-only 技能（僅提供文件說明）
command: echo "TODO: 請替換為實際指令 {{param_name}}"

# [選填] 快取時間 — 相同參數的重複呼叫會直接回傳快取結果
# cache_duration: 3h

# [選填] Docker 映像 — 指定後技能會在容器中執行
# image: python:3.11-slim

# [選填] 參數選項列表 — 限制 Agent 可填入的值
# options:
#   param_name:
#     - value1
#     - value2

# [選填] 參數別名 — 非正式用語自動對應到正確值
# option_aliases:
#   param_name:
#     "別名": "正式值"

# [選填] 依賴宣告
# metadata:
#   pcai:
#     requires:
#       bins: ["curl"]
#       env: ["API_KEY"]
---

# {{SKILL_NAME}}

{{DESCRIPTION}}

## Purpose

請說明「何時」應該使用這個 Skill。  
範例：當使用者需要查詢天氣時、當使用者要求讀取 Email 時。

## Steps

1. 第一步驟
2. 第二步驟
3. 第三步驟

## Output Format

請說明此技能的輸出格式。  
範例：JSON 格式、Markdown 格式、純文字。

## Examples

**使用者輸入：** "TODO"  
**Agent 動作：** 呼叫 `{{SKILL_NAME}}` 工具  
**預期輸出：** "TODO"
