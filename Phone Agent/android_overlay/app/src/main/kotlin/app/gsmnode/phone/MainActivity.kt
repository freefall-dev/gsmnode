package app.gsmnode.phone

import io.flutter.embedding.android.FlutterFragmentActivity

// FlutterFragmentActivity (not FlutterActivity): local_auth drives androidx
// BiometricPrompt, which needs a FragmentActivity host to show its sheet.
//
// The activity is deliberately thin. It attaches to the process-wide engine
// cached by GsmNodeApplication rather than creating one of its own, so closing
// the app detaches the UI without tearing down the Dart gateway loop. The SMS,
// MMS and call channels live in SmsBridge, bound to the application context —
// see GsmNodeApplication for why.
class MainActivity : FlutterFragmentActivity() {

    override fun getCachedEngineId(): String = GsmNodeApplication.ENGINE_ID

    // The cached engine belongs to the process, not to this activity.
    override fun shouldDestroyEngineWithHost(): Boolean = false
}
