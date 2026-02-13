#!/usr/bin/env bash
# scaffold.sh — 自動建立新 Skill 骨架目錄
# 用法: bash scaffold.sh <skill_name> [description]

set -euo pipefail

SKILL_NAME="${1:?用法: bash scaffold.sh <skill_name> [description]}"
DESCRIPTION="${2:-請在此填寫技能描述}"

# 取得 skills 根目錄（此腳本位於 skill-creator/scripts/）
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SKILLS_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TARGET_DIR="$SKILLS_ROOT/$SKILL_NAME"

# 檢查目錄是否已存在
if [ -d "$TARGET_DIR" ]; then
    echo "❌ 錯誤: 目錄已存在: $TARGET_DIR"
    exit 1
fi

# 建立目錄結構
echo "📁 建立 Skill 骨架: $TARGET_DIR"
mkdir -p "$TARGET_DIR/scripts"
mkdir -p "$TARGET_DIR/templates"
mkdir -p "$TARGET_DIR/references"

# 從範本產生 SKILL.md
TEMPLATE_PATH="$SCRIPT_DIR/../templates/SKILL_TEMPLATE.md"
if [ -f "$TEMPLATE_PATH" ]; then
    sed -e "s/{{SKILL_NAME}}/$SKILL_NAME/g" \
        -e "s/{{DESCRIPTION}}/$DESCRIPTION/g" \
        "$TEMPLATE_PATH" > "$TARGET_DIR/SKILL.md"
else
    # 若範本不存在，直接寫入最小結構
    cat > "$TARGET_DIR/SKILL.md" << EOF
---
name: $SKILL_NAME
description: $DESCRIPTION
command: echo "TODO: 請替換為實際指令"
---

# $SKILL_NAME

$DESCRIPTION

## Purpose
請說明何時應該使用這個技能。

## Steps
1. TODO

## Output Format
請說明輸出格式。

## Examples
TODO
EOF
fi

echo "✅ Skill 骨架建立完成！"
echo ""
echo "   $TARGET_DIR/"
echo "   ├── SKILL.md"
echo "   ├── scripts/"
echo "   ├── templates/"
echo "   └── references/"
echo ""
echo "📝 請編輯 $TARGET_DIR/SKILL.md 完成技能定義。"
