FROM node:22-alpine AS frontend-builder

WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ ./
RUN npm run build

# ─────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS backend-builder

WORKDIR /src
RUN apk add --no-cache build-base

COPY backend/go.mod backend/go.sum* ./backend/
COPY go.work ./
WORKDIR /src/backend
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=1 go build -o /out/releasehub ./cmd/releasehub

# ─────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata nginx

WORKDIR /app

# Backend binary
COPY --from=backend-builder /out/releasehub /app/releasehub

# Frontend static files
COPY --from=frontend-builder /src/frontend/dist /usr/share/nginx/html

# Nginx config (reverse proxy to local backend)
COPY docker/nginx.conf /etc/nginx/http.d/default.conf

# Startup script: run nginx + backend together
COPY docker/start.sh /app/start.sh
RUN chmod +x /app/start.sh

EXPOSE 80

CMD ["/app/start.sh"]
