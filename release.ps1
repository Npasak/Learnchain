param(
    [string]$Version
)

$ErrorActionPreference = "Stop"

if ($PSScriptRoot) {
    Set-Location -Path $PSScriptRoot
}

if (-not $Version) {
    $Version = Read-Host "กรอกเลขเวอร์ชันที่ต้องการออก (เช่น v1.0.1)"
}

if (-not $Version.StartsWith("v")) {
    $Version = "v$Version"
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    if (Test-Path "C:\Program Files\Go\bin\go.exe") {
        $env:PATH += ";C:\Program Files\Go\bin"
    }
}

$env:CGO_ENABLED = "0"
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Commit = "local"

if ((Test-Path ".git") -and (Get-Command git -ErrorAction SilentlyContinue)) {
    $sha = git rev-parse --short HEAD 2>$null
    if ($sha) { $Commit = $sha }
}

Write-Host "========================================================" -ForegroundColor Cyan
Write-Host "  LearnChain Release Builder ($Version)" -ForegroundColor Cyan
Write-Host "========================================================" -ForegroundColor Cyan

$LdFlags = "-s -w -X elearning-sbt/version.Version=$Version -X elearning-sbt/version.Commit=$Commit -X elearning-sbt/version.BuildDate=$BuildDate"
go build -ldflags="$LdFlags" -o LearnChain.exe .

$hash = (Get-FileHash -Path LearnChain.exe -Algorithm SHA256).Hash.ToLower()
"$hash  LearnChain.exe" | Out-File -FilePath checksums.txt -Encoding ascii

Write-Host "✅ Build สำเร็จ: LearnChain.exe ($Version)" -ForegroundColor Green
Write-Host "🔐 SHA-256: $hash (บันทึกใน checksums.txt แล้ว)" -ForegroundColor Green

if ((Test-Path ".git") -and (Get-Command git -ErrorAction SilentlyContinue)) {
    $pushNow = Read-Host "พบ Git repository — ต้องการสร้าง tag $Version และ push ขึ้น GitHub เลยหรือไม่? (y/N)"
    if ($pushNow -eq "y" -or $pushNow -eq "Y") {
        git tag $Version
        git push origin $Version
        Write-Host "🚀 Push tag $Version สำเร็จ! GitHub Actions จะสร้าง Release ให้อัตโนมัติ" -ForegroundColor Green
    }
} else {
    Write-Host "ℹ️ ยังไม่ได้เชื่อมต่อ Git: สามารถอัปโหลด LearnChain.exe และ checksums.txt ขึ้น GitHub Releases ได้เลย" -ForegroundColor Yellow
}
