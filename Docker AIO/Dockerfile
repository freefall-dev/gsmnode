# syntax=docker/dockerfile:1
#
# All-in-one image: PocketBase + API Server + Web App in a single container.
#
# The build context MUST be the project root so this file can reach both
# "API Server/" and "Web App/". Build it with:
#
#   docker build -f "Docker AIO/Dockerfile" -t gsmnode-aio .
#
# Run it (all three services start together):
#
#   docker run -d --name gsmnode -p 80:80 -p 8070:8070 \
#     -e PB_ADMIN_EMAIL=admin@example.com \
#     -e PB_ADMIN_PASSWORD=change-me \
#     -e JWT_SECRET=change-me-to-a-long-random-string \
#     -e GSMNODE_SUPERADMIN_EMAIL=owner@example.com \
#     -e GSMNODE_SUPERADMIN_PASSWORD=change-me \
#     -v gsmnode_pb:/pb/pb_data \
#     gsmnode-aio
#
# On first boot the API Server reconciles the PocketBase schema and creates the
# app super-admin from GSMNODE_SUPERADMIN_*. Both steps are idempotent.
#
# Then: web app on http://host/  and PocketBase admin on http://host:8070/_/

# --- Stage 1: build the Go API Server ---------------------------------------
FROM golang:1.26-alpine AS api-build
WORKDIR /src
COPY ["API Server/go.mod", "./"]
COPY ["API Server/go.su[m]", "./"]
RUN go mod download
# Only cmd/ + internal are needed; the panel is already built into
# internal/api/dist and embedded via //go:embed. Entry point is cmd/server.
COPY ["API Server/cmd", "./cmd"]
COPY ["API Server/internal", "./internal"]
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api-server ./cmd/server

# --- Stage 2: build the Vue Web App -----------------------------------------
# The Vue source lives under "Web App/web/".
FROM node:22-alpine AS web-build
WORKDIR /app
COPY ["Web App/web/package.json", "Web App/web/package-lock.json", "./"]
RUN npm ci
COPY ["Web App/web/index.html", "Web App/web/vite.config.js", "./"]
COPY ["Web App/web/src", "./src"]
COPY ["Web App/web/public", "./public"]
# Empty -> bundle uses same-origin "/api", proxied to the API Server by nginx.
ARG VITE_API_BASE
# vite.config writes to ../server/dist by default; emit into ./dist here.
RUN npm run build -- --outDir dist --emptyOutDir

# --- Stage 3: runtime (all services) ----------------------------------------
FROM alpine:latest

ARG PB_VERSION=""
ARG TARGETARCH="amd64"

RUN apk add --no-cache ca-certificates tzdata unzip wget nginx supervisor \
    && mkdir -p /run/nginx

# PocketBase from the official release (pinned via PB_VERSION, else latest).
WORKDIR /pb
RUN set -eux; \
    ver="${PB_VERSION}"; \
    if [ -z "$ver" ]; then \
      ver="$(wget -qO- https://api.github.com/repos/pocketbase/pocketbase/releases/latest \
        | grep -o '"tag_name": *"v[^"]*"' | head -1 | sed -E 's/.*"v([^"]+)".*/\1/')"; \
    fi; \
    echo "Installing PocketBase v${ver} (${TARGETARCH})"; \
    wget -q -O /tmp/pb.zip \
      "https://github.com/pocketbase/pocketbase/releases/download/v${ver}/pocketbase_${ver}_linux_${TARGETARCH}.zip"; \
    unzip /tmp/pb.zip -d /pb; \
    rm /tmp/pb.zip

# API Server binary + built Web App static assets.
COPY --from=api-build /out/api-server /usr/local/bin/api-server
COPY --from=web-build /app/dist /usr/share/nginx/html

# nginx: serve the SPA and proxy /api/ to the API Server on localhost.
RUN cat > /etc/nginx/http.d/default.conf <<'NGINX'
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    gzip on;
    gzip_types text/plain text/css application/javascript application/json image/svg+xml;
    gzip_min_length 1024;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        try_files $uri =404;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
NGINX

# supervisord runs the three processes and keeps them alive.
RUN cat > /etc/supervisord.conf <<'SUPERVISOR'
[supervisord]
nodaemon=true
user=root
pidfile=/run/supervisord.pid
logfile=/dev/null
logfile_maxbytes=0

; PocketBase: upsert the superuser (idempotent) then serve.
[program:pocketbase]
directory=/pb
command=/bin/sh -c '/pb/pocketbase superuser upsert "$PB_ADMIN_EMAIL" "$PB_ADMIN_PASSWORD" 2>/dev/null || true; exec /pb/pocketbase serve --http=0.0.0.0:8070'
priority=10
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0

; API Server: wait for PocketBase to be healthy, then start.
[program:api-server]
command=/bin/sh -c 'until wget -qO- http://127.0.0.1:8070/api/health >/dev/null 2>&1; do echo "waiting for pocketbase..."; sleep 1; done; exec /usr/local/bin/api-server'
priority=20
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0

[program:nginx]
command=/usr/sbin/nginx -g 'daemon off;'
priority=30
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
SUPERVISOR

# API Server config: everything is local to this container.
ENV API_ADDR=:8080 \
    POCKETBASE_URL=http://127.0.0.1:8070 \
    JWT_ACCESS_TTL=24h \
    MESSAGE_TTL=5m \
    CORS_ALLOW_ORIGINS=http://localhost:8090 \
    PB_BOOTSTRAP=true

# Required at runtime (no safe defaults): PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD, and
# JWT_SECRET (a long random string). Pass them with `docker run -e ...`.
# Optional: GSMNODE_SUPERADMIN_EMAIL / GSMNODE_SUPERADMIN_PASSWORD create the
# first app super-admin on boot; PB_BOOTSTRAP=false skips schema setup.

VOLUME /pb/pb_data
# 80 = Web App, 8070 = PocketBase admin, 8080 = API Server + embedded API panel.
EXPOSE 80 8070 8080

CMD ["supervisord", "-c", "/etc/supervisord.conf"]
