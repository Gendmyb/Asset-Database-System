# Asset Database System — Multi-stage Dockerfile
# Stage 1: Build frontend (React + Vite)
# Stage 2: Build backend (Go, embed frontend dist)
# Stage 3: Minimal runtime image

# ---- Stage 1: Frontend Build ----
FROM node:22-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: Go Build (with embedded frontend) ----
FROM golang:1.25-alpine AS go-builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache go modules
COPY assetserver/go.mod assetserver/go.sum ./
RUN go mod download

# Copy backend source
COPY assetserver/ ./

# Copy frontend build output into assetserver/web/dist/ for embed
COPY --from=web-builder /src/web/dist ./web/dist

# Build with embedded frontend
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app ./cmd/api-server

# ---- Stage 3: Runtime ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata curl

COPY --from=go-builder /app /app

# Migrations (for goose-based migration in future phase)
COPY assetserver/migrations/ /migrations/

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app"]
