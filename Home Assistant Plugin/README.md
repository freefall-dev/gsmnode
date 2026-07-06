# gsmnode — Home Assistant Integration

A custom Home Assistant integration that sends SMS **and places phone calls**
through the gsmnode **API Server**. It can be added and configured entirely
from the Home Assistant UI.

```
Home Assistant ──► API Server (/api/messages, /api/calls) ──► your phone ──► SMS / call
```

Like every other client, it talks **only** to the API Server (never PocketBase).

## What you get

- **UI setup** (config flow) — add it under *Settings → Devices & Services*.
- **Services** — `gsmnode.send_sms` and `gsmnode.call`, with field
  pickers in the automation editor and *Developer Tools → Actions*.
- **Sensor** — `binary_sensor` "API Server" (connectivity) so you can see and
  automate on the gateway being up/down.

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
   - **API Server URL** — e.g. `http://10.2.1.101:8080` (must be reachable from HA)
   - **Email** / **Password** — a gateway user (create one with
     `node "API Server/scripts/create-user.mjs" ha@local "pass" "Home Assistant"`)
   - **Default device ID** *(optional)* — pin sends/calls to a specific phone

   The form validates by logging in; you'll get *Invalid auth* or *Cannot
   connect* if something's wrong.

A **gsmnode** device appears with an **API Server** connectivity sensor.

## Use it

### Send an SMS

```yaml
action: gsmnode.send_sms
data:
  phone_numbers: ["+15551234567"]
  message: "Hello from Home Assistant"
  device_id: my-phone   # optional
  sim_number: 1         # optional (dual-SIM)
```

### Place a call

```yaml
action: gsmnode.call
data:
  phone_number: "+15551234567"
  device_id: my-phone   # optional
```

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

- On first send it logs in (`POST /api/auth/login`) and caches the JWT; on a
  `401` it re-logs in once and retries.
- `send_sms` → `POST /api/messages`; `call` → `POST /api/calls`.
- The sensor polls `GET /api/health` every 30s.
- All HTTP uses Home Assistant's shared aiohttp session (fully async).

## Notes / limitations

- The API Server must be reachable from the Home Assistant host.
- No external dependencies (`requirements: []`).
