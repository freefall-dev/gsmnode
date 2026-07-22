# gsmnode — Home Assistant Integration

A custom Home Assistant integration that sends SMS, places phone calls, and
hears back from the gateway when messages and calls arrive — through the gsmnode
**API Server**.

```
Home Assistant ──► API Server (/api/messages, /api/calls) ──► your phone ──► SMS / call
Home Assistant ◄── API Server (webhooks)                  ◄── your phone ◄── SMS / call
```

Like every other client, it talks **only** to the API Server (never PocketBase).

**There is no YAML.** Every part of this — the connection, the sidebar item,
which events arrive, and who the notify entity texts — is set from the Home
Assistant UI, and nothing goes in `configuration.yaml`.

## What you get

- **UI setup** (config flow) — add it under *Settings → Devices & Services*, and
  change the URL or password later without losing your entities (*Reconfigure*).
- **Services** — `gsmnode.send_sms` and `gsmnode.call`, with field
  pickers in the automation editor and *Developer Tools → Actions*.
- **Sensors** — a `binary_sensor` "API Server" (is the gateway up?) plus one
  connectivity sensor **per registered phone**, so an automation can react to
  the phone that actually sends your texts dropping off.
- **Incoming events** *(optional)* — tick the gateway events you care about and
  they arrive on the Home Assistant bus, ready for an Event trigger. The webhook
  is registered at both ends for you. See below.
- **Notify entity** *(optional)* — `notify.send_message` support for alerts and
  blueprints, texting a set of numbers you configure.
- **Sidebar item** *(optional)* — a **gsmnode** entry in Home Assistant's left
  menu that opens the Web App or the API Server panel in place. See below.
- **Branding** — the gsmnode mark and wordmark ship in `brand/`, so the
  integration carries its own icon instead of a puzzle piece (Home Assistant
  **2026.3+**; older versions ignore the folder). The two actions get icons on
  any version through `icons.json`.

## Install

1. Copy the integration folder into your Home Assistant config directory so it
   lands at:

   ```
   <config>/custom_components/gsmnode/
   ```

   (Copy this repo's `custom_components/gsmnode/` next to your
   `configuration.yaml`. Use the Samba / File editor / SSH add-on, or the
   mapped volume for Docker.)

2. **Restart Home Assistant** (Settings → System → Restart).

## Add it from the UI

1. **Settings → Devices & Services → Add Integration**.
2. Search for **gsmnode**.
3. Fill in the form:
   - **API Server URL** — e.g. `http://10.2.1.10:8080` (must be reachable from HA)
   - **Email** / **Password** — a gateway user (create one with
     `node "API Server/scripts/create-user.mjs" ha@local "pass" "Home Assistant"`)
   - **Default phone** *(optional)* — pin sends/calls to a specific phone, by its
     device ID

   The form validates by logging in; you'll get *Invalid auth* or *Cannot
   connect* if something's wrong.

A **gsmnode** device appears with an **API Server** connectivity sensor, and
each phone registered to that account gets its own device and sensor beneath it.

If the password changes on the gateway, Home Assistant raises a
**Reauthentication** notification instead of silently failing; if the server
moves, use **Reconfigure** on the integration.

## Use it

### Send an SMS

```yaml
action: gsmnode.send_sms
data:
  phone_numbers: ["+15551234567"]
  message: "Hello from Home Assistant"
  device_id: my-phone   # optional
  sim_number: 0         # optional (dual-SIM; slots count from 0)
  schedule_at: "2026-07-22 18:30:00"  # optional (send later)
```

### Place a call

```yaml
action: gsmnode.call
data:
  phone_number: "+15551234567"
  device_id: my-phone   # optional
```

With more than one gateway configured, add `config_entry_id:` to say which one
to use — the automation editor offers a picker for it.

### Example automation

```yaml
# Built in the automation editor; this is only what it looks like underneath.
triggers:
  - trigger: state
    entity_id: binary_sensor.basement_leak
    to: "on"
actions:
  - action: gsmnode.send_sms
    data:
      phone_numbers: ["+15551234567"]
      message: "Water leak detected in the basement!"
  - action: gsmnode.call
    data:
      phone_number: "+15551234567"
```

### React to the gateway going offline

```yaml
triggers:
  - trigger: state
    entity_id: binary_sensor.gsmnode_api_server
    to: "off"
    for: "00:02:00"
actions:
  - action: persistent_notification.create
    data:
      title: "gsmnode"
      message: "The API Server is unreachable."
```

The same trigger shape works on a phone's own sensor (its entity id comes from
the phone's name) — useful when the API Server is up but the phone that does the
sending has stopped routing.

## The sidebar item

The gateway already has two full overviews — the Web App and the API Server's
own panel — so the integration does not reimplement either. It puts an item in
Home Assistant's sidebar that opens the one you choose, in place, without
leaving Home Assistant.

Set it up under **Settings → Devices & Services → gsmnode → Configure →
Sidebar item**:

| Overview to show | Opens |
|---|---|
| **No sidebar item** | nothing — the default |
| **Web App** | the full gateway UI: messages, inbox, devices, webhooks, settings |
| **API Server panel** | status and administration: users, plugins, connected devices |
| **Another address** | any URL you give it |

The address is worked out for you: the API Server panel is the URL you signed in
against, and the Web App's is whatever the API Server reports for it on
`/api/status` — both shown in the form. Fill the **Address** field in only when
that is not the address your *browser* needs. The panel is loaded by the
browser, not by Home Assistant, so a container name or a `localhost` that means
something different on the two machines has to be overridden here.

**Sidebar title** names the item — worth setting if you run more than one
gateway — and **Administrators only** hides it from non-admin users. Changing
any of this reloads the integration and the sidebar updates immediately.

Two limits worth knowing before you wonder why a page is blank:

- A page served over **http cannot be embedded in a Home Assistant served over
  https**. Browsers block the mixed content and Home Assistant shows an error in
  place of the frame. Put the gateway behind the same kind of TLS Home Assistant
  uses, or open it in its own tab.
- The embedded app keeps **its own login**. Signing in to Home Assistant does
  not sign you in to the Web App; that session lives in the frame and persists
  there like it would in a tab.

## Incoming events

Sending is only half a gateway. **Configure → Incoming events** ticks what the
gateway should tell Home Assistant about:

| Event | Fires as | Carries |
|---|---|---|
| `sms:received` | `gsmnode_sms_received` | `phone_number`, `message`, `received_at`, `sim_slot`, `encrypted` |
| `sms:sent` · `sms:delivered` · `sms:failed` | `gsmnode_sms_sent` … | `message_id`, `phone_numbers`, `status`, `error` |
| `sms:data-received` | `gsmnode_sms_data_received` | the above plus `data_payload`, `data_port` |
| `mms:received` · `mms:downloaded` | `gsmnode_mms_received` … | the above plus `subject`, `attachments` |
| `call:received` · `call:sent` · `call:failed` | `gsmnode_call_received` … | `call_id`, `phone_number`, `direction`, `status`, `started_at`, `duration` |

Nothing has to be registered by hand at either end. Home Assistant mints a
webhook of its own when the entry is created, and the integration subscribes
that URL with the API Server for exactly the ticked events — unticking removes
the subscription again, and deleting the integration unsubscribes it entirely.
Only subscriptions carrying this Home Assistant's webhook id are touched, so
anything you registered yourself in the Web App is left alone.

To use one, build an automation (**Settings → Automations → Create**), choose
the **Event** trigger, and give it the event type from the table — the options
form prints the exact names your current selection produces. Every event also
carries `device_id` and `created_at`, and the payload is flattened to the top
level, so a template reads `trigger.event.data.phone_number` directly:

```yaml
# What the UI editor produces — you never have to write this by hand.
triggers:
  - trigger: event
    event_type: gsmnode_sms_received
actions:
  - action: persistent_notification.create
    data:
      title: "SMS from {{ trigger.event.data.phone_number }}"
      message: "{{ trigger.event.data.message }}"
```

Two things to know:

- The gateway has to be able to **reach Home Assistant**. The address used is
  the one Home Assistant knows itself by (internal first, external as a
  fallback); if the gateway needs a different one, set it in the same form.
- With **end-to-end encryption** on, `phone_number` and `message` arrive as
  `gsmenc:v1:…` ciphertext. Home Assistant has no passphrase and cannot read
  them — this integration does not do E2E.

## Notify entity

**Configure → Notify entity** takes a list of numbers and creates a notify
entity that texts them, so anything speaking `notify.send_message` — alerts,
blueprints, scripts — can reach the gateway:

```yaml
action: notify.send_message
target:
  entity_id: notify.gsmnode_sms
data:
  message: "Washing machine finished"
```

A notify entity has no recipient field of its own, which is why the numbers are
fixed here. Anything that needs to pick recipients, a phone or a SIM per message
wants `gsmnode.send_sms` instead. Leave the list empty and no entity is created.

> Replaced the old YAML `notify.gsmnode` platform in 3.0.0. If you had one in
> `configuration.yaml`, delete that block and set the numbers here; automations
> calling `notify.gsmnode` with a `target:` should move to `gsmnode.send_sms`,
> which takes recipients per call.

## How it works

- On first use it logs in (`POST /api/auth/login`) and caches the token; on a
  `401` it re-logs in once and retries, and gives up into a reauth prompt if
  that fails too.
- `send_sms` → `POST /api/messages`; `call` → `POST /api/calls`.
- Every 30s one coordinator polls `GET /api/health` and `GET /api/devices` and
  feeds both the gateway sensor and the per-phone ones. An unreachable server
  turns the sensor **off** rather than making it *unavailable* — reporting that
  is the sensor's whole job.
- All HTTP uses Home Assistant's shared aiohttp session (fully async), with a
  15s timeout per request.
- The sidebar item is an iframe panel (`frontend.async_register_built_in_panel`)
  registered when the entry loads and removed when it unloads. Nothing is
  proxied through Home Assistant — the browser fetches the page straight from
  the gateway.
- Incoming events use Home Assistant's own webhook component. The id is minted
  once per entry and stored with it, the URL is `GET`-proof (`POST` only), and
  each delivery is re-fired on the bus. Subscriptions on the gateway are
  reconciled on every load, so the set on the server always matches the ticks in
  the form.

## Brand assets

`brand/` holds what Home Assistant shows for the integration itself, in the
layout and sizes its [brands repository](https://github.com/home-assistant/brands)
requires — a local `brand/` folder wins over the CDN, so nothing has to be
submitted upstream:

| File | Source (design kit) | Size |
|---|---|---|
| `icon.png` | `app-icon-512.png`, scaled | 256×256 |
| `icon@2x.png` | `app-icon-512.png` | 512×512 |
| `logo.png` | `lockup-horizontal-color.png` | 936×224 |
| `dark_logo.png` | `lockup-horizontal-white.png` | 936×224 |

The icon is the app tile — signal-green with white arrows — which stays legible
on either theme, so no `dark_icon.png` is needed. The wordmark does need both:
`logo.png` sets "gsm" in ink, `dark_logo.png` in white. There is no
`logo@2x.png`; the widest lockup in the kit is 936×224, and its short side would
have to be upscaled to reach the hDPI range.

## Notes / limitations

- The API Server must be reachable from the Home Assistant host.
- No external dependencies (`requirements: []`).
- SIM slots are **0-based** here and everywhere else in gsmnode: slot `0` is the
  first SIM. (A phone's sensor lists its slots and carriers in its attributes.)
- Only plain SMS and calls are wired up. The API Server also accepts **MMS**
  (`type: mms` with subject/attachments) and **data SMS** (`type: data`); those
  have no service here yet.
- **End-to-end encryption is not supported.** Messages sent from here are
  plaintext and flagged as such, so a Phone Agent with a passphrase still sends
  them fine — but the API Server can read them, unlike ones composed in the Web
  App. Incoming SMS relayed to a HA webhook likewise arrive as ciphertext if the
  Phone Agent encrypted them.
