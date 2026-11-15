# LearnChain dApp - Automated Build Script (Option B: Pure Go / Zero-CGO)
$ErrorActionPreference = "Stop"

# Ensure current working directory is the script's directory
if ($PSScriptRoot) {
    Set-Location -Path $PSScriptRoot
}

# Ensure Go is in PATH
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    if (Test-Path "C:\Program Files\Go\bin\go.exe") {
        $env:PATH += ";C:\Program Files\Go\bin"
    }
}

Write-Host "========================================================" -ForegroundColor Cyan
Write-Host "   LearnChain SBT dApp - Builder (Option B: Zero-CGO)  " -ForegroundColor Cyan
Write-Host "========================================================" -ForegroundColor Cyan
Write-Host ""

$env:CGO_ENABLED = "0"

# Determine build version metadata
$AppVersion = if ($env:APP_VERSION) { $env:APP_VERSION } else { "v1.0.0-local" }
$GitCommit = "local"
if (Get-Command git -ErrorAction SilentlyContinue) {
    $tag = git describe --tags --exact-match 2>$null
    if ($tag) { $AppVersion = $tag }
    $sha = git rev-parse --short HEAD 2>$null
    if ($sha) { $GitCommit = $sha }
}
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$LdFlags = "-H=windowsgui -s -w -X elearning-sbt/version.Version=$AppVersion -X elearning-sbt/version.Commit=$GitCommit -X elearning-sbt/version.BuildDate=$BuildDate"

Write-Host "📦 [1/2] Fetching modules and dependencies (go mod tidy)..." -ForegroundColor Yellow
go mod tidy

Write-Host "⚡ [2/2] Compiling standalone executable (LearnChain.exe $AppVersion)..." -ForegroundColor Yellow
go build -ldflags="$LdFlags" -o LearnChain.exe .

Write-Host ""
Write-Host "========================================================" -ForegroundColor Green
Write-Host "✅ [SUCCESS] Compilation completed: LearnChain.exe ($AppVersion)" -ForegroundColor Green
Write-Host "🚀 Launching application at http://localhost:8888 ..." -ForegroundColor Green
Write-Host "========================================================" -ForegroundColor Green
Write-Host ""

.\LearnChain.exe
