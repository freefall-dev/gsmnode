# gsmnode — Web App

Browser console for the gateway: a **Vue 3 + Tailwind** SPA served by a small **Go
backend-for-frontend (BFF)**. Styled with the gsmnode design system
(`../Design/SMS Gateway logo design/`): signal-green `#2E9E6B` on ink, Space
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
    store/auth.js login state
    router.js     routes + auth guard
    views/        Login, Devices, Send, Messages, Inbox, Webhooks
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

## Pages

- **Login** — authenticates via `/api/auth/login`, stores the JWT. Includes an
  editable **Server settings** section (like the phone app): enter an API Server
  URL to point the browser directly at any server (uses the server's CORS), or
  leave it blank to use this site's built-in BFF proxy. The choice is remembered
  in `localStorage`.
- **Devices** — list/remove your registered phones, see online status & last seen.
- **Send SMS** — queue an outbound message (multiple numbers, pick device/SIM).
- **Call** — remotely tell a device to place an outbound phone call.
- **Messages** — outbound history with live status (`Pending`→…→`Delivered`/`Failed`), filterable.
- **Inbox** — incoming messages received by your devices.
- **Webhooks** — register/delete callbacks for `sms:received|sent|delivered|failed`.

The header shows a live **API Server status** indicator (green/red dot) that polls
`/api/health` every 10s against the configured server, with latency on hover.

Log in with the user you created via `../API Server/scripts/create-user.mjs`.
