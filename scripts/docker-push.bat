@echo off
setlocal

if "%1"=="" (
    echo Usage: docker-push.bat username [tag]
    echo Example: docker-push.bat myusername latest
    exit /b 1
)

set USERNAME=%1
set TAG=%2
if "%TAG%"=="" set TAG=latest

set IMAGE=%USERNAME%/tgbot:%TAG%

echo === Docker Build and Push ===
echo Image: %IMAGE%

echo.
echo [1/2] Building image...
docker build -t %IMAGE% .
if errorlevel 1 (
    echo Build failed
    exit /b 1
)

echo.
echo [2/2] Pushing to registry...
docker push %IMAGE%
if errorlevel 1 (
    echo Push failed. Run 'docker login' first.
    exit /b 1
)

echo.
echo Done!
echo.
echo On VPS run:
echo docker pull %IMAGE% ^&^& docker rm -f tgbot ^&^& docker run -d --name tgbot --restart unless-stopped --env-file ~/tgbot/.env %IMAGE%
