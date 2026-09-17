# Stage 1: Build static binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o telego-server ./cmd/server

# Stage 2: Minimal hardened runtime image (~25MB)
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S -g 10001 telego && \
    adduser -S -u 10001 -G telego telego

WORKDIR /app

COPY --from=builder /app/telego-server /app/telego-server

USER telego:telego

EXPOSE 8082

ENV TELEGO_HTTP_ADDR=0.0.0.0:8082 \
    TELEGO_REDIS_ADDR=redis:6379 \
    TELEGO_LOG_LEVEL=info \
    TELEGO_LOG_FORMAT=json

ENTRYPOINT ["/app/telego-server"]
