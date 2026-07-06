# gsmnode

Turn Android phones into a programmable SMS gateway, controlled through a web UI
and a REST API — inspired by [android-sms-gateway](https://docs.sms-gate.app/).

Brand + design system live in
[`Design/SMS Gateway logo design/`](Design/SMS%20Gateway%20logo%20design/) —
signal-green `#2E9E6B` on ink, Space Grotesk (display) · IBM Plex Sans (body) ·
JetBrains Mono (code/labels), the lowercase `gsm`+`node` wordmark, and the
two-arrow routing mark. All three UI surfaces implement it (the Web App and API
panel share a persisted light/dark toggle, `localStorage` key `gsmnode-theme`,
`data-gsm-theme` attribute; the Phone App follows the system theme).

Three application surfaces sit in front of a shared PocketBase. **The API Server
is the only component that talks to PocketBase**; the Web App and Phone App talk
only to the API Server.

```
┌────────────┐        ┌──────────────────┐        ┌──────────────┐
│  Web App   │───────►│                  │───────►│              │
│ (Vue/Go)   │        │   API Server     │        │  PocketBase  │
├────────────┤        │     (Go)         │        │ 10.2.1.10:   │
│ Phone App  │───────►│                  │───────►│    8028      │
│ (Flutter)  │        └──────────────────┘        └──────────────┘
└────────────┘
```

## Surfaces

| Folder | Stack | Port | Status |
|---|---|---|---|
| [`API Server/`](API%20Server/) | Go | `:8080` | ✅ Built & verified (live E2E) |
| [`Web App/`](Web%20App/) | Go BFF + Vue 3 + Tailwind | `:8090` | ✅ Built & verified |
| [`Phone App/`](Phone%20App/) | Flutter (Android) | — | ✅ Built & run on a real device; foreground service + delivery reports |
| [`Home Assistant Plugin/`](Home%20Assistant%20Plugin/) | HA custom component (Python) | — | ✅ `notify.gsmnode` service; flow validated |

## PocketBase collections

Managed by `API Server/scripts/setup-pocketbase.mjs` (idempotent):

- `users` — auth (existing default collection)
- `devices` — `device_id, name, platform, app_version, push_token, auth_token, status, last_seen_at, owner`
- `messages` — `phone_numbers, text_message, sim_number, status, error, schedule_at, sent_at, delivered_at, device, owner`
- `inbox` — `phone_number, message, received_at, device, owner`
- `webhooks` — `event, url, device, owner`

Collections are locked to superuser access; the API Server enforces per-user
ownership in application logic.

## Run order

1. **PocketBase** — already running at `http://10.2.1.10:8028`.
2. **API Server** (`:8080`):
   ```powershell
   cd "API Server"
   Copy-Item .env.example .env   # fill in PB_ADMIN_* and JWT_SECRET
   node scripts/setup-pocketbase.mjs           # one-time schema setup
   node scripts/create-user.mjs you@example.com "password" "Your Name"
   ./scripts/Run-ApiServer.ps1
   ```
3. **Web App** (`:8090`):
   ```powershell
   cd "Web App"; ./server/Run-WebApp.ps1
   ```
   Open http://localhost:8090 and sign in.
4. **Phone App** — see [`Phone App/README.md`](Phone%20App/README.md) (install
   Flutter + JDK 17, `flutter create`, copy `android_overlay/`, `flutter run`).

## Message lifecycle

`Pending` → (device pulls) `Processed` → `Sent` → `Delivered` · or `Failed`.

## Webhooks

Events `sms:received`, `sms:sent`, `sms:delivered`, `sms:failed` are POSTed to
registered URLs as `{event, device_id, payload, created_at}`.

## Per-surface docs

- [API Server README](API%20Server/README.md) — full endpoint reference, setup
- [Web App README](Web%20App/README.md) — dev/build, pages
- [Phone App README](Phone%20App/README.md) — Flutter build + native SMS wiring
