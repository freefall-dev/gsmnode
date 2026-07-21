package app.gsmnode.phoneagent

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Handler
import android.os.Looper

/// Receives incoming MMS notifications (WAP push, M-Notification.ind) and
/// surfaces an "mms" event to Dart. This reports the arrival with the sender and
/// subject parsed from the notification PDU.
///
/// NOTE: Fetching the full MMS body + attachments from the carrier MMSC is a
/// separate download transaction (HTTP over the MMS APN) that a non-default SMS
/// app cannot reliably perform. So the arrival is reported without attachments;
/// the server treats this as `mms:received`. Full attachment download is a
/// documented on-device follow-up.
class MmsReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val data = intent.getByteArrayExtra("data") ?: return
        val (from, subject) = parseNotification(data)

        val payload = HashMap<String, Any?>()
        payload["type"] = "mms"
        payload["from"] = from
        payload["body"] = ""
        payload["subject"] = subject
        payload["attachments"] = emptyList<Map<String, Any?>>()
        payload["timestamp"] = System.currentTimeMillis()

        Handler(Looper.getMainLooper()).post {
            SmsBridge.incomingSink?.success(payload)
        }
    }

    /// Best-effort scan of an M-Notification.ind PDU for the From (0x89) and
    /// Subject (0x96) header values. Returns ("", "") when they can't be read.
    private fun parseNotification(pdu: ByteArray): Pair<String, String> {
        var from = ""
        var subject = ""
        var i = 0
        try {
            while (i < pdu.size - 1) {
                val field = pdu[i].toInt() and 0xFF
                when (field) {
                    0x89 -> { // From: value-length, then encoded address
                        i++
                        val len = pdu[i].toInt() and 0xFF
                        i++
                        // Skip a leading address-type token if present.
                        val start = i
                        from = readCString(pdu, start).also { i = start + it.length + 1 }
                        i = start + len
                    }
                    0x96 -> { // Subject: (optional charset) text
                        i++
                        subject = readCString(pdu, i)
                        i += subject.length + 1
                    }
                    else -> i++
                }
            }
        } catch (_: Exception) {
            // Malformed PDU — return whatever we managed to read.
        }
        return Pair(from.trim(), subject.trim())
    }

    private fun readCString(b: ByteArray, start: Int): String {
        var end = start
        while (end < b.size && b[end].toInt() != 0) end++
        return String(b, start, end - start, Charsets.UTF_8)
            .filter { it.code in 32..126 || it.code > 160 }
    }
}
