# gsmnode — API Server

Go service that is the **single trusted entry point** in front of PocketBase.
The Web App and Phone App talk only to this server; it performs all PocketBase
access as a superuser and enforces ownership in application logic.

```
Web App  ─┐
          ├─►  API Server (:8080)  ─►  PocketBase (10.2.1.10:8028)
Phone App ─┘
```

The server root (`GET /`) serves a gsmnode-branded **web panel**: a live health
readout plus a quick reference of both API audiences. Open
http://localhost:8080/ in a browser to check the server at a glance.

The panel is a **Vue 3 + Tailwind v4** app in [`panel/`](panel/), built into
`internal/api/dist` and embedded into the Go binary at compile time:

```powershell
cd panel
npm install
npm run build        # outputs to ../internal/api/dist
cd ..; go build -o api-server.exe ./cmd/server   # embeds the fresh dist
```

For panel development with hot reload (proxies `/api` to a running server on
`:8080`): `cd panel; npm run dev` → http://localhost:5174.

## Requirements

- Go 1.26+ (`go version`)
- A reachable PocketBase v0.23+ instance and its **superuser** credentials
- Node 18+ (only for the setup scripts)

## 1. Configure

```powershell
Copy-Item .env.example .env
# edit .env: set PB_ADMIN_EMAIL, PB_ADMIN_PASSWORD, and a strong JWT_SECRET
```

| Variable | Purpose | Default |
|---|---|---|
| `API_ADDR` | Listen address | `:8080` |
| `POCKETBASE_URL` | PocketBase base URL | `http://10.2.1.10:8028` |
| `PB_ADMIN_EMAIL` / `PB_ADMIN_PASSWORD` | PocketBase superuser login | — (required) |
| `JWT_SECRET` | Signs client JWTs | dev placeholder |
| `JWT_ACCESS_TTL` | Access-token lifetime | `24h` |
| `CORS_ALLOW_ORIGINS` | Comma list, or `*` | `*` |

## 2. Set up PocketBase collections

Creates/updates `devices`, `messages`, `inbox`, `webhooks` (the default `users`
auth collection is reused). Idempotent.

```powershell
$env:POCKETBASE_URL="http://10.2.1.10:8028"
$env:PB_ADMIN_EMAIL="admin@example.com"
$env:PB_ADMIN_PASSWORD="your-password"
node scripts/setup-pocketbase.mjs
```

Create a login user:

```powershell
node scripts/create-user.mjs user@example.com "user-password" "Display Name"
```

## 3. Run

```powershell
./scripts/Run-ApiServer.ps1
# or:  go run ./cmd/server
```

Health check: `GET http://localhost:8080/api/health`.

## API

### Authentication scheme

- **Client API** (`/api/...`): `Authorization: Bearer <JWT>` from `/api/auth/login`.
  Used by the Web App and integrators.
- **Mobile API** (`/api/mobile/...`): `Authorization: Bearer <device_token>`
  returned by device registration. Used by the Phone App.

### Client / 3rd-party endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/health` | Readiness probe (public) |
| `POST` | `/api/auth/login` | `{email, password}` → JWT + user |
| `POST` | `/api/auth/refresh` | New JWT (auth) |
| `GET` | `/api/auth/me` | Current user |
| `GET` | `/api/devices` | List your devices |
| `DELETE` | `/api/devices/{id}` | Remove a device |
| `POST` | `/api/messages` | Enqueue SMS `{phone_numbers[], text_message, device_id?, sim_number?, schedule_at?}` |
| `POST` | `/api/calls` | Enqueue a phone call `{phone_number, device_id?}` |
| `GET` | `/api/messages` | List messages (`?status=&device_id=&type=&page=&per_page=`) |
| `GET` | `/api/messages/{id}` | Message state |
| `GET` | `/api/inbox` | Received SMS |
| `GET` | `/api/webhooks` | List webhooks |
| `POST` | `/api/webhooks` | Register `{event, url, device_id?}` |
| `DELETE` | `/api/webhooks/{id}` | Delete webhook |

### Mobile / device endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/mobile/v1/device` | Register device (auth: **user JWT**) → returns `auth_token` |
| `POST` | `/api/mobile/v1/ping` | Heartbeat (auth: device token). Optional body `{sims: [...]}` advertises the device's SIM slots |
| `GET` | `/api/mobile/v1/messages` | Pull pending messages; marks them `Processed` |
| `PATCH` | `/api/mobile/v1/messages/{id}` | Report `{status, error?}` (`Sent`/`Delivered`/`Failed`) |
| `POST` | `/api/mobile/v1/inbox` | Report received SMS `{phone_number, message, received_at?, sim_slot?}` |

### Multiple SIM cards

On dual-SIM devices the phone enumerates its active SIMs (`READ_PHONE_STATE`) and
reports them on each heartbeat. `GET /api/devices` then returns a `sims` array per
device — `[{slot, subscription_id, carrier, number, display_name}]` — so callers
know which slots exist before selecting one.

- **Outbound:** pass `sim_number` (the 0-based slot) to `POST /api/messages`. The
  device sends on that SIM's radio; if the requested slot has no active
  subscription (or `READ_PHONE_STATE` isn't granted) the send is **rejected** and
  reported `Failed` rather than silently going out on the default SIM.
- **Inbound:** received SMS carry `sim_slot` (the 0-based slot they arrived on),
  surfaced on `GET /api/inbox` items and in the `sms:received` webhook payload.

### Message lifecycle

`Pending` → (device pulls) `Processed` → `Sent` → `Delivered`, or `Failed`.

Calls are stored as messages with `type: "call"` (SMS is `type: "sms"`) and flow
through the same pull/report pipeline. The device dials the number natively and
reports `Sent` (a call has no delivery report) or `Failed`.

### Webhook events

`sms:received`, `sms:sent`, `sms:delivered`, `sms:failed`. Delivered as
`POST {event, device_id, payload, created_at}` to the registered URL.

## Quick smoke test

```powershell
# log in
$login = curl -s -X POST http://localhost:8080/api/auth/login `
  -H "Content-Type: application/json" `
  -d '{"email":"user@example.com","password":"user-password"}' | ConvertFrom-Json
$token = $login.access_token

# register a device (as the phone app would)
curl -s -X POST http://localhost:8080/api/mobile/v1/device `
  -H "Authorization: Bearer $token" -H "Content-Type: application/json" `
  -d '{"device_id":"test-1","name":"Test Phone","platform":"android"}'
```

## Project layout

```
cmd/server/main.go          entry point, wiring, graceful shutdown
internal/config             env/.env configuration
internal/pb                 PocketBase REST client (superuser)
internal/auth               JWT issue/verify, device token generation
internal/api                router, middleware, handlers
scripts/                    PocketBase setup + run helpers
```
