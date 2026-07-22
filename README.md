# gsmnode

Turn Android phones into a programmable SMS gateway, controlled through a web UI
and a REST API — inspired by [android-sms-gateway](https://docs.sms-gate.app/).

The shared design system is signal-green `#2E9E6B` on ink, Space Grotesk
(display) · IBM Plex Sans (body) · JetBrains Mono (code/labels), the lowercase
`gsm`+`node` wordmark, and the two-arrow routing mark. The source design kit is
not part of this repository; the tokens are implemented per surface (`theme.js`,
`style.css`, `theme.dart`) and the exported brand images ship where a surface
needs to render them. Every UI surface implements it (the Web App, API panel
and Phone App share a persisted light/dark/system preference under the key
`gsmnode-theme`; the Phone Agent follows the system theme; the Home Assistant
Plugin ships the mark and both wordmark variants in its `brand/` folder).

The application surfaces sit in front of a shared PocketBase. **The API Server
is the only component that talks to PocketBase**; every other surface talks only
to the API Server.

```
┌────────────┐        ┌──────────────────┐        ┌──────────────┐
│  Web App   │───────►│                  │───────►│              │
│ (Vue/Go)   │        │                  │        │              │
├────────────┤        │                  │        │              │
│ Phone App  │───────►│                  │        │              │
│ (Flutter)  │        │   API Server     │        │  PocketBase  │
├────────────┤        │      (Go)        │        │    (:8028)   │
│Phone Agent │───────►│                  │        │              │
│ (Flutter)  │        │                  │        │              │
├────────────┤        │                  │        │              │
│ HA plugin  │◄──────►│                  │───────►│              │
│  (Python)  │        │                  │        │              │
└────────────┘        └──────────────────┘        └──────────────┘
```

Two distinct phone-side surfaces, deliberately kept separate:

- **Phone Agent** — *controls the phone*: sends/receives SMS & MMS and
  makes/receives calls on behalf of the gateway.
- **Phone App** — *controls the gateway*: a mobile mirror of the Web App, with
  the same screens and the same functionality. It can optionally sit behind the
  phone's own face/fingerprint lock (**Settings → App lock**).

They install separately (`app.gsmnode.phoneagent` and `app.gsmnode.phoneapp`) and can
sit side by side on one device.

The **Home Assistant Plugin** is the fourth client, and the only one that is
both: it sends SMS and places calls from automations, watches the gateway and
each phone as connectivity sensors, subscribes itself to the gateway's webhooks
so incoming messages and calls become Home Assistant events, and can put either
of the existing overviews in Home Assistant's sidebar. It is configured entirely
from the Home Assistant UI — nothing goes in `configuration.yaml`.

## Surfaces

| Folder | Stack | Port | Status |
|---|---|---|---|
| [`API Server/`](API%20Server/) | Go | `:8080` | ✅ Built & verified (live E2E) |
| [`Web App/`](Web%20App/) | Go BFF + Vue 3 + Tailwind | `:8090` | ✅ Built & verified |
| [`Phone Agent/`](Phone%20Agent/) | Flutter (Android) | — | ✅ Built & run on a real device; foreground service + delivery reports |
| [`Phone App/`](Phone%20App/) | Flutter (Android) | — | ✅ Built — mobile mirror of the Web App; not yet run against a live server |
| [`Home Assistant Plugin/`](Home%20Assistant%20Plugin/) | HA custom component (Python) | — | ✅ UI-only (no YAML): config flow, `send_sms`/`call` services, gateway + per-phone sensors, incoming events, notify entity, optional sidebar panel |

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

Building from source, against a PocketBase you run yourself. To skip all of this
and bring the server side up in containers instead, jump to
[Deploy with Docker](#deploy-with-docker).

```powershell
git clone https://github.com/freefall-dev/gsmnode.git
cd gsmnode
```

1. **PocketBase** v0.23+ — run it however you like (the
   [official binary](https://pocketbase.io/docs/), or the image built by
   [`Docker/pocketbase/`](Docker/pocketbase/)) and note its URL; the rest of this
   assumes `http://localhost:8028`. Create its superuser on first launch — that
   is the `PB_ADMIN_*` login below.
2. **API Server** (`:8080`):
   ```powershell
   cd "API Server"
   Copy-Item .env.example .env   # set POCKETBASE_URL and PB_ADMIN_*
   node scripts/setup-pocketbase.mjs           # one-time schema setup
   node scripts/create-user.mjs you@example.com "password" "Your Name"
   ./scripts/Run-ApiServer.ps1
   ```
3. **Web App** (`:8090`):
   ```powershell
   cd "Web App"; ./server/Run-WebApp.ps1
   ```
   Open http://localhost:8090 and sign in.
4. **Phone Agent** — see [`Phone Agent/README.md`](Phone%20Agent/README.md) (install
   Flutter + JDK 17, `flutter create`, copy `android_overlay/`, `flutter run`).
5. **Phone App** (optional) — see [`Phone App/README.md`](Phone%20App/README.md)
   (`flutter pub get`, `flutter run`, then point it at the API Server on the
   sign-in screen).
6. **Home Assistant Plugin** (optional) — install it through HACS from
   [`freefall-dev/gsmnode-ha`](https://github.com/freefall-dev/gsmnode-ha), or
   copy `Home Assistant Plugin/custom_components/gsmnode/` into
   `<config>/custom_components/` by hand. Restart Home Assistant, then add
   **gsmnode** under Settings → Devices & Services. See
   [`Home Assistant Plugin/README.md`](Home%20Assistant%20Plugin/README.md).

   That GitHub repository is this folder, published on its own — HACS installs
   only from a repository whose root is the integration. It is regenerated by
   [`scripts/publish-ha-plugin.sh`](scripts/publish-ha-plugin.sh); the process
   is written down in
   [`Home Assistant Plugin/PUBLISHING.md`](Home%20Assistant%20Plugin/PUBLISHING.md).

## Deploy with Docker

The run order above builds from source against a PocketBase you run yourself. To
bring the whole server side up in containers instead, pick one of two shapes —
both carry a `docker-compose.yml` that builds from this working tree, and a
`docker-compose.prod.yml` that pulls prebuilt images instead of building:

| | Layout | Use when |
|---|---|---|
| [`Docker/`](Docker/) | Three containers — PocketBase, API Server, Web App | You want to scale, replace or upgrade the pieces independently. The only one with published images, so the only one you can run without building |
| [`Docker AIO/`](Docker%20AIO/) | One container, all three under supervisord (nginx serves the SPA) | One host, one thing to run |

To build from this tree:

```powershell
cd Docker                      # or "Docker AIO"
Copy-Item .env.example .env
# edit .env: PB_ADMIN_* and GSMNODE_SUPERADMIN_* at minimum
docker compose up -d --build
```

Or, for `Docker/` only, to pull prebuilt images instead of building —
`docker-compose.prod.yml` and `.env.prod.example` are the only two files you need,
so this works without cloning the repository:

```powershell
cd Docker
Copy-Item .env.prod.example .env
# edit .env: PB_ADMIN_* and GSMNODE_SUPERADMIN_* at minimum
docker compose -f docker-compose.prod.yml up -d
```

Either way the Web App ends up on `:8090`, the API Server and its panel on
`:8080`, and PocketBase's admin UI on `:8070/_/`. Nothing needs seeding by hand:
PocketBase upserts its superuser from `PB_ADMIN_*`, and the API Server then
reconciles the schema and creates the app super-admin from
`GSMNODE_SUPERADMIN_*`. Every step is idempotent, so restarts and upgrades are
safe.

### Published images

Three images, on both registries, `linux/amd64` only — on arm64 (Raspberry Pi,
Apple Silicon) build from source with the first recipe above. `Docker AIO/` has
no published image; it always builds.

| | GHCR (the default) | Docker Hub |
|---|---|---|
| API Server | `ghcr.io/tajniak81/gsmnode-api-server` | `tajniak81/gsmnode-api-server` |
| Web App | `ghcr.io/tajniak81/gsmnode-web-app` | `tajniak81/gsmnode-web-app` |
| PocketBase | `ghcr.io/tajniak81/pocketbase-docker-root` | `tajniak81/pocketbase-docker-root` |

`docker-compose.prod.yml` defaults to the GHCR ones at `:latest`. Set `PB_IMAGE`
/ `API_IMAGE` / `WEB_IMAGE` in `.env` to pin a version, switch to Docker Hub, or
point at your own build — the gsmnode images also carry `0`, `0.0`, `0.0.4` and
the short commit sha, and the PocketBase one tracks PocketBase's own version.

`Docker/docker-compose.prod.yml` binds the API panel and the PocketBase admin UI
to localhost, leaving only the Web App on the network (`PB_BIND` / `API_BIND`
open them up). The all-in-one publishes all three on every interface — put it
behind a reverse proxy, or don't expose `:8070` and `:8080` on an untrusted one.

Two volumes hold everything worth keeping, and both want backing up: `pb_data`
is the database, and `api_data` is the API Server's plugin state — `plugins.json`
plus the settings the panel persists. Plugin config holds credentials such as
IMAP/SMTP logins, so treat it as sensitively as the database.

The phone surfaces are not containerized — they are Android apps, installed on
the phone (see their READMEs).

## Message lifecycle

`Pending` → (device pulls) `Processed` → `Sent` → `Delivered` · or `Failed`.

## Webhooks

Events `sms:received`, `sms:sent`, `sms:delivered`, `sms:failed`,
`sms:data-received`, `mms:received`, `mms:downloaded`, `call:received`,
`call:sent`, `call:failed` are POSTed to registered URLs as
`{event, device_id, payload, created_at}`. The
[Home Assistant Plugin](Home%20Assistant%20Plugin/README.md) subscribes itself to
whichever of these you tick and re-fires them on Home Assistant's event bus.

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
Agent (login). Message text and recipient numbers are then AES-256-GCM encrypted in
the browser/phone before they reach the API Server, which stores only ciphertext.
See [`API Server/README.md`](API%20Server/README.md) for the wire format.

## Per-surface docs

- [API Server README](API%20Server/README.md) — full endpoint reference, setup
- [Web App README](Web%20App/README.md) — dev/build, pages
- [Phone App README](Phone%20App/README.md) — Flutter build, screens, API mapping
- [Phone Agent README](Phone%20Agent/README.md) — Flutter build + native SMS wiring
- [Home Assistant Plugin README](Home%20Assistant%20Plugin/README.md) — install,
  services, sensors, incoming events, notify entity, sidebar panel

## License

[Apache License 2.0](LICENSE) — the same license Home Assistant itself uses.

The gsmnode name, wordmark and routing mark are **not** covered by that grant;
see [NOTICE](NOTICE) and section 6 of the License. The brand image files ship
inside the tree so the surfaces can render them, which is not permission to put
the marks on something else.
