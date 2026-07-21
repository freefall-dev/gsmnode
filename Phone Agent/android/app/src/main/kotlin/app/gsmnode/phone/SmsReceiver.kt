package app.gsmnode.phone

import android.Manifest
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Handler
import android.os.Looper
import android.provider.Telephony
import android.telephony.SubscriptionManager
import androidx.core.content.ContextCompat

/// Receives incoming SMS and forwards each message to Dart via the EventChannel
/// sink held by SmsBridge. Works while the app process is alive.
class SmsReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            Telephony.Sms.Intents.SMS_RECEIVED_ACTION -> handleTextSms(context, intent)
            Telephony.Sms.Intents.DATA_SMS_RECEIVED_ACTION -> handleDataSms(context, intent)
            else -> return
        }
    }

    private fun handleTextSms(context: Context, intent: Intent) {
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

        emit(
            mapOf(
                "type" to "sms",
                "from" to from,
                "body" to body.toString(),
                "timestamp" to timestamp,
                // 0-based physical slot the message arrived on, or -1 if unknown
                // (single-SIM device, or READ_PHONE_STATE not granted).
                "simSlot" to receivingSlot(context, intent),
            )
        )
    }

    /// Handles a binary data SMS: concatenates the PDUs' raw bytes and forwards
    /// them base64-encoded, tagged with the destination port.
    private fun handleDataSms(context: Context, intent: Intent) {
        val messages = Telephony.Sms.Intents.getMessagesFromIntent(intent) ?: return
        if (messages.isEmpty()) return

        val from = messages[0].displayOriginatingAddress ?: ""
        var timestamp = System.currentTimeMillis()
        val bytes = java.io.ByteArrayOutputStream()
        for (m in messages) {
            m.userData?.let { bytes.write(it) }
            timestamp = m.timestampMillis
        }
        val port = intent.data?.port ?: -1
        val payloadB64 = android.util.Base64.encodeToString(
            bytes.toByteArray(), android.util.Base64.NO_WRAP
        )

        emit(
            mapOf(
                "type" to "data",
                "from" to from,
                "body" to "",
                "dataPayload" to payloadB64,
                "dataPort" to port,
                "timestamp" to timestamp,
                "simSlot" to receivingSlot(context, intent),
            )
        )
    }

    private fun emit(payload: Map<String, Any?>) {
        // EventSink must be touched on the main thread.
        Handler(Looper.getMainLooper()).post {
            SmsBridge.incomingSink?.success(payload)
        }
    }

    /// Resolves which SIM slot an incoming SMS arrived on. The broadcast carries a
    /// subscription id (under one of a couple of OEM-dependent extras); we map it
    /// to a physical slot via SubscriptionManager. Returns -1 when it can't be
    /// determined.
    private fun receivingSlot(context: Context, intent: Intent): Int {
        var subId = intent.getIntExtra(
            "android.telephony.extra.SUBSCRIPTION_INDEX",
            SubscriptionManager.INVALID_SUBSCRIPTION_ID,
        )
        if (subId < 0) subId = intent.getIntExtra("subscription", -1)
        if (subId < 0) return -1
        if (ContextCompat.checkSelfPermission(
                context, Manifest.permission.READ_PHONE_STATE
            ) != PackageManager.PERMISSION_GRANTED
        ) return -1
        val sm = context.getSystemService(SubscriptionManager::class.java) ?: return -1
        val info = try {
            sm.getActiveSubscriptionInfo(subId)
        } catch (e: SecurityException) {
            null
        }
        return info?.simSlotIndex ?: -1
    }
}
