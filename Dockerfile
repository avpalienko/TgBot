# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download dependencies first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot ./cmd/bot

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
