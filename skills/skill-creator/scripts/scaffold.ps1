# scaffold.ps1 — 自動建立新 Skill 骨架目錄 (Windows PowerShell)
# 用法: powershell -File scaffold.ps1 <skill_name> [description]

param(
    [Parameter(Mandatory=$true, Position=0)]
    [string]$SkillName,

    [Parameter(Position=1)]
    [string]$Description = "請在此填寫技能描述"
)

$ErrorActionPreference = "Stop"

# 取得 skills 根目錄
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$SkillsRoot = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path
$TargetDir = Join-Path $SkillsRoot $SkillName

# 檢查目錄是否已存在
if (Test-Path $TargetDir) {
    Write-Host "❌ 錯誤: 目錄已存在: $TargetDir" -ForegroundColor Red
    exit 1
}

# 建立目錄結構
Write-Host "📁 建立 Skill 骨架: $TargetDir" -ForegroundColor Cyan
New-Item -ItemType Directory -Path (Join-Path $TargetDir "scripts") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $TargetDir "templates") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $TargetDir "references") -Force | Out-Null

# 從範本產生 SKILL.md
$TemplatePath = Join-Path $ScriptDir "..\templates\SKILL_TEMPLATE.md"
$SkillMdPath = Join-Path $TargetDir "SKILL.md"

if (Test-Path $TemplatePath) {
    $content = Get-Content $TemplatePath -Raw -Encoding UTF8
    $content = $content -replace '\{\{SKILL_NAME\}\}', $SkillName
    $content = $content -replace '\{\{DESCRIPTION\}\}', $Description
    Set-Content -Path $SkillMdPath -Value $content -Encoding UTF8
} else {
    # 若範本不存在，直接寫入最小結構
    $minimalContent = @"
---
name: $SkillName
description: $Description
command: echo "TODO: 請替換為實際指令"
---

# $SkillName

$Description

## Purpose
請說明何時應該使用這個技能。

## Steps
1. TODO

## Output Format
請說明輸出格式。

## Examples
TODO
"@
    Set-Content -Path $SkillMdPath -Value $minimalContent -Encoding UTF8
}

Write-Host "✅ Skill 骨架建立完成！" -ForegroundColor Green
Write-Host ""
Write-Host "   $TargetDir\"
Write-Host "   ├── SKILL.md"
Write-Host "   ├── scripts\"
Write-Host "   ├── templates\"
Write-Host "   └── references\"
Write-Host ""
Write-Host "📝 請編輯 $SkillMdPath 完成技能定義。" -ForegroundColor Yellow
