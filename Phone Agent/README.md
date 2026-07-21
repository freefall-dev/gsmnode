# gsmnode — Phone Agent (Flutter / Android)

Turns an Android phone into the SMS gateway endpoint. It registers with the API
Server, polls for pending outbound messages, sends them over the radio, reports
delivery state, and forwards incoming SMS to the server's inbox.

```
API Server  ──(pending messages)──►  Phone Agent  ──►  SmsManager (send)
     ▲                                     │
     └──(status reports / inbox)───────────┘  ◄── BroadcastReceiver (incoming)
```

> The Phone Agent talks **only** to the API Server (never to PocketBase directly),
> using the device token issued at registration.

> **Not to be confused with the `Phone App/` folder** — that is a separate,
> not-yet-started surface that will mirror the Web App. This one *controls the
> phone* (SMS/MMS + calls).

## Prerequisites

1. **Flutter SDK** (stable) — https://docs.flutter.dev/get-started/install/windows
2. **JDK 17+** — Android Gradle will not build on JDK 8
3. **Android SDK** (via Android Studio or `flutter doctor --android-licenses`)
4. A **physical Android phone** with a SIM (the emulator can't send real SMS)

Verify with `flutter doctor` — resolve anything it flags before continuing.

## Set up

The Dart app *and* the Android Gradle project are both committed, so a fresh
clone only needs packages:

```powershell
flutter pub get
```

The Gradle **wrapper** is not committed (`gradlew`, `gradlew.bat`,
`gradle/wrapper/gradle-wrapper.jar`). If those are missing, regenerate the
platform files in place — it leaves `lib/` and `pubspec.yaml` alone:

```powershell
flutter create . --org app.gsmnode --project-name phoneagent --platforms=android
```

> The application id is **`app.gsmnode.phoneagent`** (`namespace` + `applicationId` in
> `android/app/build.gradle`, and the Kotlin package under
> `android/app/src/main/kotlin/app/gsmnode/phoneagent/`). The Dart package name is
> `sms_gateway_phone`, which is deliberately different — don't "fix" one to match
> the other, and don't rename either: it changes the installed app's identity.

`flutter create` overwrites the generated stubs, so re-apply the native bridge
afterwards. `android_overlay/` mirrors the real `android/` paths for exactly
this — it is a byte-identical copy of the Kotlin and manifest under `android/`:

```powershell
Copy-Item -Recurse -Force android_overlay/* android/
```

It carries `app/src/main/AndroidManifest.xml` (SMS/MMS/call permissions and the
receivers) plus the Kotlin bridge in `app/src/main/kotlin/app/gsmnode/phoneagent/`:
`MainActivity.kt` (send), `SmsReceiver.kt` / `MmsReceiver.kt` / `CallReceiver.kt`
(inbound), `SmsStatusReceiver.kt` (delivery reports), `MmsPduBuilder.kt` and
`GatewayForegroundService.kt`.

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

Scheduling is the server's job: `GET /messages` withholds anything whose
`schedule_at` is still in the future, so the gateway sends what it is handed. It
does check `schedule_at` and park an early message until it comes due, but only
as a backstop against an older server — a parked message is already marked
`Processed`, so if the app dies first the server's expiry sweeper fails it.

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

## Data SMS, MMS, calls & encryption

Beyond text SMS the gateway now also handles:

- **Data SMS** — `SmsManager.sendDataMessage` for outbound; a `DATA_SMS_RECEIVED`
  filter on `SmsReceiver` for inbound (payload forwarded base64 + port).
- **MMS** — best-effort `SmsManager.sendMultimediaMessage` with an M-Send.req PDU
  composed by `MmsPduBuilder.kt` (shared with the platform via a `FileProvider`);
  `MmsReceiver` reports inbound MMS notifications (WAP push). **Caveats:** real MMS
  delivery depends on the carrier MMSC/APN, and fetching a full inbound MMS body +
  attachments is a separate MMSC download a non-default SMS app can't reliably do,
  so inbound MMS is reported as a notification (sender + subject) without
  attachments.
- **Incoming/outgoing calls** — `CallReceiver` watches `PHONE_STATE` and reports
  ringing/answered/missed/completed to the server's call log. `NEW_OUTGOING_CALL`
  is deprecated on Android 10+ and only fires for the default dialer, so outgoing
  calls are logged primarily via the message pipeline the gateway itself uses.
- **End-to-end encryption** — enter a passphrase at login; `crypto_service.dart`
  (AES-256-GCM + PBKDF2, matching the Web App) decrypts outbound messages before
  they hit the radio and encrypts inbound before forwarding them.

New runtime permissions to grant on device: **RECEIVE_MMS/RECEIVE_WAP_PUSH**,
**READ_CALL_LOG**, and (for the call state) **READ_PHONE_STATE**. These are
declared in the manifest; grant them alongside SMS/phone on first run.

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
  wake the device instead of polling. `registerDevice` already accepts a
  `push_token`, but nothing populates it. Requires a Firebase project +
  `google-services.json` + server-side FCM sending in the API Server.
- **Per-recipient delivery state**: a message with several recipients is one
  server-side record, so all of its send/delivery callbacks report against the
  same id and the last one wins. Mixed outcomes across recipients are therefore
  not represented faithfully.
