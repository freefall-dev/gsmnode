package app.gsmnode.phone

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper
import android.provider.Telephony

/// Receives incoming SMS and forwards each message to Dart via the EventChannel
/// sink held by MainActivity. Works while the app process is alive.
class SmsReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Telephony.Sms.Intents.SMS_RECEIVED_ACTION) return

        val messages = Telephony.Sms.Intents.getMessagesFromIntent(intent) ?: return
        if (messages.isEmpty()) return

        // Multipart SMS arrive as several PDUs from the same sender; concatenate.
        val from = messages[0].displayOriginatingAddress ?: ""
        val body = StringBuilder()
        var timestamp = System.currentTimeMillis()
        for (m in messages) {
            body.append(m.displayMessageBody ?: "")
            timestamp = m.timestampMillis
        }

        val payload = mapOf(
            "from" to from,
            "body" to body.toString(),
            "timestamp" to timestamp,
        )

        // EventSink must be touched on the main thread.
        Handler(Looper.getMainLooper()).post {
            MainActivity.incomingSink?.success(payload)
        }
    }
}
