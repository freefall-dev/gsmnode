# gsmnode — Home Assistant Integration

A custom Home Assistant integration that sends SMS **and places phone calls**
through the gsmnode **API Server**. It can be added and configured entirely
from the Home Assistant UI.

```
Home Assistant ──► API Server (/api/messages, /api/calls) ──► your phone ──► SMS / call
```

Like every other client, it talks **only** to the API Server (never PocketBase).

## What you get

- **UI setup** (config flow) — add it under *Settings → Devices & Services*, and
  change the URL or password later without losing your entities (*Reconfigure*).
- **Services** — `gsmnode.send_sms` and `gsmnode.call`, with field
  pickers in the automation editor and *Developer Tools → Actions*.
- **Sensors** — a `binary_sensor` "API Server" (is the gateway up?) plus one
  connectivity sensor **per registered phone**, so an automation can react to
  the phone that actually sends your texts dropping off.

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
automation:
  - alias: "Water leak — text then call"
    trigger:
      - platform: state
        entity_id: binary_sensor.basement_leak
        to: "on"
    action:
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
automation:
  - alias: "Alert if SMS gateway API is down"
    trigger:
      - platform: state
        entity_id: binary_sensor.gsmnode_api_server
        to: "off"
        for: "00:02:00"
    action:
      - action: persistent_notification.create
        data:
          title: "gsmnode"
          message: "The API Server is unreachable."
```

The same trigger shape works on a phone's own sensor (its entity id comes from
the phone's name) — useful when the API Server is up but the phone that does the
sending has stopped routing.

## Receiving SMS in Home Assistant

To **receive** incoming texts, register the API Server's `sms:received` webhook
against a HA webhook trigger. A ready-to-use snippet is in
[`configuration.example.yaml`](configuration.example.yaml).

## Legacy YAML `notify` platform (optional)

A `notify.gsmnode` service is still available for those who prefer YAML
(`notify.py`). It's independent of the UI integration — see
[`configuration.example.yaml`](configuration.example.yaml). For new setups, the
UI integration above is recommended.

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
