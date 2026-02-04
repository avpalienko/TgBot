# Deploy bot to VPS
# Usage: .\scripts\deploy.ps1 -Host user@your-vps-ip

param(
    [Parameter(Mandatory=$true)]
    [string]$VpsHost
)

$ErrorActionPreference = "Stop"

Write-Host "=== TgBot Deployment ===" -ForegroundColor Cyan

# Step 1: Build
Write-Host "`n[1/3] Building Linux binary..." -ForegroundColor Yellow
& "$PSScriptRoot\build-linux.ps1"

# Step 2: Copy
Write-Host "`n[2/3] Copying to VPS..." -ForegroundColor Yellow
scp tgbot "${VpsHost}:~/"

if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to copy binary" -ForegroundColor Red
    exit 1
}

# Step 3: Install
Write-Host "`n[3/3] Installing on VPS..." -ForegroundColor Yellow
$installScript = @'
sudo systemctl stop tgbot 2>/dev/null || true
sudo mkdir -p /opt/tgbot
sudo mv ~/tgbot /opt/tgbot/
sudo chmod +x /opt/tgbot/tgbot
sudo chown tgbot:tgbot /opt/tgbot/tgbot 2>/dev/null || sudo chown root:root /opt/tgbot/tgbot
sudo systemctl start tgbot 2>/dev/null && echo "Service restarted" || echo "Service not configured yet"
'@

ssh $VpsHost $installScript

Write-Host "`nDone!" -ForegroundColor Green
Write-Host "Check logs: ssh $VpsHost 'sudo journalctl -u tgbot -f'"
