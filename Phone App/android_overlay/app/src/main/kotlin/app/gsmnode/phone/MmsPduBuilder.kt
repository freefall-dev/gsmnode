package app.gsmnode.phone

import java.io.ByteArrayOutputStream

/// Minimal composer for an MMS M-Send.req PDU (WSP/MMS encapsulation), enough to
/// hand a text + attachment message to the platform's MMS stack via
/// SmsManager.sendMultimediaMessage.
///
/// This is intentionally compact and best-effort: it covers the common single-
/// recipient, text + binary-parts case. Real delivery still depends on the
/// carrier MMSC / APN. Field codes follow OMA MMS Encapsulation 1.2.
object MmsPduBuilder {

    data class Part(val name: String, val contentType: String, val data: ByteArray)

    // MMS header field assigned numbers (high bit set = short-integer field id).
    private const val MESSAGE_TYPE = 0x8C
    private const val TRANSACTION_ID = 0x98
    private const val MMS_VERSION = 0x8D
    private const val FROM = 0x89
    private const val TO = 0x97
    private const val SUBJECT = 0x96
    private const val CONTENT_TYPE = 0x84

    private const val M_SEND_REQ = 0x80
    private const val FROM_INSERT_ADDRESS = 0x81 // token: let the MMSC fill "From"
    private const val VERSION_1_2 = 0x92

    fun buildSendReq(recipient: String, subject: String, parts: List<Part>): ByteArray {
        val out = ByteArrayOutputStream()

        out.write(MESSAGE_TYPE); out.write(M_SEND_REQ)

        out.write(TRANSACTION_ID)
        writeTextString(out, System.currentTimeMillis().toString())

        out.write(MMS_VERSION); out.write(VERSION_1_2)

        // From: value-length 1 + insert-address token (MMSC fills the real number).
        out.write(FROM)
        out.write(1)
        out.write(FROM_INSERT_ADDRESS)

        // To: recipient encoded as "<number>/TYPE=PLMN".
        out.write(TO)
        writeEncodedString(out, "$recipient/TYPE=PLMN")

        if (subject.isNotEmpty()) {
            out.write(SUBJECT)
            writeEncodedString(out, subject)
        }

        // Content-Type: application/vnd.wap.multipart.mixed, then the multipart body.
        out.write(CONTENT_TYPE)
        writeTextString(out, "application/vnd.wap.multipart.mixed")
        writeMultipart(out, parts)

        return out.toByteArray()
    }

    private fun writeMultipart(out: ByteArrayOutputStream, parts: List<Part>) {
        writeUintvar(out, parts.size.toLong())
        for (p in parts) {
            val header = ByteArrayOutputStream()
            writeTextString(header, p.contentType)
            // Content-Location (0x8E) so parts are named.
            header.write(0x8E)
            writeTextString(header, p.name)

            writeUintvar(out, header.size().toLong()) // headers length
            writeUintvar(out, p.data.size.toLong())   // data length
            out.write(header.toByteArray())
            out.write(p.data)
        }
    }

    // --- WSP primitive encoders ------------------------------------------------

    private fun writeTextString(out: ByteArrayOutputStream, s: String) {
        val bytes = s.toByteArray(Charsets.UTF_8)
        // Quote a leading high byte per WSP text-string rules.
        if (bytes.isNotEmpty() && (bytes[0].toInt() and 0x80) != 0) out.write(0x7F)
        out.write(bytes)
        out.write(0x00)
    }

    // Encoded-string-value: charset-prefixed for non-ASCII, plain otherwise.
    private fun writeEncodedString(out: ByteArrayOutputStream, s: String) {
        val bytes = s.toByteArray(Charsets.UTF_8)
        val ascii = bytes.all { (it.toInt() and 0x80) == 0 }
        if (ascii) {
            writeTextString(out, s)
        } else {
            val body = ByteArrayOutputStream()
            body.write(0xEA) // charset UTF-8 (IANA MIBenum 106 → 0x6A | 0x80)
            body.write(bytes)
            body.write(0x00)
            writeValueLength(out, body.size().toLong())
            out.write(body.toByteArray())
        }
    }

    private fun writeValueLength(out: ByteArrayOutputStream, len: Long) {
        if (len < 31) {
            out.write(len.toInt())
        } else {
            out.write(31) // length-quote
            writeUintvar(out, len)
        }
    }

    // Variable-length unsigned integer (uintvar), 7 bits per byte, MSB=continue.
    private fun writeUintvar(out: ByteArrayOutputStream, value: Long) {
        if (value < 0x80) {
            out.write(value.toInt())
            return
        }
        val bytes = ArrayList<Int>()
        var v = value
        bytes.add((v and 0x7F).toInt())
        v = v shr 7
        while (v > 0) {
            bytes.add(((v and 0x7F) or 0x80).toInt())
            v = v shr 7
        }
        for (i in bytes.indices.reversed()) out.write(bytes[i])
    }
}
