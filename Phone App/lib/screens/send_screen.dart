import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../main.dart';
import '../services/api_client.dart';
import '../theme.dart';
import '../widgets/ui.dart';

enum _Kind { sms, data, mms }

class _Attachment {
  _Attachment({
    required this.filename,
    required this.contentType,
    required this.data,
    required this.size,
  });

  final String filename;
  final String contentType;
  final String data; // base64, no data: prefix
  final int size;
}

/// Queue an outbound SMS, data SMS or MMS — the phone's `Send.vue`.
class SendScreen extends StatefulWidget {
  const SendScreen({super.key});

  @override
  State<SendScreen> createState() => _SendScreenState();
}

class _SendScreenState extends State<SendScreen> {
  final _phones = TextEditingController();
  final _text = TextEditingController();
  final _subject = TextEditingController();
  final _dataPayload = TextEditingController();
  final _dataPort = TextEditingController(text: '0');
  final _simSlotFree = TextEditingController();

  _Kind _kind = _Kind.sms;
  final List<_Attachment> _attachments = [];
  String _deviceId = ''; // "" = auto (most recent)
  String _simSlot = ''; // "" = the device's default SIM
  bool _dataPayloadIsText = true;
  late bool _encrypt = crypto.enabled;

  List<Map<String, dynamic>> _devices = const [];
  bool _sending = false;
  String? _error;
  String? _result;

  @override
  void initState() {
    super.initState();
    _loadDevices();
  }

  @override
  void dispose() {
    _phones.dispose();
    _text.dispose();
    _subject.dispose();
    _dataPayload.dispose();
    _dataPort.dispose();
    _simSlotFree.dispose();
    super.dispose();
  }

  Future<void> _loadDevices() async {
    try {
      final res = await apiClient.get('/devices');
      if (!mounted) return;
      setState(() => _devices = itemsOf(res));
    } catch (_) {
      // Listing failure is non-fatal for the form: "Auto (most recent)" still
      // works, and the SIM slot falls back to a free-text field.
    }
  }

  /// SIMs advertised by the chosen device (empty on "Auto", or when the device
  /// hasn't reported any yet).
  List<Map<String, dynamic>> get _deviceSims {
    for (final d in _devices) {
      if (d['device_id'] != _deviceId) continue;
      return (d['sims'] as List?)
              ?.whereType<Map>()
              .map((e) => e.cast<String, dynamic>())
              .toList() ??
          const [];
    }
    return const [];
  }

  static String _simOptionLabel(Map<String, dynamic> sim) {
    final carrier = (sim['carrier'] ?? '').toString();
    final display = (sim['display_name'] ?? '').toString();
    final name = carrier.isNotEmpty
        ? carrier
        : display.isNotEmpty
            ? display
            : 'SIM';
    final number = (sim['number'] ?? '').toString();
    return number.isEmpty
        ? 'Slot ${sim['slot']} · $name'
        : 'Slot ${sim['slot']} · $name · $number';
  }

  Future<void> _pickAttachments() async {
    final picked = await ImagePicker().pickMultipleMedia();
    if (picked.isEmpty) return;
    // The API takes attachment bytes inline, base64-encoded.
    final added = <_Attachment>[];
    for (final f in picked) {
      final bytes = await f.readAsBytes();
      added.add(_Attachment(
        filename: f.name,
        contentType: f.mimeType ?? _contentTypeFor(_extensionOf(f.name)),
        data: base64.encode(bytes),
        size: bytes.length,
      ));
    }
    if (!mounted) return;
    setState(() => _attachments.addAll(added));
  }

  static String? _extensionOf(String name) {
    final dot = name.lastIndexOf('.');
    return dot < 0 ? null : name.substring(dot + 1);
  }

  /// A picked file doesn't always carry a MIME type, so infer one from the
  /// extension. Only the common MMS payloads are worth naming; anything else
  /// travels as opaque bytes, which is what the carrier gets either way.
  static String _contentTypeFor(String? extension) {
    return switch (extension?.toLowerCase()) {
      'jpg' || 'jpeg' => 'image/jpeg',
      'png' => 'image/png',
      'gif' => 'image/gif',
      'webp' => 'image/webp',
      'mp4' => 'video/mp4',
      '3gp' => 'video/3gpp',
      'mp3' => 'audio/mpeg',
      'txt' => 'text/plain',
      'pdf' => 'application/pdf',
      _ => 'application/octet-stream',
    };
  }

  Future<void> _send() async {
    setState(() {
      _error = null;
      _result = null;
    });

    final phoneList = _phones.text
        .split(RegExp(r'[\n,;]+'))
        .map((p) => p.trim())
        .where((p) => p.isNotEmpty)
        .toList();

    final text = _text.text.trim();
    if (phoneList.isEmpty) {
      setState(() => _error = 'Enter at least one phone number.');
      return;
    }
    if (_kind == _Kind.sms && text.isEmpty) {
      setState(() => _error = 'Message text is required.');
      return;
    }
    if (_kind == _Kind.data && _dataPayload.text.trim().isEmpty) {
      setState(() => _error = 'Data payload is required.');
      return;
    }
    if (_kind == _Kind.mms && text.isEmpty && _attachments.isEmpty) {
      setState(() => _error = 'MMS needs text or at least one attachment.');
      return;
    }

    setState(() => _sending = true);
    try {
      final useEnc = _encrypt && crypto.enabled;
      // Recipients and text can be end-to-end encrypted. The data payload and
      // MMS attachments are left as-is (binary already opaque to the server) —
      // the Web App and Phone Agent draw the line in the same place.
      final body = <String, dynamic>{
        'type': _kind.name,
        'phone_numbers':
            useEnc ? await crypto.encryptList(phoneList) : phoneList,
        'encrypted': useEnc,
      };
      if (_kind == _Kind.sms || _kind == _Kind.mms) {
        body['text_message'] = useEnc ? await crypto.encrypt(_text.text) : _text.text;
      }
      if (_kind == _Kind.mms) {
        body['subject'] = _subject.text;
        body['attachments'] = [
          for (final a in _attachments)
            {
              'filename': a.filename,
              'content_type': a.contentType,
              'data': a.data,
            },
        ];
      }
      if (_kind == _Kind.data) {
        final payload = _dataPayload.text.trim();
        body['data_payload'] = _dataPayloadIsText
            ? base64.encode(utf8.encode(_dataPayload.text))
            : payload;
        body['data_port'] = int.tryParse(_dataPort.text.trim()) ?? 0;
      }
      if (_deviceId.isNotEmpty) body['device_id'] = _deviceId;

      final slot = _deviceSims.isNotEmpty ? _simSlot : _simSlotFree.text.trim();
      if (slot.isNotEmpty) {
        final parsed = int.tryParse(slot);
        if (parsed != null) body['sim_number'] = parsed;
      }

      final res = await apiClient.post('/messages', body) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() {
        _result = 'Queued — message ${res['id']} (${res['status']})';
        _text.clear();
        _dataPayload.clear();
        _attachments.clear();
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 18, 16, 32),
      children: [
        const PageHeader(
          title: 'Send message',
          subtitle: 'Queue an SMS, data SMS, or MMS for a device',
        ),
        GsmCard(
          padding: EdgeInsets.zero,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.all(14),
                child: Row(
                  children: [
                    Expanded(
                      child: SingleChildScrollView(
                        scrollDirection: Axis.horizontal,
                        child: SegmentedTabs<_Kind>(
                          value: _kind,
                          options: const [
                            (_Kind.sms, 'SMS'),
                            (_Kind.data, 'Data SMS'),
                            (_Kind.mms, 'MMS'),
                          ],
                          onChanged: (k) => setState(() => _kind = k),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              Divider(height: 1, color: cg.borderSubtle),
              Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: _formBody(cg),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  List<Widget> _formBody(GsmSemantic cg) {
    return [
      LabeledField(
        label: 'Phone numbers',
        help: 'Separate multiple numbers with commas or new lines.',
        child: TextField(
          controller: _phones,
          minLines: 2,
          maxLines: 4,
          keyboardType: TextInputType.multiline,
          style: gsmMono(size: 13, color: cg.textPrimary),
          decoration: const InputDecoration(
            hintText: '+15551234567, +15559876543',
          ),
        ),
      ),
      const SizedBox(height: 18),

      if (_kind == _Kind.sms || _kind == _Kind.mms) ...[
        LabeledField(
          label: _kind == _Kind.mms ? 'Message (optional)' : 'Message',
          child: TextField(
            controller: _text,
            minLines: 3,
            maxLines: 8,
            keyboardType: TextInputType.multiline,
            decoration: const InputDecoration(hintText: 'Your message…'),
          ),
        ),
        const SizedBox(height: 18),
      ],

      if (_kind == _Kind.mms) ...[
        LabeledField(
          label: 'Subject (optional)',
          child: TextField(
            controller: _subject,
            decoration: const InputDecoration(hintText: 'Subject line'),
          ),
        ),
        const SizedBox(height: 18),
        LabeledField(
          label: 'Attachments',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              OutlinedButton.icon(
                onPressed: _pickAttachments,
                icon: const Icon(Icons.perm_media_outlined, size: 16),
                label: const Text('Add photos or video'),
              ),
              for (var i = 0; i < _attachments.length; i++)
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                    decoration: BoxDecoration(
                      color: cg.sunkenBg,
                      borderRadius: BorderRadius.circular(6),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            '${_attachments[i].filename} · ${_attachments[i].contentType}',
                            overflow: TextOverflow.ellipsis,
                            style: gsmMono(size: 11, color: cg.textSecondary),
                          ),
                        ),
                        GestureDetector(
                          onTap: () => setState(() => _attachments.removeAt(i)),
                          child: Text(
                            'remove',
                            style: TextStyle(fontSize: 12, color: cg.danger),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
        ),
        const SizedBox(height: 18),
      ],

      if (_kind == _Kind.data) ...[
        Row(
          children: [
            Expanded(
              child: Text(
                'Payload',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: cg.textPrimary,
                ),
              ),
            ),
            Text(
              'encode text as base64',
              style: TextStyle(fontSize: 11, color: cg.textMuted),
            ),
            Checkbox(
              value: _dataPayloadIsText,
              visualDensity: VisualDensity.compact,
              onChanged: (v) => setState(() => _dataPayloadIsText = v ?? true),
            ),
          ],
        ),
        const SizedBox(height: 2),
        TextField(
          controller: _dataPayload,
          minLines: 3,
          maxLines: 6,
          keyboardType: TextInputType.multiline,
          style: gsmMono(size: 13, color: cg.textPrimary),
          decoration: InputDecoration(
            hintText: _dataPayloadIsText
                ? 'Any text (will be base64-encoded)…'
                : 'Base64-encoded bytes…',
          ),
        ),
        const SizedBox(height: 18),
        LabeledField(
          label: 'Destination port',
          child: TextField(
            controller: _dataPort,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(hintText: '0'),
          ),
        ),
        const SizedBox(height: 18),
      ],

      LabeledField(
        label: 'Device',
        child: GsmDropdown<String>(
          value: _deviceId,
          items: [
            const DropdownMenuItem(value: '', child: Text('Auto (most recent)')),
            for (final d in _devices)
              DropdownMenuItem(
                value: (d['device_id'] ?? '').toString(),
                child: Text(
                  (d['name'] ?? '').toString().isEmpty
                      ? (d['device_id'] ?? '').toString()
                      : d['name'].toString(),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
          ],
          onChanged: (v) => setState(() {
            _deviceId = v ?? '';
            _simSlot = ''; // the previous device's slots no longer apply
          }),
        ),
      ),
      const SizedBox(height: 18),

      LabeledField(
        label: 'SIM (optional)',
        help: '0-based slot; blank uses the device default.',
        child: _deviceSims.isNotEmpty
            ? GsmDropdown<String>(
                value: _simSlot,
                items: [
                  const DropdownMenuItem(value: '', child: Text('Default SIM')),
                  for (final s in _deviceSims)
                    DropdownMenuItem(
                      value: '${s['slot']}',
                      child: Text(_simOptionLabel(s), overflow: TextOverflow.ellipsis),
                    ),
                ],
                onChanged: (v) => setState(() => _simSlot = v ?? ''),
              )
            : TextField(
                controller: _simSlotFree,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(hintText: 'Default'),
              ),
      ),
      const SizedBox(height: 18),

      if (_kind != _Kind.data)
        InkWell(
          onTap: crypto.enabled ? () => setState(() => _encrypt = !_encrypt) : null,
          child: Row(
            children: [
              Checkbox(
                value: _encrypt && crypto.enabled,
                visualDensity: VisualDensity.compact,
                onChanged: crypto.enabled
                    ? (v) => setState(() => _encrypt = v ?? false)
                    : null,
              ),
              Icon(Icons.lock_outline,
                  size: 14,
                  color: crypto.enabled ? cg.textPrimary : cg.textMuted),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  crypto.enabled
                      ? 'End-to-end encrypt recipients + text'
                      : 'End-to-end encrypt (set a passphrase in Settings)',
                  style: TextStyle(
                    fontSize: 13,
                    color: crypto.enabled ? cg.textPrimary : cg.textMuted,
                  ),
                ),
              ),
            ],
          ),
        ),

      if (_error != null) ...[
        const SizedBox(height: 14),
        MessageBanner(_error!),
      ],
      if (_result != null) ...[
        const SizedBox(height: 14),
        MessageBanner(_result!, tone: BannerTone.success),
      ],

      const SizedBox(height: 18),
      SizedBox(
        height: 48,
        child: FilledButton.icon(
          onPressed: _sending ? null : _send,
          icon: const Icon(Icons.send, size: 17),
          label: Text(_sending ? 'Queuing…' : 'Send'),
        ),
      ),
    ];
  }
}
