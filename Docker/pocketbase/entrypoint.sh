#!/bin/sh
set -e

# Create/update the superuser from env vars so the API Server can authenticate
# on first boot. `superuser upsert` is idempotent (PocketBase v0.23+).
if [ -n "$PB_ADMIN_EMAIL" ] && [ -n "$PB_ADMIN_PASSWORD" ]; then
  echo "Ensuring PocketBase superuser $PB_ADMIN_EMAIL exists..."
  /pb/pocketbase superuser upsert "$PB_ADMIN_EMAIL" "$PB_ADMIN_PASSWORD" \
    || echo "warning: superuser upsert failed; create an admin via the UI at /_/"
fi

exec /pb/pocketbase serve --http=0.0.0.0:8070
