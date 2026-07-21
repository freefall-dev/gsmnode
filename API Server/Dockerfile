# syntax=docker/dockerfile:1

# --- Build stage -------------------------------------------------------------
# Compile a static Go binary. The Vue panel is pre-built into internal/api/dist
# and embedded via //go:embed, so no Node toolchain is needed here.
FROM golang:1.26-alpine AS build

WORKDIR /src

# Cache module downloads separately from the source for faster rebuilds.
COPY go.mod ./
# go.sum is optional; copy it if present.
COPY go.su[m] ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a static binary that runs on a bare alpine image. The
# entry point is the cmd/server package.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api-server ./cmd/server

# --- Runtime stage -----------------------------------------------------------
FROM alpine:latest

# HTTPS calls to PocketBase need CA certificates; tzdata for correct timestamps.
RUN apk add --no-cache ca-certificates tzdata

# Run as an unprivileged user.
RUN addgroup -S app && adduser -S -G app app

COPY --from=build /out/api-server /usr/local/bin/api-server

# The server writes two things at runtime, both relative to its working
# directory: plugins.json (plugin enable-state + config) and .env, which the
# panel's Settings screen persists edits to. /data is its working directory so
# both land in one place that the app user can write and a volume can outlive
# the container — without it, plugin config is lost on every recreate and
# setting changes are lost on every restart.
RUN mkdir -p /data && chown app:app /data
WORKDIR /data
VOLUME /data

# Config comes entirely from environment variables (see .env.example).
# PB_ADMIN_EMAIL and PB_ADMIN_PASSWORD are required for PocketBase access.
ENV API_ADDR=:8080 \
    PLUGINS_FILE=/data/plugins.json
EXPOSE 8080

USER app
ENTRYPOINT ["/usr/local/bin/api-server"]
