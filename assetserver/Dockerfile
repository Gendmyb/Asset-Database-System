# Asset Database System — Dockerfile
# 多阶段构建: Go 1.23 → 最小镜像 (~12 MB)

FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY assetserver/go.mod assetserver/go.sum ./
RUN go mod download

COPY assetserver/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /api-server ./cmd/api-server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /api-server /usr/local/bin/api-server

EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/api-server"]
