# syntax=docker/dockerfile:1
#
# Web App image: the Vue SPA is built and embedded into the Go backend-for-
# frontend (BFF), which serves it and reverse-proxies /api/* to the API Server
# (API_BASE). This mirrors the production Run-WebApp.ps1 flow, so the browser is
# always same-origin and all data access still flows through the API Server.
#
#   Build context is the "Web App" directory (see Docker/docker-compose.yml).

# --- Stage 1: build the Vue SPA ---------------------------------------------
FROM node:22-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/index.html web/vite.config.js ./
COPY web/src ./src
COPY web/public ./public
# Empty -> bundle uses same-origin "/api", which the BFF proxies to API_BASE.
ARG VITE_API_BASE=""
ENV VITE_API_BASE=${VITE_API_BASE}
# vite.config writes to ../server/dist by default; emit into ./dist here so the
# next stage can embed it.
RUN npm run build -- --outDir dist --emptyOutDir

# --- Stage 2: build the Go BFF, embedding the SPA ---------------------------
FROM golang:1.26-alpine AS server-build
WORKDIR /src
COPY server/go.mod ./
# go.sum is optional (stdlib-only module today); copy it if present.
COPY server/go.su[m] ./
RUN go mod download
COPY server/ ./
# Embed the freshly built SPA (main.go uses //go:embed all:dist).
COPY --from=web-build /web/dist ./dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/web-bff .

# --- Runtime stage ----------------------------------------------------------
FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=server-build /out/web-bff /app/web-bff

# Config comes from environment variables (see server/.env.example).
ENV WEB_ADDR=:8090 \
    API_BASE=http://api-server:8080
EXPOSE 8090

USER app
ENTRYPOINT ["/app/web-bff"]
