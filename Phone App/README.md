# gsmnode — Phone App (Flutter / Android)

Turns an Android phone into the SMS gateway endpoint. It registers with the API
Server, polls for pending outbound messages, sends them over the radio, reports
delivery state, and forwards incoming SMS to the server's inbox.

```
API Server  ──(pending messages)──►  Phone App  ──►  SmsManager (send)
     ▲                                    │
     └──(status reports / inbox)──────────┘  ◄── BroadcastReceiver (incoming)
```

> The Phone App talks **only** to the API Server (never to PocketBase directly),
> using the device token issued at registration.

## Status: source scaffold

This folder contains the complete Dart app and the native Android (Kotlin) SMS
bridge, but **not** the generated Gradle/platform scaffolding. You generate that
once with `flutter create` after installing the toolchain (below).

## Prerequisites

1. **Flutter SDK** (stable) — https://docs.flutter.dev/get-started/install/windows
2. **JDK 17** (this machine currently has only JDK 8 — Android Gradle needs 17+)
3. **Android SDK** (via Android Studio or `flutter doctor --android-licenses`)
4. A **physical Android phone** with a SIM (the emulator can't send real SMS)

Verify with `flutter doctor` — resolve anything it flags before continuing.

## Generate platform scaffolding & wire in the native code

From this `Phone App/` folder:

```powershell
# 1. Generate the android/ Gradle project (keeps lib/ and pubspec.yaml)
flutter create . --org app.smsgateway --project-name sms_gateway_phone --platforms=android

# 2. Overlay the SMS-enabled manifest + Kotlin (overwrites the generated stubs)
Copy-Item -Recurse -Force android_overlay/* android/

# 3. Fetch packages
flutter pub get
```

`android_overlay/` mirrors the real `android/` paths, so step 2 drops:
- `app/src/main/AndroidManifest.xml` — SMS permissions + the `SmsReceiver`
- `app/src/main/kotlin/app/smsgateway/sms_gateway_phone/MainActivity.kt` — send bridge
- `app/src/main/kotlin/app/smsgateway/sms_gateway_phone/SmsReceiver.kt` — incoming bridge

If Gradle complains about SDK levels, set `minSdkVersion 23` (or higher) in
`android/app/build.gradle`.

## Run

```powershell
flutter devices            # confirm your phone is listed
flutter run                # build & install on the connected phone
```

1. On first launch, enter the **API Server URL**, your **email/password**, and a
   **device name**, then tap *Sign in & register device*.
   - Emulator → host: use `http://10.0.2.2:8080`.
   - Physical phone → use the host's LAN IP, e.g. `http://10.2.1.x:8080`
     (the same network as the phone; make sure the API Server is reachable).
2. On the home screen, **grant SMS & phone permissions**, then **Start gateway**.
3. Send a test message from the Web App → it appears in the activity log and is
   delivered via the phone. Texts received by the phone show up in the Web App
   **Inbox**.

## How it maps to the API

| Action | Endpoint |
|---|---|
| Login | `POST /api/auth/login` (JWT) |
| Register device | `POST /api/mobile/v1/device` → device token |
| Poll pending | `GET /api/mobile/v1/messages` (marks them `Processed`) |
| Report state | `PATCH /api/mobile/v1/messages/{id}` (`Sent`/`Failed`) |
| Incoming SMS | `POST /api/mobile/v1/inbox` |
| Heartbeat | `POST /api/mobile/v1/ping` |

Pulled items carry a `type` of `sms` or `call`. For `call` the app places a
native phone call via `TelecomManager.placeCall` (needs `CALL_PHONE` — covered by
the phone permission) instead of sending SMS, then reports `Sent`.
`TelecomManager.placeCall` is used rather than `startActivity(ACTION_CALL)` so the
call still goes through when the screen is locked / the app is backgrounded
(Android blocks background activity starts, but not telecom-routed calls).

## Code layout

```
lib/
  main.dart                 entry, bootstraps services, picks first screen
  config.dart               default API base + poll/ping intervals
  models/message.dart       outbound message model
  services/
    storage.dart            persisted settings/tokens (shared_preferences)
    api_client.dart         API Server HTTP client
    sms_service.dart        platform-channel bridge (send + incoming stream)
    gateway_service.dart    the poll → send → report loop + inbox forwarding
  screens/
    login_screen.dart       login + device registration
    home_screen.dart        start/stop, permissions, activity log
android_overlay/            native Android files to copy after `flutter create`
```

## Background & delivery reports (implemented)

- **Foreground service** (`GatewayForegroundService.kt`): starting the gateway
  launches an ongoing-notification foreground service + partial wakelock, so the
  poll/send loop keeps running while the screen is off or the app is backgrounded.
- **Delivery reports**: `MainActivity.sendSms` attaches `sent`/`delivered`
  PendingIntents; `SmsStatusReceiver` forwards the outcome (tagged with the
  message id) to Dart, which reports `Delivered`/`Failed` to the API Server.

## Multiple SIM cards

On dual-SIM devices the app enumerates the active SIMs (needs `READ_PHONE_STATE`,
covered by the phone permission group) and reports them to the server on each
heartbeat, so the Web App / API can offer real slot choices.

- **Sending on a specific SIM:** the pulled message's `sim_number` (0-based slot)
  selects the SIM. If that slot has no active subscription, the send is
  **rejected** and reported `Failed` rather than silently using the default SIM.
  A message with no `sim_number` uses the device's default SIM.
- **Incoming SMS:** the receiver records which slot a message arrived on and
  forwards it as `sim_slot` to the inbox.

## Known next steps (not yet implemented)

- **Survive full task removal / long Doze**: the foreground service covers
  screen-lock, but surviving the user swiping the app away or hours of Doze would
  need a dedicated background Dart isolate (e.g. `flutter_background_service`).
- **Push wake-up (FCM)**: register an FCM token at registration so the server can
  wake the device instead of polling. Requires a Firebase project +
  `google-services.json` + server-side FCM sending in the API Server.
- **MMS / data SMS**: only text SMS is implemented.
