# Build stage
FROM golang:1.25-alpine AS builder

# Install git for version info
RUN apk add --no-cache git

WORKDIR /app

# Download dependencies first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Get version info and build
RUN GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown") && \
    GIT_DATE=$(git log -1 --format=%ci 2>/dev/null || echo "unknown") && \
    GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown") && \
    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") && \
    PKG="github.com/user/tgbot/internal/version" && \
    CGO_ENABLED=0 GOOS=linux go build \
        -ldflags="-s -w \
            -X '${PKG}.GitCommit=${GIT_COMMIT}' \
            -X '${PKG}.GitDate=${GIT_DATE}' \
            -X '${PKG}.GitBranch=${GIT_BRANCH}' \
            -X '${PKG}.BuildDate=${BUILD_DATE}'" \
        -o bot ./cmd/bot

# Runtime stage
FROM alpine:3.19

# Add ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/bot .

# Run as non-root user
RUN adduser -D -u 1000 botuser
USER botuser

CMD ["./bot"]
