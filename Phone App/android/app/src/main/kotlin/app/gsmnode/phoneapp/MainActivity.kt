package app.gsmnode.phoneapp

import io.flutter.embedding.android.FlutterFragmentActivity

// FlutterFragmentActivity, not FlutterActivity: the app lock's BiometricPrompt
// (androidx.biometric, via local_auth) is a fragment and needs a FragmentActivity
// to attach to. Everything else about the host activity is the template default.
class MainActivity : FlutterFragmentActivity()
