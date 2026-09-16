# Stage 1: Build binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o telego-server ./cmd/server

# Stage 2: Minimal runtime image (~20MB)
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/telego-server /app/telego-server

EXPOSE 8082

ENTRYPOINT ["/app/telego-server"]
CMD ["--http-addr=0.0.0.0:8082", "--redis-addr=redis:6379", "--log-format=json"]
