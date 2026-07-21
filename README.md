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

- `users` — auth (existing default collection) + `role, organization, pluginSettings`
- `organizations` — `name, pluginSettings` (tenants; `pluginSettings` holds the org layer of the plugin cascade)
- `devices` — `device_id, name, platform, app_version, push_token, auth_token, status, last_seen_at, owner`
- `messages` — `phone_numbers, text_message, type (sms/call/data/mms), sim_number, status, error, data_payload, data_port, subject, attachments, encrypted, schedule_at, sent_at, delivered_at, device, owner`
- `inbox` — `phone_number, message, type (sms/data/mms), received_at, sim_slot, data_payload, data_port, subject, attachments, encrypted, device, owner`
- `calls` — `phone_number, direction, status, sim_slot, duration, started_at, device, owner`
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

Events `sms:received`, `sms:sent`, `sms:delivered`, `sms:failed`,
`sms:data-received`, `mms:received`, `mms:downloaded`, `call:received`,
`call:sent`, `call:failed` are POSTed to registered URLs as
`{event, device_id, payload, created_at}`.

## Plugins & Email-to-SMS

The API Server has a **plugin system** (built-in Go connectors + external HTTP
plugins), managed by a superadmin in the panel's **Plugins** section or under
`/api/admin/plugins*`. Built-in config persists to `plugins.json`. See
[`API Server/internal/plugins/README.md`](API%20Server/internal/plugins/README.md).

The first built-in is **Email-to-SMS** (modelled on
[sms-gate.app](https://docs.sms-gate.app/services/email-to-sms/)): an email to
`<phone>@<domain>` becomes an outbound SMS. Two intake modes:

- **SMTP** — the plugin runs an SMTP server; the sender authenticates (AUTH PLAIN)
  with their gsmnode login and the SMS is owned by that user.
- **IMAP** — the plugin polls each user's own mailbox. Users connect their mailbox
  in the Web App (**Settings → Integrations**), resolved through a
  global → org → user cascade (`/api/integrations/email-to-sms`).

Per-user settings are generic: a plugin declares the fields it accepts
(`UserConfigurable`, or a `userConfig` block in an external plugin's manifest)
and the cascade, the `/api/integrations*` endpoints and the Web App form are all
derived from that declaration — no per-plugin API or UI code.

## End-to-end encryption

Optional and opt-in: set a shared passphrase in the Web App (Settings) and Phone
App (login). Message text and recipient numbers are then AES-256-GCM encrypted in
the browser/phone before they reach the API Server, which stores only ciphertext.
See [`API Server/README.md`](API%20Server/README.md) for the wire format.

## Per-surface docs

- [API Server README](API%20Server/README.md) — full endpoint reference, setup
- [Web App README](Web%20App/README.md) — dev/build, pages
- [Phone App README](Phone%20App/README.md) — Flutter build + native SMS wiring
