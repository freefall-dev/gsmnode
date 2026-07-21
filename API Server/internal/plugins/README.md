# gsmnode plugins

A **plugin** extends the API Server behind one uniform contract. There are two
kinds:

| Kind | Written as | Added by | Rebuild? | Use when |
|---|---|---|---|---|
| **built-in** | Go code in this repo | a rebuild | yes | first-party, high-trust connectors/services |
| **external** | any HTTP service | registering a URL at runtime | **no** | third-party / less-trusted / independently deployed |

Enable-state and per-plugin config persist to `plugins.json` (gitignored;
override the path with `PLUGINS_FILE`) and load on boot. Every plugin is managed
by a **superadmin** from the panel (`/`) or the `/api/admin/plugins*` API.

Built-in today: **`email-to-sms`** — turns inbound email into outbound SMS
(SMTP server and/or IMAP polling). See `builtin/emailtosms/`.

## The contract

All plugins satisfy the Go interface in [`plugin.go`](plugin.go):

```go
type Plugin interface {
    Descriptor() Descriptor
    Init(ctx context.Context, config map[string]string) error
    HealthCheck(ctx context.Context) Health
    Invoke(ctx context.Context, action string, params json.RawMessage) (json.RawMessage, error)
    Shutdown(ctx context.Context) error
}
```

- **`Descriptor`** — static metadata (name, provider, version, capabilities,
  auth type, config fields). `ConfigFields` drives the panel's generated form.
- **`Init`** — called with the resolved config (secrets included) whenever the
  plugin is enabled or its config changes. Prepare clients / start workers here.
- **`HealthCheck`** — probe and classify: `Health{Status, LatencyMs, Detail}`
  where `Status` is `ok` / `degraded` / `down`.
- **`Invoke`** — run a named capability. Part of the contract for the future; no
  HTTP endpoint exposes it in v1.
- **`Shutdown`** — release resources / stop workers.

Set **`Secret: true`** on credential fields — the server masks them (`••••••••`)
and, on save, a field left at the mask keeps its stored value. **`Required: true`**
fields must be non-empty before the plugin can be enabled.

## Building a built-in

1. Create `internal/plugins/builtin/<name>/`, implement `Plugin`, and
   `plugins.Register("<name>", …)` from `init()`.
2. Blank-import the package from [`builtin/builtin.go`](builtin/builtin.go).
3. Rebuild: `go build ./cmd/server`. The plugin appears in the panel, disabled.

A built-in that must call back into the app (as `email-to-sms` does to
authenticate a sender and enqueue an SMS) defines a small host interface in its
own package and has the api package inject an adapter at startup — keeping the
dependency one-way (api → plugin). See `builtin/emailtosms/host.go` and
`internal/api/plugin_host.go`.

## External plugins (no rebuild)

Register any HTTP service that answers `GET /manifest`, `GET /health`, and
`POST /invoke`. An external plugin joins the per-user cascade by adding a
`userConfig` block (same shape as `UserConfigSpec`) to its manifest and serving
`POST /user-health` with `{owner, config}` → `{status, detail}`; a plugin that
declares `userConfig` but serves no such route falls back to plain `/health`.
From the panel's **Plugins** card → *Register external*, or:

```bash
curl -X POST http://localhost:8080/api/admin/plugins \
  -H "Authorization: $SUPERADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"acme","baseURL":"http://127.0.0.1:9100","provider":"ACME"}'
```

## Per-user cascade

A plugin can be offered to end users with per-user settings, resolved
**global → org → user** (the top layer wins; a lower layer fills blanks). The
global layer is the plugin's `plugins.json` config (superadmin); the org and
user layers live in a `pluginSettings` JSON field on the `organizations` and
`users` collections. The cascade itself is generic — a plugin only **declares**
what it accepts, by implementing `UserConfigurable` from
[`userconfig.go`](userconfig.go):

```go
type UserConfigurable interface {
    UserConfig() UserConfigSpec
    UserHealthCheck(ctx context.Context, uc UserContext, cfg map[string]string) Health
}
```

`UserConfigSpec` carries a title, a description, the label for the per-user
opt-in, and the fields. Each `UserField` is a `ConfigField` plus:

| Field | Purpose |
|---|---|
| `GlobalKey` | the global-config key that seeds this field (`""` = same key, `NoGlobalKey` = no global layer, for personal credentials) |
| `Group` | fields sharing a group resolve together from one layer, so the halves of a credential are never mixed |
| `MaskWhenInherited` | mask the value toward the client when it came from a layer the caller cannot edit |

Declaring that is all a plugin does: resolution, secret masking, locking,
persistence, the HTTP endpoints and the Web App form all derive from the spec.
`UserHealthCheck` must be self-contained — it is called without `Init`, so it
never disturbs the live instance or its listeners.

`email-to-sms` uses this for per-user IMAP mailboxes — see
[`builtin/emailtosms/userconfig.go`](builtin/emailtosms/userconfig.go) for the
declaration and [`internal/api/integrations.go`](../../internal/api/integrations.go)
for the cascade.

### End-user API

Any authenticated user manages their own layer; an org admin may also edit their
organization's. `{name}` is any plugin declaring per-user settings.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/integrations` | every integration the caller can configure, resolved |
| `GET` | `/api/integrations/{name}` | one integration + its spec |
| `PUT` | `/api/integrations/{name}` | `{enabled?, scope?: "user"\|"org", config?}` |
| `POST` | `/api/integrations/{name}/health` | probe the caller's resolved settings |

## Superadmin API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/admin/plugins` | list all plugins + state + last health |
| `GET` | `/api/admin/plugins/{name}` | one plugin |
| `PUT` | `/api/admin/plugins/{name}` | `{enabled?, config?}` — enable/disable + configure |
| `POST` | `/api/admin/plugins` | `{name, baseURL, provider?}` — register external |
| `DELETE` | `/api/admin/plugins/{name}` | remove an external plugin (built-ins only disable) |
| `POST` | `/api/admin/plugins/{name}/health` | run a health check now |
