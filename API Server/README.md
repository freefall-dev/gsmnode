# gsmnode — API Server

Go service that is the **single trusted entry point** in front of PocketBase.
The Web App and Phone Agent talk only to this server; it performs all PocketBase
access as a superuser and enforces ownership in application logic.

```
Web App     ─┐
             ├─►  API Server (:8080)  ─►  PocketBase (10.2.1.10:8028)
Phone Agent ─┘
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
| `MESSAGE_TTL` | How long a message may stay unprocessed before the sweeper fails it | `5m` |
| `CORS_ALLOW_ORIGINS` | Comma list, or `*` | `*` |
| `WEBAPP_URL` | Probed at `/healthz` for the Web App health on the panel | `http://localhost:8090` |
| `PLUGINS_FILE` | Local JSON store for plugin enable-state + config | `plugins.json` |
| `PB_BOOTSTRAP` | Reconcile the schema + super-admin on boot | `true` |
| `GSMNODE_SUPERADMIN_EMAIL` / `_PASSWORD` / `_NAME` | First app login (role `superadmin`), created on boot when both email and password are set — distinct from the PocketBase superuser | — |

`WEBAPP_URL` and `CORS_ALLOW_ORIGINS` are also editable from the panel's
**Settings**, which merges the change back into `.env` so it survives a restart.
That file and `PLUGINS_FILE` are resolved relative to the working directory, so
the server needs one it can write.

## 2. Set up PocketBase collections

Creates/updates `organizations`, `devices`, `messages`, `inbox`, `calls` and
`webhooks` (the default `users` auth collection is reused, with `role`,
`organization` and `pluginSettings` added). Idempotent.

The server does the same reconcile itself on boot unless `PB_BOOTSTRAP=false`,
so this script is for setting the schema up without starting the server —
running both is harmless.

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

### In Docker

[`docker-compose.yml`](docker-compose.yml) here runs this server alone against an
external PocketBase, reaching a Web App on the host through
`host.docker.internal`. For the full stack — PocketBase and the Web App
alongside it — use [`../Docker/`](../Docker/) or the single-container
[`../Docker AIO/`](../Docker%20AIO/) instead.

The image runs unprivileged from `/data`, and that is where `plugins.json` and
the `.env` the panel writes back to end up, so `/data` is a volume in every
compose file here. Mount it: dropping it loses plugin configuration each time
the container is recreated.

## API

### Authentication scheme

- **Client API** (`/api/...`): `Authorization: Bearer <JWT>` from `/api/auth/login`.
  Used by the Web App and integrators.
- **Mobile API** (`/api/mobile/...`): `Authorization: Bearer <device_token>`
  returned by device registration. Used by the Phone Agent.

### Client / 3rd-party endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/health` | Readiness probe (public) |
| `POST` | `/api/auth/login` | `{email, password}` → JWT + user |
| `POST` | `/api/auth/refresh` | New JWT (auth) |
| `GET` | `/api/auth/me` | Current user |
| `GET` | `/api/devices` | List your devices |
| `DELETE` | `/api/devices/{id}` | Remove a device |
| `POST` | `/api/messages` | Enqueue SMS/data/MMS (see below) |
| `POST` | `/api/calls` | Enqueue a phone call `{phone_number, device_id?, sim_number?}` |
| `GET` | `/api/calls` | List the call log (`?direction=incoming\|outgoing`) |
| `GET` | `/api/messages` | List messages (`?status=&device_id=&type=&page=&per_page=`) |
| `GET` | `/api/messages/{id}` | Message state |
| `GET` | `/api/inbox` | Received SMS/data/MMS (`?type=sms\|data\|mms`) |
| `GET` | `/api/webhooks` | List webhooks |
| `POST` | `/api/webhooks` | Register `{event, url, device_id?, secret?}` |
| `DELETE` | `/api/webhooks/{id}` | Delete webhook |
| `GET` | `/api/integrations/email-to-sms` | Your resolved Email-to-SMS settings (cascade) |
| `PUT` | `/api/integrations/email-to-sms` | Save your (or your org's) IMAP mailbox `{enabled?, scope?, config?}` |
| `POST` | `/api/integrations/email-to-sms/health` | Probe your resolved IMAP mailbox |

### Plugins (superadmin)

Extension services managed by a superadmin; built-in state persists to
`plugins.json` (`PLUGINS_FILE`). See
[`internal/plugins/README.md`](internal/plugins/README.md).

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/admin/plugins` | List plugins + state + last health |
| `PUT` | `/api/admin/plugins/{name}` | Enable/disable + configure `{enabled?, config?}` |
| `POST` | `/api/admin/plugins` | Register an external plugin `{name, baseURL, provider?}` |
| `DELETE` | `/api/admin/plugins/{name}` | Remove an external plugin |
| `POST` | `/api/admin/plugins/{name}/health` | Run a health check |

The built-in **`email-to-sms`** plugin turns inbound email (`<phone>@<domain>`)
into outbound SMS via an SMTP server and/or IMAP polling.

`POST /api/messages` accepts a `type` of `sms` (default), `data`, or `mms`:

- **sms** — `{phone_numbers[], text_message, device_id?, sim_number?, schedule_at?}`
- **data** — `{type:"data", phone_numbers[], data_payload(base64), data_port?, ...}`
- **mms** — `{type:"mms", phone_numbers[], text_message?, subject?, attachments:[{filename, content_type, data(base64)}], ...}`

Any send may set `encrypted: true`, in which case `phone_numbers` + `text_message`
hold client-side ciphertext (see **End-to-end encryption** below) that the server
stores and relays verbatim.

`schedule_at` defers a send: the message stays `Pending` and is withheld from
`GET /api/mobile/v1/messages` until that time passes, so a device never sees it
early. Its expiry timeout is measured from the scheduled time rather than from
creation, giving a device its normal window to pick the message up once due.

### Mobile / device endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/mobile/v1/device` | Register device (auth: **user JWT**) → returns `auth_token` |
| `POST` | `/api/mobile/v1/ping` | Heartbeat (auth: device token). Optional body `{sims: [...]}` advertises the device's SIM slots |
| `GET` | `/api/mobile/v1/messages` | Pull pending messages that are due; marks them `Processed` |
| `PATCH` | `/api/mobile/v1/messages/{id}` | Report `{status, error?}` (`Sent`/`Delivered`/`Failed`) |
| `POST` | `/api/mobile/v1/inbox` | Report received SMS/data/MMS `{type?, phone_number, message, data_payload?, data_port?, subject?, attachments?, received_at?, sim_slot?, encrypted?}` |
| `POST` | `/api/mobile/v1/calls` | Report a call event `{phone_number, direction, status, sim_slot?, duration?, started_at?}` |

### Multiple SIM cards

On dual-SIM devices the phone enumerates its active SIMs (`READ_PHONE_STATE`) and
reports them on each heartbeat. `GET /api/devices` then returns a `sims` array per
device — `[{slot, subscription_id, carrier, number, display_name}]` — so callers
know which slots exist before selecting one.

- **Outbound:** pass `sim_number` (the 0-based slot) to `POST /api/messages`. The
  device sends on that SIM's radio; if the requested slot has no active
  subscription (or `READ_PHONE_STATE` isn't granted) the send is **rejected** and
  reported `Failed` rather than silently going out on the default SIM.
- **Calls:** `POST /api/calls` takes the same `sim_number`. It is stored on the
  call message like any other, and the device dials through that SIM's calling
  account; a slot with no active subscription falls back to the phone's default
  account rather than failing the call.
- **Inbound:** received SMS carry `sim_slot` (the 0-based slot they arrived on),
  surfaced on `GET /api/inbox` items and in the `sms:received` webhook payload.

### Message lifecycle

`Pending` → (device pulls) `Processed` → `Sent` → `Delivered`, or `Failed`.

Calls are stored as messages with `type: "call"` (SMS is `type: "sms"`) and flow
through the same pull/report pipeline. The device dials the number natively and
reports `Sent` (a call has no delivery report) or `Failed`.

### Webhook events

Delivered as `POST {event, device_id, payload, created_at}` to the registered URL.

| Event | Fires when |
|---|---|
| `sms:received` | An inbound text SMS is reported |
| `sms:sent` / `sms:delivered` / `sms:failed` | An outbound message changes state |
| `sms:data-received` | An inbound binary data SMS is reported |
| `mms:received` | An inbound MMS arrives (notification) |
| `mms:downloaded` | An inbound MMS's body + attachments are available |
| `call:received` | An inbound call is reported |
| `call:sent` | An outbound call is reported |
| `call:failed` | A call is missed / rejected / failed |

#### Signing

Register with a `secret` and every delivery to that URL is signed with it:

```
X-GsmNode-Timestamp: 1700000000
X-GsmNode-Signature: sha256=<hex HMAC-SHA256 of "<timestamp>.<raw body>">
```

The receiver recomputes the MAC over the bytes it read and rejects a mismatch,
and rejects a timestamp too far from its own clock. Both are needed: the
signature proves the delivery came from this server and was not edited, and the
age check stops a captured one being replayed. The timestamp is inside the MAC
so it cannot be rewritten to make an old delivery look fresh.

The secret is chosen by the subscriber, stored on the webhook record, and never
returned — it is deliberately absent from the webhook DTO, so listing your
webhooks cannot read it back. A webhook registered without one is delivered
unsigned, which is what keeps subscriptions made before signing existed working.

Delivery failures log the webhook's origin only, never the full URL: a receiver
like Home Assistant authenticates by an unguessable path, so the path is itself
a credential and does not belong in a log file.

### End-to-end encryption (optional)

When a client sets a shared passphrase, it encrypts `text_message` and each
recipient number (and decrypts the inbox) itself, marking the record
`encrypted: true`. The API Server and PocketBase only ever store ciphertext —
they never see the passphrase. The scheme is AES-256-GCM with a PBKDF2-HMAC-SHA256
key (150k iterations); the wire form is `gsmenc:v1:` + base64(`salt16‖iv12‖ct`).
The Web App (`web/src/crypto.js`) and Phone Agent
(`lib/services/crypto_service.dart`) implement the identical format so they
interoperate. With no passphrase set, everything is stored in cleartext as before.

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
internal/bootstrap          schema + super-admin reconcile on boot
internal/api                router, middleware, handlers
  dist/                     built panel (generated; embedded at compile time)
internal/plugins            plugin contract, manager, built-ins
panel/                      Vue 3 + Tailwind source for the panel at /
scripts/                    PocketBase setup + run helpers
```
