# Build Linux binary from Windows
# Usage: .\scripts\build-linux.ps1

$ErrorActionPreference = "Stop"

Write-Host "Building for Linux amd64..." -ForegroundColor Cyan

$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

try {
    go build -ldflags="-s -w" -o tgbot ./cmd/bot
    
    if (Test-Path "tgbot") {
        $size = (Get-Item "tgbot").Length / 1MB
        Write-Host "Build successful: tgbot ($([math]::Round($size, 2)) MB)" -ForegroundColor Green
    }
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
