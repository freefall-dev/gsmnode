# gsmnode — Web App

Browser console for the gateway: a **Vue 3 + Tailwind** SPA served by a small **Go
backend-for-frontend (BFF)**. Styled with the gsmnode design system: signal-green
`#2E9E6B` on ink, Space
Grotesk (display) · IBM Plex Sans (body) · JetBrains Mono (code/labels), Lucide
icons, and a persisted light/dark toggle (`localStorage` key `gsmnode-theme`,
`data-gsm-theme` attribute). The BFF serves the built SPA and reverse-proxies
`/api/*` to the API Server, so the browser is always same-origin and all data
access still flows through the API Server.

```
Browser ─► Web App BFF (:8090) ──/api/*──► API Server (:8080) ─► PocketBase
                  └── serves embedded Vue SPA
```

## Layout

```
server/           Go BFF: embeds web/dist, proxies /api -> API_BASE
  main.go
  .env.example
  dist/           built SPA (generated; embedded at compile time)
web/              Vue 3 + Vite + Tailwind v4 source
  src/
    api.js        fetch wrapper (bearer token, same-origin /api)
    crypto.js     AES-256-GCM + PBKDF2 (matches both phone surfaces)
    theme.js      light/dark/system preference (gsmnode-theme)
    store/auth.js login state
    router.js     routes + auth guard
    components/   shared pieces (status dot, users/orgs/integrations managers)
    views/        Login, Devices, Send, Call, Messages, Inbox, Webhooks,
                  Settings, and Layout (the sidebar shell)
```

## Requirements

- Node 18+ and Go 1.26+
- A running **API Server** (see `../API Server`)

## Develop

Two terminals:

```powershell
# terminal 1 — API Server (see ../API Server/README.md)
cd "../API Server"; ./scripts/Run-ApiServer.ps1

# terminal 2 — Vite dev server with hot reload (proxies /api -> :8080)
cd web; npm install; npm run dev      # http://localhost:5173
```

## Build & run (production-style)

```powershell
./server/Run-WebApp.ps1               # builds frontend, then serves on :8090
# or manually:
cd web; npm run build                 # outputs to ../server/dist
cd ../server; go run .                # http://localhost:8090
```

Config (`server/.env`, copy from `.env.example`):

| Variable | Purpose | Default |
|---|---|---|
| `WEB_ADDR` | Listen address | `:8090` |
| `API_BASE` | API Server base URL | `http://localhost:8080` |

`GET /healthz` is a cheap liveness probe that answers without touching the SPA;
the API Server polls it to show this app's health on its own panel.

### In Docker

[`docker-compose.yml`](docker-compose.yml) here runs the BFF alone, pointing
`API_BASE` at an API Server on the host. For the full stack, use
[`../Docker/`](../Docker/) or the single-container
[`../Docker AIO/`](../Docker%20AIO/) — in the AIO image nginx takes the BFF's
place, serving the SPA and proxying `/api/` to the API Server in the same
container.

## Pages

- **Login** — authenticates via `/api/auth/login`, stores the JWT. Includes an
  editable **Server settings** section (as the Phone App's sign-in screen does):
  enter an API Server URL to point the browser directly at any server (uses the
  server's CORS), or leave it blank to use this site's built-in BFF proxy. The
  choice is remembered in `localStorage`.
- **Devices** — list/remove your registered phones, see online status, SIM slots
  & last seen.
- **Send SMS** — queue an outbound message (multiple numbers, pick device/SIM).
- **Call** — remotely tell a device to place an outbound phone call, and browse
  the call log by direction.
- **Messages** — outbound history with live status (`Pending`→…→`Delivered`/`Failed`), filterable.
- **Inbox** — incoming messages received by your devices.
- **Webhooks** — register/delete callbacks for `sms:received|sent|delivered|failed`.
- **Settings** — display name, password, theme (light/dark/system) and the
  end-to-end encryption passphrase, which stays in this browser. An
  **Integrations** tab carries each user's own plugin settings (their IMAP
  mailbox, for Email-to-SMS), on forms generated from what the plugin declares
  rather than written per plugin — see
  [`../API Server/internal/plugins/README.md`](../API%20Server/internal/plugins/README.md).
  Managers additionally get **Users** and **Organizations**.

The header shows a live **API Server status** indicator (green/red dot) that polls
`/api/health` every 10s against the configured server, with latency on hover.

Log in with the user you created via `../API Server/scripts/create-user.mjs`.
