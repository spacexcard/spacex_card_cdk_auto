# ===== Stage 1: build frontend =====
FROM node:20-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ===== Stage 2: build backend (CGO for sqlite3) =====
FROM golang:1.24-bookworm AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cdk-recharge ./cmd/server

# ===== Stage 3: runtime =====
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend  /out/cdk-recharge /app/cdk-recharge
COPY --from=frontend /src/frontend/dist /app/web
COPY VERSION /app/VERSION

ENV DB_PATH=/app/data/cdk_recharge.db \
    WEB_DIR=/app/web \
    SERVER_HOST=0.0.0.0 \
    SERVER_PORT=8080 \
    SERVER_MODE=release \
    INSTALL_MODE=wizard \
    TRUSTED_PROXIES=127.0.0.1,172.16.0.0/12

VOLUME ["/app/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://127.0.0.1:8080/health || exit 1

CMD ["/app/cdk-recharge"]
