#!/usr/bin/env bash
# validate.sh — 驗證指定 Skill 目錄是否符合 agentskills.io 規格
# 用法: bash validate.sh <skill_directory>

set -euo pipefail

SKILL_DIR="${1:?用法: bash validate.sh <skill_directory>}"
ERRORS=0
WARNINGS=0

echo "🔍 驗證 Skill: $SKILL_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 1. 檢查目錄是否存在
if [ ! -d "$SKILL_DIR" ]; then
    echo "❌ 目錄不存在: $SKILL_DIR"
    exit 1
fi

# 2. 檢查 SKILL.md 是否存在
SKILL_MD="$SKILL_DIR/SKILL.md"
if [ ! -f "$SKILL_MD" ]; then
    echo "❌ [必要] 缺少 SKILL.md"
    ERRORS=$((ERRORS + 1))
else
    echo "✅ SKILL.md 存在"

    # 3. 檢查 YAML Frontmatter
    if ! head -1 "$SKILL_MD" | grep -q "^---"; then
        echo "❌ [必要] SKILL.md 缺少 YAML Frontmatter (需以 --- 開頭)"
        ERRORS=$((ERRORS + 1))
    else
        echo "✅ YAML Frontmatter 格式正確"

        # 提取 frontmatter 內容
        FRONTMATTER=$(sed -n '/^---$/,/^---$/p' "$SKILL_MD" | head -n -1 | tail -n +2)

        # 4. 檢查 name 欄位
        if echo "$FRONTMATTER" | grep -q "^name:"; then
            NAME=$(echo "$FRONTMATTER" | grep "^name:" | head -1 | sed 's/^name:[[:space:]]*//')
            echo "✅ name: $NAME"
        else
            echo "❌ [必要] 缺少 name 欄位"
            ERRORS=$((ERRORS + 1))
        fi

        # 5. 檢查 description 欄位
        if echo "$FRONTMATTER" | grep -q "^description:"; then
            echo "✅ description 欄位已填寫"
        else
            echo "❌ [必要] 缺少 description 欄位"
            ERRORS=$((ERRORS + 1))
        fi

        # 6. 檢查 command 欄位 (選填)
        if echo "$FRONTMATTER" | grep -q "^command:"; then
            COMMAND=$(echo "$FRONTMATTER" | grep "^command:" | head -1 | sed 's/^command:[[:space:]]*//')
            echo "✅ command: $COMMAND"

            # 檢查參數格式
            PARAMS=$(echo "$COMMAND" | grep -oP '\{\{[^}]+\}\}' || true)
            if [ -n "$PARAMS" ]; then
                echo "   📋 偵測到參數: $PARAMS"
            fi
        else
            echo "ℹ️  無 command 欄位 (context-only 技能)"
        fi
    fi
fi

# 7. 檢查選填目錄
for SUBDIR in scripts templates references; do
    if [ -d "$SKILL_DIR/$SUBDIR" ]; then
        FILE_COUNT=$(find "$SKILL_DIR/$SUBDIR" -type f 2>/dev/null | wc -l)
        echo "✅ $SUBDIR/ 目錄存在 ($FILE_COUNT 個檔案)"
    else
        echo "ℹ️  無 $SUBDIR/ 目錄 (選填)"
    fi
done

# 結果摘要
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ $ERRORS -eq 0 ]; then
    echo "✅ 驗證通過！Skill 符合 agentskills.io 規格。"
else
    echo "❌ 驗證失敗：發現 $ERRORS 個錯誤。"
    exit 1
fi
