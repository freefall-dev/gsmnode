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

WORKDIR /app
COPY --from=build /out/api-server /app/api-server

# Config comes entirely from environment variables (see .env.example).
# PB_ADMIN_EMAIL and PB_ADMIN_PASSWORD are required for PocketBase access.
ENV API_ADDR=:8080
EXPOSE 8080

USER app
ENTRYPOINT ["/app/api-server"]
