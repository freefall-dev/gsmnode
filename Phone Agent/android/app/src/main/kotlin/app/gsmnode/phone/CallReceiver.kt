package app.gsmnode.phone

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper
import android.telephony.TelephonyManager

/// Surfaces incoming and outgoing call events to Dart via SmsBridge.callSink.
///
/// Android reports call state as transitions (RINGING → OFFHOOK → IDLE), not
/// discrete events, so we keep a little state across broadcasts to classify each
/// call: an incoming call that reaches OFFHOOK was answered (→ "completed" on
/// hang-up), one that goes straight to IDLE was "missed". Outgoing calls are
/// caught via NEW_OUTGOING_CALL.
class CallReceiver : BroadcastReceiver() {

    companion object {
        // Cross-broadcast state (a single active call at a time).
        @Volatile private var lastState = TelephonyManager.CALL_STATE_IDLE
        @Volatile private var ringNumber: String? = null
        @Volatile private var outgoingNumber: String? = null
        @Volatile private var wasAnswered = false
        @Volatile private var startedAtMs = 0L
    }

    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            Intent.ACTION_NEW_OUTGOING_CALL -> {
                outgoingNumber = intent.getStringExtra(Intent.EXTRA_PHONE_NUMBER)
            }
            TelephonyManager.ACTION_PHONE_STATE_CHANGED -> handleState(intent)
        }
    }

    private fun handleState(intent: Intent) {
        val stateStr = intent.getStringExtra(TelephonyManager.EXTRA_STATE)
        val number = intent.getStringExtra(TelephonyManager.EXTRA_INCOMING_NUMBER)
        val state = when (stateStr) {
            TelephonyManager.EXTRA_STATE_RINGING -> TelephonyManager.CALL_STATE_RINGING
            TelephonyManager.EXTRA_STATE_OFFHOOK -> TelephonyManager.CALL_STATE_OFFHOOK
            else -> TelephonyManager.CALL_STATE_IDLE
        }
        if (state == lastState) return

        when (state) {
            TelephonyManager.CALL_STATE_RINGING -> {
                ringNumber = number
                wasAnswered = false
                startedAtMs = System.currentTimeMillis()
                emit(number ?: "", "incoming", "ringing", null)
            }
            TelephonyManager.CALL_STATE_OFFHOOK -> {
                wasAnswered = true
                startedAtMs = System.currentTimeMillis()
                // An OFFHOOK not preceded by RINGING is the outgoing call connecting.
                if (lastState != TelephonyManager.CALL_STATE_RINGING && outgoingNumber != null) {
                    emit(outgoingNumber ?: "", "outgoing", "answered", null)
                }
            }
            TelephonyManager.CALL_STATE_IDLE -> {
                val durationSec = if (startedAtMs > 0)
                    ((System.currentTimeMillis() - startedAtMs) / 1000).toInt() else null
                when {
                    outgoingNumber != null -> {
                        emit(outgoingNumber!!, "outgoing", "completed", durationSec)
                    }
                    ringNumber != null && wasAnswered -> {
                        emit(ringNumber!!, "incoming", "completed", durationSec)
                    }
                    ringNumber != null -> {
                        emit(ringNumber!!, "incoming", "missed", null)
                    }
                }
                ringNumber = null
                outgoingNumber = null
                wasAnswered = false
                startedAtMs = 0L
            }
        }
        lastState = state
    }

    private fun emit(number: String, direction: String, status: String, duration: Int?) {
        val payload = HashMap<String, Any?>()
        payload["number"] = number
        payload["direction"] = direction
        payload["status"] = status
        payload["timestamp"] = System.currentTimeMillis()
        if (duration != null) payload["duration"] = duration
        Handler(Looper.getMainLooper()).post {
            SmsBridge.callSink?.success(payload)
        }
    }
}
