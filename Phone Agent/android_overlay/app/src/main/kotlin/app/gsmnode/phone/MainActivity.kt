package app.gsmnode.phone

import android.Manifest
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.telecom.TelecomManager
import android.telephony.SmsManager
import android.telephony.SubscriptionManager
import androidx.core.content.ContextCompat
import io.flutter.embedding.android.FlutterFragmentActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodChannel

// FlutterFragmentActivity (not FlutterActivity): local_auth drives androidx
// BiometricPrompt, which needs a FragmentActivity host to show its sheet.
class MainActivity : FlutterFragmentActivity() {

    companion object {
        const val METHOD_CHANNEL = "app.gsmnode/sms"
        const val EVENT_CHANNEL = "app.gsmnode/sms_incoming"
        const val STATUS_CHANNEL = "app.gsmnode/sms_status"
        const val CALL_CHANNEL = "app.gsmnode/call_incoming"
        const val STATUS_ACTION = "app.gsmnode.SMS_STATUS"

        // Sinks used by the broadcast receivers to push events into Dart.
        @Volatile var incomingSink: EventChannel.EventSink? = null
        @Volatile var statusSink: EventChannel.EventSink? = null
        @Volatile var callSink: EventChannel.EventSink? = null
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        val messenger = flutterEngine.dartExecutor.binaryMessenger

        MethodChannel(messenger, METHOD_CHANNEL).setMethodCallHandler { call, result ->
            when (call.method) {
                "sendSms" -> {
                    val phone = call.argument<String>("phone")
                    val message = call.argument<String>("message")
                    val simSlot = call.argument<Int>("simSlot")
                    val messageId = call.argument<String>("messageId")
                    if (phone.isNullOrBlank() || message == null) {
                        result.error("BAD_ARGS", "phone and message are required", null)
                        return@setMethodCallHandler
                    }
                    try {
                        sendSms(phone, message, simSlot, messageId)
                        result.success(true)
                    } catch (e: Exception) {
                        result.error("SEND_FAILED", e.message, null)
                    }
                }
                "sendDataSms" -> {
                    val phone = call.argument<String>("phone")
                    val payload = call.argument<String>("payload")
                    val port = call.argument<Int>("port") ?: 0
                    val simSlot = call.argument<Int>("simSlot")
                    val messageId = call.argument<String>("messageId")
                    if (phone.isNullOrBlank() || payload == null) {
                        result.error("BAD_ARGS", "phone and payload are required", null)
                        return@setMethodCallHandler
                    }
                    try {
                        sendDataSms(phone, payload, port, simSlot, messageId)
                        result.success(true)
                    } catch (e: Exception) {
                        result.error("SEND_FAILED", e.message, null)
                    }
                }
                "sendMms" -> {
                    val phone = call.argument<String>("phone")
                    val subject = call.argument<String>("subject") ?: ""
                    val text = call.argument<String>("text") ?: ""
                    val attachments = call.argument<List<Map<String, Any?>>>("attachments")
                        ?: emptyList()
                    val simSlot = call.argument<Int>("simSlot")
                    val messageId = call.argument<String>("messageId")
                    if (phone.isNullOrBlank()) {
                        result.error("BAD_ARGS", "phone is required", null)
                        return@setMethodCallHandler
                    }
                    try {
                        sendMms(phone, subject, text, attachments, simSlot, messageId)
                        result.success(true)
                    } catch (e: Exception) {
                        result.error("SEND_FAILED", e.message, null)
                    }
                }
                "getSims" -> {
                    try {
                        result.success(listSims())
                    } catch (e: Exception) {
                        result.error("SIM_LIST_FAILED", e.message, null)
                    }
                }
                "placeCall" -> {
                    val phone = call.argument<String>("phone")
                    if (phone.isNullOrBlank()) {
                        result.error("BAD_ARGS", "phone is required", null)
                        return@setMethodCallHandler
                    }
                    try {
                        placeCall(phone)
                        result.success(true)
                    } catch (e: Exception) {
                        result.error("CALL_FAILED", e.message, null)
                    }
                }
                "startService" -> {
                    GatewayForegroundService.start(this)
                    result.success(true)
                }
                "stopService" -> {
                    GatewayForegroundService.stop(this)
                    result.success(true)
                }
                else -> result.notImplemented()
            }
        }

        EventChannel(messenger, EVENT_CHANNEL).setStreamHandler(
            object : EventChannel.StreamHandler {
                override fun onListen(args: Any?, sink: EventChannel.EventSink?) {
                    incomingSink = sink
                }
                override fun onCancel(args: Any?) {
                    incomingSink = null
                }
            })

        EventChannel(messenger, STATUS_CHANNEL).setStreamHandler(
            object : EventChannel.StreamHandler {
                override fun onListen(args: Any?, sink: EventChannel.EventSink?) {
                    statusSink = sink
                }
                override fun onCancel(args: Any?) {
                    statusSink = null
                }
            })

        EventChannel(messenger, CALL_CHANNEL).setStreamHandler(
            object : EventChannel.StreamHandler {
                override fun onListen(args: Any?, sink: EventChannel.EventSink?) {
                    callSink = sink
                }
                override fun onCancel(args: Any?) {
                    callSink = null
                }
            })
    }

    private fun hasPhoneStatePermission(): Boolean =
        ContextCompat.checkSelfPermission(
            this, Manifest.permission.READ_PHONE_STATE
        ) == PackageManager.PERMISSION_GRANTED

    private fun defaultSmsManager(): SmsManager =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S)
            getSystemService(SmsManager::class.java)
        else
            @Suppress("DEPRECATION") SmsManager.getDefault()

    /// Resolves the SmsManager for a send. A null slot means "use the default
    /// SIM". A non-null slot is honoured strictly: if the slot can't be targeted
    /// (permission missing, or no active subscription in that slot) we throw
    /// rather than silently falling back to the default SIM, so the caller learns
    /// the message did NOT go out on the SIM it asked for.
    private fun smsManagerFor(simSlot: Int?): SmsManager {
        if (simSlot == null) return defaultSmsManager()
        if (!hasPhoneStatePermission()) {
            throw IllegalStateException(
                "cannot target SIM slot $simSlot: READ_PHONE_STATE permission not granted")
        }
        val sm = getSystemService(SubscriptionManager::class.java)
            ?: throw IllegalStateException("SubscriptionManager unavailable on this device")
        val info = sm.getActiveSubscriptionInfoForSimSlotIndex(simSlot)
            ?: throw IllegalArgumentException("SIM slot $simSlot has no active subscription")
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S)
            getSystemService(SmsManager::class.java)
                .createForSubscriptionId(info.subscriptionId)
        else
            @Suppress("DEPRECATION")
            SmsManager.getSmsManagerForSubscriptionId(info.subscriptionId)
    }

    /// Enumerates the active SIMs so the server/UI can present real slot choices
    /// (carrier, number) instead of guessing at slot indices. Requires
    /// READ_PHONE_STATE; returns an empty list when the permission isn't granted
    /// yet or the device has no telephony.
    private fun listSims(): List<Map<String, Any?>> {
        if (!hasPhoneStatePermission()) return emptyList()
        val sm = getSystemService(SubscriptionManager::class.java) ?: return emptyList()
        val infos = try {
            sm.activeSubscriptionInfoList
        } catch (e: SecurityException) {
            null
        } ?: return emptyList()
        return infos
            .sortedBy { it.simSlotIndex }
            .map { info ->
                mapOf(
                    "slot" to info.simSlotIndex,
                    "subscription_id" to info.subscriptionId,
                    "carrier" to (info.carrierName?.toString() ?: ""),
                    "number" to (info.number ?: ""),
                    "display_name" to (info.displayName?.toString() ?: ""),
                )
            }
    }

    /// Builds a PendingIntent that, when fired by the radio, broadcasts the
    /// send/delivery outcome to SmsStatusReceiver (tagged with the message id).
    private fun statusPendingIntent(messageId: String?, phone: String, kind: String): PendingIntent {
        val intent = Intent(this, SmsStatusReceiver::class.java).apply {
            action = STATUS_ACTION
            putExtra("messageId", messageId)
            putExtra("phone", phone)
            putExtra("kind", kind)
        }
        val requestCode = (messageId.orEmpty() + phone + kind).hashCode()
        return PendingIntent.getBroadcast(
            this, requestCode, intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
    }

    /// Places a phone call. Requires the CALL_PHONE permission (requested in Dart
    /// via the phone permission group).
    ///
    /// Uses TelecomManager.placeCall, which routes through the system telecom
    /// service rather than starting the dialer activity ourselves. This is what
    /// lets the call go through when the screen is locked / the app is in the
    /// background — a plain startActivity(ACTION_CALL) is blocked by Android's
    /// background-activity-start restrictions in that state. Falls back to
    /// ACTION_CALL only on very old devices without TelecomManager.placeCall.
    private fun placeCall(phone: String) {
        val uri = Uri.fromParts("tel", phone, null)
        val hasPerm = ContextCompat.checkSelfPermission(
            this, Manifest.permission.CALL_PHONE
        ) == PackageManager.PERMISSION_GRANTED
        if (!hasPerm) {
            throw SecurityException("CALL_PHONE permission not granted")
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            val telecom = getSystemService(TelecomManager::class.java)
            if (telecom != null) {
                telecom.placeCall(uri, Bundle())
                return
            }
        }
        val intent = Intent(Intent.ACTION_CALL, uri)
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        startActivity(intent)
    }

    private fun sendSms(phone: String, message: String, simSlot: Int?, messageId: String?) {
        val sms = smsManagerFor(simSlot)
        val parts = sms.divideMessage(message)
        val sentPI = statusPendingIntent(messageId, phone, "sent")
        val deliveredPI = statusPendingIntent(messageId, phone, "delivered")
        if (parts.size > 1) {
            val sentList = ArrayList<PendingIntent>(parts.size)
            val deliveredList = ArrayList<PendingIntent>(parts.size)
            for (i in parts.indices) {
                sentList.add(sentPI)
                deliveredList.add(deliveredPI)
            }
            sms.sendMultipartTextMessage(phone, null, parts, sentList, deliveredList)
        } else {
            sms.sendTextMessage(phone, null, message, sentPI, deliveredPI)
        }
    }

    /// Sends a binary data SMS: base64 [payload] to [port] on the chosen SIM.
    private fun sendDataSms(
        phone: String, payload: String, port: Int, simSlot: Int?, messageId: String?
    ) {
        val bytes = android.util.Base64.decode(payload, android.util.Base64.DEFAULT)
        val sms = smsManagerFor(simSlot)
        val sentPI = statusPendingIntent(messageId, phone, "sent")
        val deliveredPI = statusPendingIntent(messageId, phone, "delivered")
        sms.sendDataMessage(phone, null, port.toShort(), bytes, sentPI, deliveredPI)
    }

    /// Sends an MMS with an optional subject/text and attachments
    /// ([{filename, content_type, data(base64)}]) via SmsManager. Best-effort:
    /// actual delivery depends on the carrier's MMSC and APN configuration. The
    /// M-Send.req PDU is composed by MmsPduBuilder and handed to the platform,
    /// which routes it through the carrier's MMS stack.
    private fun sendMms(
        phone: String,
        subject: String,
        text: String,
        attachments: List<Map<String, Any?>>,
        simSlot: Int?,
        messageId: String?
    ) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.LOLLIPOP) {
            throw IllegalStateException("MMS send requires Android 5.0+")
        }
        val parts = ArrayList<MmsPduBuilder.Part>()
        if (text.isNotEmpty()) {
            parts.add(MmsPduBuilder.Part("text.txt", "text/plain", text.toByteArray()))
        }
        for (a in attachments) {
            val name = a["filename"] as? String ?: "attachment"
            val ct = a["content_type"] as? String ?: "application/octet-stream"
            val dataB64 = a["data"] as? String ?: continue
            val bytes = android.util.Base64.decode(dataB64, android.util.Base64.DEFAULT)
            parts.add(MmsPduBuilder.Part(name, ct, bytes))
        }
        if (parts.isEmpty()) throw IllegalArgumentException("MMS has no content")

        val pdu = MmsPduBuilder.buildSendReq(phone, subject, parts)
        val cacheFile = java.io.File(cacheDir, "mms_${System.currentTimeMillis()}.pdu")
        cacheFile.writeBytes(pdu)
        val contentUri = androidx.core.content.FileProvider.getUriForFile(
            this, "$packageName.fileprovider", cacheFile
        )
        val sms = smsManagerFor(simSlot)
        val sentPI = statusPendingIntent(messageId, phone, "sent")
        sms.sendMultimediaMessage(this, contentUri, null, null, sentPI)
    }
}
