# TgBot Deployment Guide

Step-by-step guide for building and deploying TgBot to Ubuntu VPS via SSH from Windows.

## Compatibility Notes

The bot now uses:

- `github.com/openai/openai-go`
- OpenAI `Responses API`
- hosted `image_generation` tool for image generation and editing

If you set `OPENAI_BASE_URL`, make sure the target provider supports the Responses API and hosted image-generation tool calls. Plain chat-compatible endpoints are not enough for the new image workflows.

## Deployment Options

| Option | Build Location | VPS Requirements | Best For |
|--------|---------------|------------------|----------|
| **A: Binary** | Windows | Nothing | Minimal footprint, no dependencies |
| **B: Docker Image** | Windows | Docker only | Isolation, easy updates |
| **C: Docker Build** | VPS | Docker + sources | Simple setup |
| **D: Systemd + Go** | VPS | Go + sources | Full control |

---

## Option A: Binary Only (Recommended)

Build on Windows, deploy only the compiled binary. No Go or Docker needed on VPS.

**Quick deploy with scripts:**
```powershell
# First time: full setup (see steps below)
# Updates: just run
.\scripts\deploy.ps1 -VpsHost user@your-vps-ip
```

### Step 1: Cross-Compile on Windows

Open PowerShell in project directory:

```powershell
cd D:\CMA\Work\GO\TgBot

# Set environment for Linux build
$env:GOOS = "linux"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

# Build static binary
go build -ldflags="-s -w" -o tgbot ./cmd/bot

# Reset environment
Remove-Item Env:GOOS
Remove-Item Env:GOARCH
Remove-Item Env:CGO_ENABLED
```

This creates `tgbot` file (~8 MB) — a static Linux binary.

Or use the script:
```powershell
.\scripts\build-linux.ps1
```

### Step 2: Copy Binary to VPS

```powershell
# Copy binary
scp tgbot user@your-vps-ip:~/

# Copy .env.example as reference
scp .env.example user@your-vps-ip:~/tgbot.env.example
```

### Step 3: Setup on VPS

```bash
# Connect to VPS
ssh user@your-vps-ip

# Create directory
sudo mkdir -p /opt/tgbot

# Move binary
sudo mv ~/tgbot /opt/tgbot/
sudo chmod +x /opt/tgbot/tgbot

# Create .env
sudo nano /opt/tgbot/.env
```

Add configuration:
```env
TELEGRAM_BOT_TOKEN=your_token_here
OPENAI_API_KEY=sk-your-key-here
OPENAI_MODEL=gpt-4o
ALLOWED_USERS=your_user_id
```

Save: `Ctrl+O`, `Enter`, `Ctrl+X`

### Step 4: Create Service User and Permissions

```bash
# Create service user
sudo useradd -r -s /bin/false tgbot

# Set ownership and permissions
sudo chown -R tgbot:tgbot /opt/tgbot
sudo chmod 600 /opt/tgbot/.env
```

### Step 5: Create Systemd Service

```bash
sudo nano /etc/systemd/system/tgbot.service
```

Add:
```ini
[Unit]
Description=Telegram AI Bot
After=network.target

[Service]
Type=simple
User=tgbot
Group=tgbot
WorkingDirectory=/opt/tgbot
EnvironmentFile=/opt/tgbot/.env
ExecStart=/opt/tgbot/tgbot
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Save and exit.

### Step 6: Enable and Start

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable autostart on boot
sudo systemctl enable tgbot

# Start service
sudo systemctl start tgbot

# Check status
sudo systemctl status tgbot

# View logs
sudo journalctl -u tgbot -f
```

### Updating (Option A)

On Windows:
```powershell
.\scripts\deploy.ps1 -VpsHost user@your-vps-ip
```

Or manually:
```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -ldflags="-s -w" -o tgbot ./cmd/bot
scp tgbot user@your-vps-ip:~/
```

On VPS:
```bash
sudo systemctl stop tgbot
sudo mv ~/tgbot /opt/tgbot/
sudo chmod +x /opt/tgbot/tgbot
sudo chown tgbot:tgbot /opt/tgbot/tgbot
sudo systemctl start tgbot
```

---

## Option B: Docker Image (Pre-built Container)

Build Docker image on Windows, push to GitHub Container Registry (ghcr.io), pull and run on VPS.
No source code or build tools on VPS — just Docker.

**Quick deploy with scripts:**
```powershell
# On Windows: build and push image to ghcr.io
.\scripts\docker-push.ps1 -Username YOUR_GITHUB_USERNAME -Registry ghcr.io
```
```bash
# On VPS: pull and (re)start container
./deploy-docker.sh YOUR_GITHUB_USERNAME
```

### Prerequisites

- Docker Desktop installed on Windows
- GitHub account (ghcr.io is free for public repos)

### Step 1: Login to ghcr.io (one time)

On Windows:
```powershell
# Create a Personal Access Token at https://github.com/settings/tokens
# with "write:packages" scope, then login:
docker login ghcr.io -u YOUR_GITHUB_USERNAME
```

On VPS (only needed for private repos):
```bash
docker login ghcr.io -u YOUR_GITHUB_USERNAME
```

### Step 2: Build and Push Image

```powershell
cd D:\CMA\Work\GO\TgBot

# Build image
docker build -t ghcr.io/YOUR_GITHUB_USERNAME/tgbot:latest .

# Push to ghcr.io
docker push ghcr.io/YOUR_GITHUB_USERNAME/tgbot:latest
```

Or use the script:
```powershell
.\scripts\docker-push.ps1 -Username YOUR_GITHUB_USERNAME -Registry ghcr.io
```

### Step 3: Install Docker on VPS

```bash
# Connect to VPS
ssh user@your-vps-ip

# Install Docker
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER

# Logout and login for group to take effect
exit
```

### Step 4: Deploy on VPS

Copy `scripts/deploy-docker.sh` to the VPS and run it:

```bash
ssh user@your-vps-ip

# Copy the script (from Windows, one time)
# scp scripts/deploy-docker.sh user@your-vps-ip:~/

chmod +x ~/deploy-docker.sh
./deploy-docker.sh YOUR_GITHUB_USERNAME
```

On first run the script will:
1. Create `~/tgbot/.env` from a template
2. Ask you to fill in the values and re-run

Edit the config:
```bash
nano ~/tgbot/.env
```

Add:
```env
TELEGRAM_BOT_TOKEN=your_token_here
OPENAI_API_KEY=sk-your-key-here
OPENAI_MODEL=gpt-4o
ALLOWED_USERS=your_user_id
```

Then re-run:
```bash
./deploy-docker.sh YOUR_GITHUB_USERNAME
```

The script pulls the image, stops the old container (if any), starts a new one with
`--restart unless-stopped` and log rotation (`--log-opt max-size=10m --log-opt max-file=3`),
and cleans up dangling images.

### Step 5: Manual Pull and Run (alternative)

If you prefer not to use the script:

```bash
# Pull image
docker pull ghcr.io/YOUR_GITHUB_USERNAME/tgbot:latest

# Run container
docker run -d \
    --name tgbot \
    --restart unless-stopped \
    --env-file ~/tgbot/.env \
    --log-opt max-size=10m \
    --log-opt max-file=3 \
    ghcr.io/YOUR_GITHUB_USERNAME/tgbot:latest

# Check logs
docker logs -f tgbot
```

### Updating (Option B)

On Windows:
```powershell
.\scripts\docker-push.ps1 -Username YOUR_GITHUB_USERNAME -Registry ghcr.io
```

On VPS (using the deploy script):
```bash
./deploy-docker.sh YOUR_GITHUB_USERNAME
```

Or manually on VPS:
```bash
docker pull ghcr.io/YOUR_GITHUB_USERNAME/tgbot:latest
docker rm -f tgbot
docker run -d \
    --name tgbot \
    --restart unless-stopped \
    --env-file ~/tgbot/.env \
    --log-opt max-size=10m \
    --log-opt max-file=3 \
    ghcr.io/YOUR_GITHUB_USERNAME/tgbot:latest
```

### Using Docker Hub (alternative registry)

If you prefer Docker Hub over ghcr.io:

```powershell
# Login
docker login

# Build and push (no registry prefix = Docker Hub)
.\scripts\docker-push.ps1 -Username YOUR_DOCKERHUB_USERNAME
```

On VPS:
```bash
docker pull YOUR_DOCKERHUB_USERNAME/tgbot:latest
```

---

## Option C: Docker Build on VPS

Copy source code to VPS and build Docker image there.

### Step 1: Install Docker on VPS

```bash
ssh user@your-vps-ip

sudo apt update && sudo apt upgrade -y
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER
exit
```

### Step 2: Copy Project to VPS

From Windows:
```powershell
cd D:\CMA\Work\GO\TgBot
tar -czvf tgbot.tar.gz --exclude=".env" --exclude=".git" .
scp tgbot.tar.gz user@your-vps-ip:~/
```

### Step 3: Setup on VPS

```bash
ssh user@your-vps-ip

mkdir -p ~/tgbot
cd ~/tgbot
tar -xzvf ~/tgbot.tar.gz
rm ~/tgbot.tar.gz

# Create .env
nano .env
```

Add configuration and save.

### Step 4: Build and Run

```bash
docker build -t tgbot .

docker run -d \
    --name tgbot \
    --restart unless-stopped \
    --env-file .env \
    tgbot

docker logs -f tgbot
```

### Updating (Option C)

```bash
cd ~/tgbot
docker rm -f tgbot
docker build -t tgbot .
docker run -d --name tgbot --restart unless-stopped --env-file .env tgbot
```

---

## Option D: Systemd with Go on VPS

Build from source on VPS using Go.

### Step 1: Install Go on VPS

```bash
ssh user@your-vps-ip

wget https://go.dev/dl/go1.25.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.5.linux-amd64.tar.gz
rm go1.25.5.linux-amd64.tar.gz

echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

go version
```

### Step 2: Copy and Build

From Windows:
```powershell
cd D:\CMA\Work\GO\TgBot
tar -czvf tgbot.tar.gz --exclude=".env" --exclude=".git" .
scp tgbot.tar.gz user@your-vps-ip:~/
```

On VPS:
```bash
mkdir -p ~/tgbot-src
cd ~/tgbot-src
tar -xzvf ~/tgbot.tar.gz

go build -o bot ./cmd/bot

sudo mkdir -p /opt/tgbot
sudo cp bot /opt/tgbot/
```

### Step 3: Setup Service

```bash
sudo useradd -r -s /bin/false tgbot
sudo chown -R tgbot:tgbot /opt/tgbot

sudo nano /opt/tgbot/.env
# Add configuration

sudo chown tgbot:tgbot /opt/tgbot/.env
sudo chmod 600 /opt/tgbot/.env
```

Create systemd service (same as Option A, Step 5).

### Step 4: Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now tgbot
sudo journalctl -u tgbot -f
```

---

## Management Commands

### Systemd (Options A, D)

```bash
sudo systemctl status tgbot      # Status
sudo systemctl stop tgbot        # Stop
sudo systemctl start tgbot       # Start
sudo systemctl restart tgbot     # Restart
sudo journalctl -u tgbot -f      # Logs (follow)
sudo journalctl -u tgbot -n 100  # Logs (last 100)
sudo systemctl disable tgbot     # Disable autostart
```

### Docker (Options B, C)

```bash
docker ps                        # List running
docker logs -f tgbot             # Logs (follow)
docker logs --tail 100 tgbot     # Logs (last 100)
docker stop tgbot                # Stop
docker start tgbot               # Start
docker restart tgbot             # Restart
docker rm -f tgbot               # Remove container
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | - | Bot token from @BotFather |
| `OPENAI_API_KEY` | Yes | - | OpenAI API key |
| `OPENAI_MODEL` | No | gpt-4o | Responses API model for chat, reasoning, and tool orchestration |
| `OPENAI_BASE_URL` | No | - | Custom OpenAI-compatible base URL with Responses API support |
| `ALLOWED_USERS` | No | - | Comma-separated user IDs |
| `MAX_HISTORY` | No | 20 | Max messages in context |
| `LOG_LEVEL` | No | info | Logging level (debug/info/warn/error) |
| `LOG_FORMAT` | No | text | Log format (text/json) |

### Model Guidance

- Keep `OPENAI_MODEL` user-controlled.
- Use a model that supports the Responses API.
- Image generation/editing is executed through the Responses API `image_generation` tool and currently relies on `gpt-image-1` under the hood.

### Getting Tokens

1. **Telegram Bot Token**: [@BotFather](https://t.me/BotFather)
2. **OpenAI API Key**: [platform.openai.com/api-keys](https://platform.openai.com/api-keys)
3. **Your User ID**: [@userinfobot](https://t.me/userinfobot)

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Bot doesn't start | Check logs: `docker logs tgbot` or `journalctl -u tgbot` |
| "Access denied" | Verify ALLOWED_USERS contains your Telegram user ID |
| Connection timeout | Check firewall: bot needs outbound HTTPS (port 443) |
| Token invalid | Verify TELEGRAM_BOT_TOKEN in .env |
| OpenAI error | Verify OPENAI_API_KEY and check API quota |
| Permission denied | Check file ownership: `ls -la /opt/tgbot/` |
| Service won't start | Check syntax: `sudo systemctl status tgbot` |

### Firewall (if needed)

```bash
# UFW (Ubuntu)
sudo ufw allow out 443/tcp

# Or disable firewall for testing
sudo ufw disable
```

### Check Network

```bash
# Test Telegram API
curl -I https://api.telegram.org

# Test OpenAI API
curl -I https://api.openai.com
```
