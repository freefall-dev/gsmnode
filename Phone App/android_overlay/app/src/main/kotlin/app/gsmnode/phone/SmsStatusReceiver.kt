package app.gsmnode.phone

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper

/// Receives SMS send/delivery outcomes from the PendingIntents created in
/// MainActivity.sendSms and forwards them to Dart, tagged with the message id
/// so the gateway loop can report Sent / Delivered / Failed to the API Server.
class SmsStatusReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val messageId = intent.getStringExtra("messageId")
        val kind = intent.getStringExtra("kind") ?: return
        // For "sent": RESULT_OK means the radio accepted the message.
        // For "delivered": RESULT_OK means a positive delivery report arrived.
        val success = resultCode == Activity.RESULT_OK

        val payload = mapOf(
            "messageId" to messageId,
            "kind" to kind,
            "success" to success,
            "resultCode" to resultCode,
        )

        Handler(Looper.getMainLooper()).post {
            MainActivity.statusSink?.success(payload)
        }
    }
}
