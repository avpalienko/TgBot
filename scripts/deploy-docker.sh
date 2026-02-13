#!/usr/bin/env bash
# Deploy/redeploy TgBot as a Docker container from ghcr.io
# Run this script directly on the VPS.
#
# Usage:
#   ./deploy-docker.sh <github-username> [tag]
#
# Examples:
#   ./deploy-docker.sh myuser              # pulls ghcr.io/myuser/tgbot:latest
#   ./deploy-docker.sh myuser v1.2.0       # pulls ghcr.io/myuser/tgbot:v1.2.0

set -euo pipefail

# ── Colors ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# ── Constants ────────────────────────────────────────────────────────────────
CONTAINER_NAME="tgbot"
ENV_DIR="$HOME/tgbot"
ENV_FILE="$ENV_DIR/.env"

# ── Arguments ────────────────────────────────────────────────────────────────
if [ $# -lt 1 ]; then
    echo -e "${RED}Usage: $0 <github-username> [tag]${NC}"
    echo ""
    echo "  github-username  GitHub username or org"
    echo "  tag              Image tag (default: latest)"
    echo ""
    echo "Example: $0 myuser"
    exit 1
fi

USERNAME="$1"
TAG="${2:-latest}"
IMAGE="ghcr.io/${USERNAME}/tgbot:${TAG}"

echo -e "${CYAN}=== TgBot Docker Deploy ===${NC}"
echo -e "Image: ${IMAGE}"
echo ""

# ── Step 1: Validate prerequisites ──────────────────────────────────────────
echo -e "${YELLOW}[1/5] Checking prerequisites...${NC}"

if ! command -v docker &>/dev/null; then
    echo -e "${RED}Error: Docker is not installed.${NC}"
    echo "Install it with: curl -fsSL https://get.docker.com | sudo sh"
    exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
    echo -e "${YELLOW}Warning: ${ENV_FILE} not found. Creating from template...${NC}"
    mkdir -p "$ENV_DIR"
    cat > "$ENV_FILE" <<'ENVEOF'
# TgBot configuration
# Fill in the values and re-run the deploy script.

TELEGRAM_BOT_TOKEN=your_bot_token_here
OPENAI_API_KEY=sk-your-api-key-here
OPENAI_MODEL=gpt-4o
# OPENAI_BASE_URL=https://api.openai.com/v1
ALLOWED_USERS=
MAX_HISTORY=20
LOG_LEVEL=info
LOG_FORMAT=text
ENVEOF
    chmod 600 "$ENV_FILE"
    echo -e "${RED}Created ${ENV_FILE} with default values.${NC}"
    echo "Edit it with:  nano ${ENV_FILE}"
    echo "Then re-run this script."
    exit 1
fi

echo -e "${GREEN}OK${NC}"

# ── Step 2: Pull image ──────────────────────────────────────────────────────
echo -e "${YELLOW}[2/5] Pulling image...${NC}"

if ! docker pull "$IMAGE"; then
    echo ""
    echo -e "${RED}Failed to pull image.${NC}"
    echo "If the repository is private, log in first:"
    echo "  docker login ghcr.io -u ${USERNAME}"
    exit 1
fi

echo -e "${GREEN}OK${NC}"

# ── Step 3: Stop and remove old container ────────────────────────────────────
echo -e "${YELLOW}[3/5] Stopping old container...${NC}"

if docker container inspect "$CONTAINER_NAME" &>/dev/null; then
    docker rm -f "$CONTAINER_NAME" >/dev/null
    echo -e "Removed old container ${GREEN}OK${NC}"
else
    echo "No existing container found, skipping"
fi

# ── Step 4: Start new container ─────────────────────────────────────────────
echo -e "${YELLOW}[4/5] Starting container...${NC}"

docker run -d \
    --name "$CONTAINER_NAME" \
    --restart unless-stopped \
    --env-file "$ENV_FILE" \
    --log-opt max-size=10m \
    --log-opt max-file=3 \
    "$IMAGE"

echo -e "${GREEN}OK${NC}"

# ── Step 5: Cleanup old images ──────────────────────────────────────────────
echo -e "${YELLOW}[5/5] Cleaning up dangling images...${NC}"

DANGLING=$(docker images -f "dangling=true" -q 2>/dev/null || true)
if [ -n "$DANGLING" ]; then
    docker rmi $DANGLING 2>/dev/null || true
    echo -e "Removed dangling images ${GREEN}OK${NC}"
else
    echo "Nothing to clean up"
fi

# ── Done ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${CYAN}=== Deploy complete ===${NC}"
echo ""
docker ps --filter "name=${CONTAINER_NAME}" --format "table {{.ID}}\t{{.Image}}\t{{.Status}}\t{{.Names}}"
echo ""
echo -e "${YELLOW}Recent logs:${NC}"
sleep 1
docker logs --tail 15 "$CONTAINER_NAME" 2>&1 || true
echo ""
echo -e "Follow logs:  ${CYAN}docker logs -f ${CONTAINER_NAME}${NC}"
