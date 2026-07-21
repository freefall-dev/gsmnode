package app.gsmnode.phoneagent

import android.app.Application
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.embedding.engine.FlutterEngineCache
import io.flutter.embedding.engine.dart.DartExecutor
import io.flutter.plugins.GeneratedPluginRegistrant

/// Owns the Flutter engine for the whole process, instead of letting the
/// activity own it.
///
/// A plain FlutterActivity creates its engine and destroys it again in
/// onDestroy, which took the Dart gateway loop down with it every time the app
/// was swiped out of Recents — while GatewayForegroundService (a separate,
/// START_STICKY component) kept its "gateway is running" notification up. The
/// gateway looked alive and had actually stopped routing.
///
/// Caching the engine here decouples the two: the activity attaches to and
/// detaches from an engine that outlives it, the foreground service keeps the
/// process (and so the engine) alive, and re-opening the app reattaches to the
/// same isolate — so the UI shows the true state with no extra syncing.
class GsmNodeApplication : Application() {

    companion object {
        const val ENGINE_ID = "gsmnode_gateway_engine"
    }

    override fun onCreate() {
        super.onCreate()

        val engine = FlutterEngine(this)
        // Both registrations happen before the entrypoint runs, so Dart never
        // races ahead of its own channels on a cold start.
        GeneratedPluginRegistrant.registerWith(engine)
        SmsBridge(applicationContext).attach(engine)
        engine.dartExecutor.executeDartEntrypoint(DartExecutor.DartEntrypoint.createDefault())
        FlutterEngineCache.getInstance().put(ENGINE_ID, engine)
    }
}
