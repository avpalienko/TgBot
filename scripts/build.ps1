# Build binary for current platform with version info
# Usage: .\scripts\build.ps1 [-Output bot.exe]

param(
    [string]$Output = "bot.exe"
)

$ErrorActionPreference = "Stop"

Write-Host "Building for current platform..." -ForegroundColor Cyan

# Get git info
$gitCommit = git rev-parse HEAD 2>$null
if (-not $gitCommit) { $gitCommit = "unknown" }

$gitDate = git log -1 --format=%ci 2>$null
if (-not $gitDate) { $gitDate = "unknown" }

$gitBranch = git rev-parse --abbrev-ref HEAD 2>$null
if (-not $gitBranch) { $gitBranch = "unknown" }

$buildDate = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"

Write-Host "  Commit: $gitCommit" -ForegroundColor Gray
Write-Host "  Date:   $gitDate" -ForegroundColor Gray
Write-Host "  Branch: $gitBranch" -ForegroundColor Gray

# Build ldflags
$pkg = "github.com/user/tgbot/internal/version"
$ldflags = "-s -w -X '$pkg.GitCommit=$gitCommit' -X '$pkg.GitDate=$gitDate' -X '$pkg.GitBranch=$gitBranch' -X '$pkg.BuildDate=$buildDate'"

go build -ldflags $ldflags -o $Output ./cmd/bot

if (Test-Path $Output) {
    $size = (Get-Item $Output).Length / 1MB
    Write-Host "Build successful: $Output ($([math]::Round($size, 2)) MB)" -ForegroundColor Green
}
