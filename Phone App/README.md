# gsmnode — Phone App (Flutter / Android)

The gateway console on your phone: a **mobile mirror of the Web App**. Same
screens, same API, same design system — Devices, Send, Calls, Messages, Inbox,
Webhooks and Settings (including the Users / Organizations / Integrations
administration).

```
Phone App ──► API Server (:8080) ──► PocketBase
```

> **Not the same thing as [`../Phone Agent/`](../Phone%20Agent/).** The Phone
> Agent *controls the phone* — it sends and receives SMS/MMS and places calls on
> behalf of the gateway. This app *controls the gateway*: it is the Web App's UI,
> on a phone. The two are installed separately and can sit side by side on one
> device.

Like every other surface, it talks **only** to the API Server, never to
PocketBase.

## Prerequisites

1. **Flutter SDK** (stable) — https://docs.flutter.dev/get-started/install/windows
2. **JDK 17+**
3. **Android SDK** (via Android Studio or `flutter doctor --android-licenses`)
4. A running **API Server** reachable from the phone

## Set up & run

```powershell
flutter pub get
flutter devices                # confirm your phone or emulator is listed
flutter run
```

The Gradle **wrapper** is not committed. If `gradlew` / `gradle-wrapper.jar` are
missing, regenerate the platform files in place — it leaves `lib/` and
`pubspec.yaml` alone:

```powershell
flutter create . --org app.gsmnode --project-name phoneapp --platforms=android
```

> The application id is **`app.gsmnode.phoneapp`**, deliberately distinct from the
> Phone Agent's `app.gsmnode.phoneagent` so both can be installed at once. It is
> derived from `--org` + `--project-name` at create time, and the Dart package name
> in `pubspec.yaml` is `phoneapp` to match, so the command above reproduces both.
> Don't rename either: it changes the installed app's identity, and Android treats a
> new id as a different app rather than an upgrade.

On first launch, open **Server settings** on the sign-in screen and enter the API
Server URL, then sign in with the user you created via
`../API Server/scripts/create-user.mjs`.

- Emulator → host: `http://10.0.2.2:8080`
- Physical phone → the host's LAN IP, e.g. `http://192.168.1.50:8080`

The URL is remembered, and can be changed later under **Settings → Server**.

## Screens

Navigation lives in the drawer (the Web App's sidebar doesn't fit a phone); the
order and grouping are carried over unchanged, API-status footer included.

- **Devices** — registered phones with live online/offline status, SIM slots and
  last-seen; remove a device. Refreshes itself every 10s, because the status is
  heartbeat-derived and changes with nobody looking.
- **Send SMS** — queue an SMS, data SMS or MMS: multiple recipients, device and
  SIM slot, optional end-to-end encryption. MMS attachments are picked from the
  phone's photo/video library.
- **Calls** — place a call on a device, and browse the call log filtered by
  direction.
- **Messages** — outbound history with live status, filterable by status.
- **Inbox** — incoming SMS / data SMS / MMS, tabbed by type with counts. Inline
  previews for image attachments and decoded data-SMS payloads.
- **Webhooks** — register and delete callbacks for the gateway's events.
- **Settings** — server URL, display name, **app lock**, E2E passphrase, password
  change, theme (light / dark / system), and — for managers — Users and
  Organizations, plus the schema-driven **Integrations** forms.

## App lock (face / fingerprint)

**Settings → App lock** puts the phone's biometrics in front of a signed-in
session. The session itself is unaffected — the JWT is persisted either way, and
that is precisely the point: without a lock, anyone holding an unlocked phone has
the gateway.

This is [the Phone Agent's App lock](../Phone%20Agent/README.md#app-lock-face--fingerprint),
carried over rather than reinvented — same `AppLockController` / `BiometricService` /
`AppLockGate` split, same `app_lock` preference key, same lifecycle rules, same
`local_auth` major. The two surfaces differ only where a console differs from a
gateway: the Agent arms on a *registered device* and keeps routing SMS behind its
lock, this one arms on a *signed-in session* and has only the screen to guard.

- It closes on a cold start, whenever the app has been in the background for
  `AppConfig.appLockGrace` (30s), and **always on a close** — a detach is a
  deliberate exit, where the grace period is only meant to absorb an
  interruption like the photo picker on an MMS.
- The prompt gates the switch in **both directions**: turning it on proves the
  lock can be cleared before arming it, and turning it off stops someone holding
  an already-unlocked phone from quietly disarming it.
- The prompt is Android's `BiometricPrompt`, so the **screen lock (PIN, pattern,
  password) is its own fallback** — a wet finger is not a lockout. It is not
  `biometricOnly`.
- A phone that can no longer prompt *at all* (biometrics gone and the screen lock
  removed) disarms the lock and lets its owner back in. Otherwise the gate is a
  one-way door: unlocking is impossible, and so is switching it off.
- The gate is drawn *over* the navigator (`MaterialApp.builder`), not routed to:
  it covers open dialogs and pushed routes, and the shell underneath is never
  torn down, so a half-written SMS survives the phone being pocketed.
- **Sign out instead** is offered on the lock screen, which the Agent's does not:
  a console sign-out costs a password, where the Agent's would un-register the
  phone. It is confirmed inline, there being no navigator above the gate to push
  a dialog onto.
- The preference survives a sign-out, like the server URL and the passphrase —
  it is device setup, and leaving it armed is the safer default.

It guards the running app, not the screenshot Android takes of it: the app
switcher's thumbnail is captured on the way out, before the lock closes, so the
last screen is still visible there. Hiding it would mean `FLAG_SECURE` on the
activity (and a platform channel to toggle it with the setting) — not done.

`widgets/app_lock_gate.dart` takes an injectable clock, so `test/widget_test.dart`
covers the grace window, the detach rule and the stranded-phone escape by driving
the lifecycle against a fake prompt — none of it needs a device.

## End-to-end encryption

Set the passphrase under **Settings → End-to-end encryption**. It is stored on
this device only and never sent anywhere. `services/crypto_service.dart`
implements the same scheme as the Web App's `crypto.js` and the Phone Agent's
`crypto_service.dart` — PBKDF2-HMAC-SHA256 (150 000 iterations) into AES-256-GCM,
wrapped as `gsmenc:v1:` + base64(salt‖iv‖ct) — so the three surfaces interoperate.
Enter the *same* passphrase everywhere that must read the messages.

Message text and recipient numbers are encrypted; MMS subjects, MMS attachments
and data-SMS payloads deliberately are not, matching the other two surfaces.

## Code layout

```
lib/
  main.dart                     entry, bootstraps services, picks first screen
  config.dart                   default API base + poll intervals
  theme.dart                    gsmnode design tokens + Material theme
  services/
    storage.dart                persisted settings/session (shared_preferences)
    api_client.dart             API Server HTTP client (/api, bearer token)
    auth_store.dart             login state + the signed-in user
    crypto_service.dart         AES-256-GCM + PBKDF2 (matches the Web App)
    theme_controller.dart       light/dark/system preference
    biometric_service.dart      face/fingerprint prompts (the Agent's, verbatim)
    app_lock.dart               the App lock preference, armed by a prompt
  widgets/                      design-system pieces (cards, badges, selects…)
    app_lock_gate.dart          the lock overlay + its lifecycle rules
  screens/
    login_screen.dart           sign in + server URL
    home_shell.dart             drawer navigation + app bar
    …_screen.dart               one per Web App page
    settings/                   users, organizations, integrations
```

## How it maps to the API

Every screen is a straight port of a Web App view against the same endpoints:

| Screen | Endpoints |
|---|---|
| Login | `POST /api/auth/login` |
| Devices | `GET /api/devices`, `DELETE /api/devices/{id}` |
| Send SMS | `GET /api/devices`, `POST /api/messages` |
| Calls | `GET`/`POST /api/calls` |
| Messages | `GET /api/messages?status=` |
| Inbox | `GET /api/inbox` |
| Webhooks | `GET`/`POST /api/webhooks`, `DELETE /api/webhooks/{id}` |
| Settings | `PATCH /api/auth/me`, `POST /api/auth/change-password` |
| Users | `GET`/`POST /api/users`, `PATCH`/`DELETE /api/users/{id}` |
| Organizations | `GET`/`POST /api/orgs`, `PATCH`/`DELETE /api/orgs/{id}` |
| Integrations | `GET`/`PUT /api/integrations[/{name}]`, `POST …/health` |
| API status dot | `GET /api/health` every 10s |

## Build notes

Two Android settings in `android/gradle.properties` are load-bearing here:

- `kotlin.incremental=false` — Kotlin's incremental compiler stores source paths
  relative to the project and throws when a plugin's sources live on another
  drive, which is the case on Windows whenever the pub cache and the checkout sit
  on different drive letters.
- `android.builtInKotlin=false` (the Flutter template default) — needed by the
  plugins in use. It also rules out `file_picker`, whose current release skips
  applying the Kotlin Gradle Plugin under AGP 9 and then fails to link; hence
  `image_picker` for MMS attachments.

`INTERNET` is declared in the main manifest, not just the debug one — the app is
nothing but an API client, so release builds need it too. `USE_BIOMETRIC` joins
it for the app lock; neither is a runtime permission.

`MainActivity` extends **`FlutterFragmentActivity`**, not `FlutterActivity`:
`BiometricPrompt` is a fragment and needs a `FragmentActivity` host. Swapping it
back turns every unlock attempt into a crash.

## Known gaps vs the Web App

- **MMS attachments are media-only.** The browser's file input takes anything;
  here you pick from the photo/video library. See the build note above.
- **Inbox attachments are viewed, not downloaded.** Images render inline;
  anything else is listed by name. There is no "save to device" yet.
- **The superadmin Plugins panel is not mirrored** — it lives in the API Server's
  own panel, and the Web App doesn't carry it either.
