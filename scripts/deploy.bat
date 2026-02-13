@echo off
setlocal

if "%1"=="" (
    echo Usage: deploy.bat user@host
    echo Example: deploy.bat root@192.168.1.100
    exit /b 1
)

set VPS_HOST=%1

echo === TgBot Deployment ===

echo.
echo [1/3] Building Linux binary...
call "%~dp0build-linux.bat"
if errorlevel 1 (
    echo Build failed
    exit /b 1
)

echo.
echo [2/3] Copying to VPS...
scp tgbot %VPS_HOST%:~/
if errorlevel 1 (
    echo Failed to copy binary
    exit /b 1
)

echo.
echo [3/3] Installing on VPS...
ssh %VPS_HOST% "sudo systemctl stop tgbot 2>/dev/null; sudo mkdir -p /opt/tgbot; sudo mv ~/tgbot /opt/tgbot/; sudo chmod +x /opt/tgbot/tgbot; sudo chown tgbot:tgbot /opt/tgbot/tgbot 2>/dev/null; sudo systemctl start tgbot 2>/dev/null && echo 'Service restarted' || echo 'Service not configured'"

echo.
echo Done!
echo Check logs: ssh %VPS_HOST% "sudo journalctl -u tgbot -f"
