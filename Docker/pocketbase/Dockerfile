# syntax=docker/dockerfile:1

# PocketBase built from the official release binary on alpine:latest.
FROM alpine:latest

# Pin a version, or leave empty to fetch the latest release at build time.
ARG PB_VERSION=""
# Provided automatically by BuildKit (amd64 / arm64).
ARG TARGETARCH="amd64"

RUN apk add --no-cache ca-certificates unzip wget

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

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# pb_data holds the SQLite database and uploads — mount a volume here.
VOLUME /pb/pb_data
EXPOSE 8070

ENTRYPOINT ["/entrypoint.sh"]
