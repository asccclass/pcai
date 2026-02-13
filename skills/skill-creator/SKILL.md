---
# ===== YAML Frontmatter（前置資料）=====
# 以下欄位由 PCAI 的 dynamic_tool.go 解析，用於自動註冊技能

# [必填] name: 技能唯一識別名稱，也是 Agent 呼叫時使用的工具名稱
#   - 建議使用 snake_case 格式（如 my_skill）
#   - 必須在所有已註冊技能中保持唯一
name: skill-creator

# [必填] description: 技能的功能描述
#   - Agent 會根據此描述判斷何時使用這個技能
#   - 建議寫清楚「做什麼」和「何時用」
description: >
  用於建立新 Skill 的標準化範本與工具。提供骨架產生器 (scaffold)
  與規格驗證器 (validate)，確保自訂 Skill 符合 agentskills.io 開源規格。

# [選填] command: 技能要執行的指令（支援 {{param}} 參數佔位符）
#   - 格式: 任意 shell 指令，參數用 {{param_name}} 標記
#   - 支援 {{func:param}} 格式（如 {{url:location}} 表示 URL 編碼）
#   - 若省略此欄位，則為 context-only 技能（僅提供文件說明，不可被直接呼叫）
# command: (本技能為 context-only，不需要 command)

# [選填] cache_duration: 快取結果的有效時間
#   - 格式: Go Duration（如 3h, 30m, 24h）
#   - 相同參數的重複呼叫會在快取期間內直接回傳上次結果
# cache_duration: 3h

# [選填] image: Docker 映像名稱（用於 Sidecar 模式執行技能）
#   - 若指定，技能會在 Docker 容器中執行，適合需要特殊依賴的情況
# image: python:3.11-slim

# [選填] options: 參數的預設選項列表
#   - 為 command 中的 {{param}} 提供允許值清單
#   - Agent 會從選項中選擇，而非自由填寫
# options:
#   param_name:
#     - value1
#     - value2

# [選填] option_aliases: 參數值的別名對照表
#   - 讓使用者輸入的非正式用語自動對應到正確值
# option_aliases:
#   param_name:
#     "別名": "正式值"

# [選填] metadata: 額外的元資料
#   - pcai.requires.bins: 此技能依賴的外部二進位檔案
#   - pcai.requires.env: 此技能依賴的環境變數
# metadata:
#   pcai:
#     requires:
#       bins: ["gog"]
#       env: ["GOG_PATH"]
---

# Skill Creator — 標準化技能建立範本

此技能本身不執行任何指令，而是作為建立新 Skill 的**參考範本**與**工具包**。

## Purpose

當需要建立新的 PCAI Skill 時，使用此範本確保：
1. 目錄結構符合 agentskills.io 開源規格
2. SKILL.md 格式正確（YAML Frontmatter + Markdown Body）
3. 所有必要欄位已填寫

## 目錄結構

一個符合規格的 Skill 目錄應包含：

```
my_skill/
├── SKILL.md              # [必要] 核心定義檔案（YAML Frontmatter + 說明文件）
├── scripts/              # [選填] 輔助執行腳本
├── templates/            # [選填] 輸出格式範本
└── references/           # [選填] 技術參考文件
```

## Steps

### 建立新 Skill

1. 複製 `templates/SKILL_TEMPLATE.md` 到新目錄
2. 填入 `name`、`description`、`command` 等欄位
3. 執行 `scripts/scaffold` 腳本自動建立骨架（或手動建立）
4. 執行 `scripts/validate` 腳本驗證是否符合規格

### 骨架產生器 (scaffold)

```bash
# Linux/macOS
bash skills/skill-creator/scripts/scaffold.sh my_skill "我的技能描述"

# Windows
powershell -File skills/skill-creator/scripts/scaffold.ps1 my_skill "我的技能描述"
```

### 規格驗證器 (validate)

```bash
# Linux/macOS
bash skills/skill-creator/scripts/validate.sh skills/my_skill

# Windows
powershell -File skills/skill-creator/scripts/validate.ps1 skills/my_skill
```

## Output Format

骨架產生器會建立以下結構：
```
skills/<skill_name>/
├── SKILL.md              # 已填入基本資訊的定義檔
├── scripts/              # 空目錄（供放置腳本）
├── templates/            # 空目錄（供放置範本）
└── references/           # 空目錄（供放置參考文件）
```

## Examples

### 最小可用 Skill（僅需 SKILL.md）

```yaml
---
name: hello_world
description: 回傳 Hello World 訊息
command: echo "Hello, World!"
---
# Hello World Skill
一個簡單的示範技能。
```

### 完整 Skill（含參數、選項、別名）

```yaml
---
name: get_taiwan_weather
description: 查詢台灣各縣市天氣預報
command: web_fetch "https://api.example.com?location={{url:location}}"
cache_duration: 3h
options:
  location:
    - 臺北市
    - 高雄市
option_aliases:
  location:
    "台北": "臺北市"
metadata:
  pcai:
    requires:
      bins: ["curl"]
---
# 天氣查詢技能
透過 API 取得天氣資料。
```
