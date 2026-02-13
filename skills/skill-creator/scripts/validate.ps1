# validate.ps1 - Validate a Skill directory against agentskills.io spec (Windows PowerShell)
# Usage: powershell -ExecutionPolicy Bypass -File validate.ps1 <skill_directory>

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$SkillDir
)

$ErrorActionPreference = "Continue"
$Errors = 0

Write-Host "[VALIDATE] Skill: $SkillDir" -ForegroundColor Cyan
Write-Host "================================"

# 1. Check directory exists
if (-not (Test-Path $SkillDir -PathType Container)) {
    Write-Host "[FAIL] Directory not found: $SkillDir" -ForegroundColor Red
    exit 1
}

# 2. Check SKILL.md exists
$SkillMd = Join-Path $SkillDir "SKILL.md"
if (-not (Test-Path $SkillMd)) {
    Write-Host "[FAIL] Missing SKILL.md" -ForegroundColor Red
    $Errors++
}
else {
    Write-Host "[PASS] SKILL.md exists" -ForegroundColor Green

    $lines = Get-Content $SkillMd -Encoding UTF8

    # 3. Check YAML frontmatter
    if ($lines[0] -ne "---") {
        Write-Host "[FAIL] SKILL.md missing YAML frontmatter (must start with ---)" -ForegroundColor Red
        $Errors++
    }
    else {
        Write-Host "[PASS] YAML frontmatter format OK" -ForegroundColor Green

        # Extract frontmatter lines
        $frontmatterLines = @()
        $frontmatterEnd = $false
        foreach ($line in $lines[1..($lines.Length - 1)]) {
            if ($line -eq "---" -and -not $frontmatterEnd) {
                $frontmatterEnd = $true
                break
            }
            $frontmatterLines += $line
        }
        $frontmatter = $frontmatterLines -join "`n"

        # 4. Check name field
        if ($frontmatter -match '(?m)^name:\s*(.+)') {
            $name = $Matches[1].Trim()
            Write-Host "[PASS] name: $name" -ForegroundColor Green
        }
        else {
            Write-Host "[FAIL] Missing required field: name" -ForegroundColor Red
            $Errors++
        }

        # 5. Check description field
        if ($frontmatter -match '(?m)^description:') {
            Write-Host "[PASS] description field present" -ForegroundColor Green
        }
        else {
            Write-Host "[FAIL] Missing required field: description" -ForegroundColor Red
            $Errors++
        }

        # 6. Check command field (optional)
        if ($frontmatter -match '(?m)^command:\s*(.+)') {
            $command = $Matches[1].Trim()
            Write-Host "[PASS] command: $command" -ForegroundColor Green

            # Check parameter format
            $params = [regex]::Matches($command, '\{\{[^}]+\}\}')
            if ($params.Count -gt 0) {
                $paramList = ($params | ForEach-Object { $_.Value }) -join ", "
                Write-Host "  -> Params detected: $paramList" -ForegroundColor Gray
            }
        }
        else {
            Write-Host "[INFO] No command field - context-only skill" -ForegroundColor Gray
        }
    }
}

# 7. Check optional directories
foreach ($subdir in @("scripts", "templates", "references")) {
    $subdirPath = Join-Path $SkillDir $subdir
    if (Test-Path $subdirPath -PathType Container) {
        $files = Get-ChildItem $subdirPath -File -Recurse -ErrorAction SilentlyContinue
        $fileCount = 0
        if ($files) { $fileCount = $files.Count }
        Write-Host "[PASS] $subdir/ exists - $fileCount file(s)" -ForegroundColor Green
    }
    else {
        Write-Host "[INFO] No $subdir/ directory - optional" -ForegroundColor Gray
    }
}

# Summary
Write-Host ""
Write-Host "================================"
if ($Errors -eq 0) {
    Write-Host "[RESULT] PASSED - Skill conforms to agentskills.io spec." -ForegroundColor Green
}
else {
    Write-Host "[RESULT] FAILED - Found $Errors error(s)." -ForegroundColor Red
    exit 1
}
