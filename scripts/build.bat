@echo off
setlocal enabledelayedexpansion

echo Building for Windows...

:: Get git info
for /f "tokens=*" %%i in ('git rev-parse HEAD 2^>nul') do set GIT_COMMIT=%%i
if "%GIT_COMMIT%"=="" set GIT_COMMIT=unknown

:: Get commit date in ISO format without spaces
for /f "tokens=*" %%i in ('git log -1 --format^=%%cd --date^=short 2^>nul') do set GIT_DATE=%%i
if "%GIT_DATE%"=="" set GIT_DATE=unknown

for /f "tokens=*" %%i in ('git rev-parse --abbrev-ref HEAD 2^>nul') do set GIT_BRANCH=%%i
if "%GIT_BRANCH%"=="" set GIT_BRANCH=unknown

:: Get build date without problematic characters
for /f "tokens=*" %%i in ('powershell -command "Get-Date -Format yyyy-MM-dd"') do set BUILD_DATE=%%i

echo   Commit: %GIT_COMMIT%
echo   Date:   %GIT_DATE%
echo   Branch: %GIT_BRANCH%

:: Build with proper quoting
set PKG=github.com/user/tgbot/internal/version

go build -ldflags="-s -w -X %PKG%.GitCommit=%GIT_COMMIT% -X %PKG%.GitDate=%GIT_DATE% -X %PKG%.GitBranch=%GIT_BRANCH% -X %PKG%.BuildDate=%BUILD_DATE%" -o bot.exe ./cmd/bot

if exist bot.exe (
    echo Build successful: bot.exe
) else (
    echo Build failed
    exit /b 1
)
