# Build and push Docker image to registry
# Usage: .\scripts\docker-push.ps1 -Username your-dockerhub-username [-Tag latest]

param(
    [Parameter(Mandatory=$true)]
    [string]$Username,
    
    [string]$Tag = "latest",
    
    [string]$Registry = ""  # Empty for Docker Hub, or "ghcr.io" for GitHub
)

$ErrorActionPreference = "Stop"

$imageName = if ($Registry) {
    "$Registry/$Username/tgbot:$Tag"
} else {
    "$Username/tgbot:$Tag"
}

Write-Host "=== Docker Build & Push ===" -ForegroundColor Cyan
Write-Host "Image: $imageName" -ForegroundColor Gray

# Build
Write-Host "`n[1/2] Building image..." -ForegroundColor Yellow
docker build -t $imageName .

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed" -ForegroundColor Red
    exit 1
}

# Push
Write-Host "`n[2/2] Pushing to registry..." -ForegroundColor Yellow
docker push $imageName

if ($LASTEXITCODE -ne 0) {
    Write-Host "Push failed. Run 'docker login' first." -ForegroundColor Red
    exit 1
}

Write-Host "`nDone!" -ForegroundColor Green
Write-Host "`nOn VPS run:"
Write-Host "docker pull $imageName && docker rm -f tgbot && docker run -d --name tgbot --restart unless-stopped --env-file ~/tgbot/.env $imageName" -ForegroundColor Cyan
