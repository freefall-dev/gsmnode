import 'package:flutter/material.dart';

import '../../main.dart';
import '../../theme.dart';
import '../../widgets/ui.dart';

/// One plugin's per-user settings, rendered entirely from the schema the server
/// sends (`spec`) — a port of `IntegrationCard.vue`. Nothing here knows a
/// specific plugin's fields, so a new plugin that declares per-user settings
/// shows up as another card with no UI changes.
///
/// Values resolve through the server-side cascade (global → org → user): a field
/// set by a layer above arrives `locked`, with `source` naming who set it, and
/// secrets arrive masked.
///
/// An org admin gets two editable layers and a switch between them: their own
/// personal settings, and the organization's — which land locked in every
/// member's form. The server decides who may do that (`canEditOrg`) and returns
/// one entry under `scopes` per layer the caller may edit.
class IntegrationCard extends StatefulWidget {
  const IntegrationCard({super.key, required this.integration});

  final Map<String, dynamic> integration;

  @override
  State<IntegrationCard> createState() => _IntegrationCardState();
}

class _IntegrationCardState extends State<IntegrationCard> {
  late Map<String, dynamic> _view = widget.integration;

  final Map<String, TextEditingController> _controllers = {};
  final Map<String, String> _draft = {}; // select values live here directly

  String _scope = 'user'; // "user" | "org"
  bool _enabled = false;
  bool _busy = false;
  String? _error;
  String? _notice;
  Map<String, dynamic>? _health;

  @override
  void initState() {
    super.initState();
    _seed(_view);
  }

  @override
  void dispose() {
    for (final c in _controllers.values) {
      c.dispose();
    }
    super.dispose();
  }

  // --- schema accessors -----------------------------------------------------

  Map<String, dynamic> get _spec =>
      (_view['spec'] as Map?)?.cast<String, dynamic>() ?? const {'fields': []};

  List<Map<String, dynamic>> get _fields =>
      (_spec['fields'] as List?)
          ?.whereType<Map>()
          .map((e) => e.cast<String, dynamic>())
          .toList() ??
      const [];

  String get _name => (_view['name'] ?? '').toString();
  String get _title => (_spec['title'] ?? _name).toString();
  bool get _canEditOrg => _view['canEditOrg'] == true;
  bool get _canEdit => _view['isSuperadmin'] != true;
  bool get _editingOrg => _scope == 'org';

  Map<String, dynamic>? get _scopes =>
      (_view['scopes'] as Map?)?.cast<String, dynamic>();

  /// The layer currently being edited. Falls back to the user scope, which the
  /// server always returns.
  Map<String, dynamic>? get _activeScope {
    final scopes = _scopes;
    if (scopes == null) return null;
    final chosen = scopes[_scope] ?? scopes['user'];
    return (chosen as Map?)?.cast<String, dynamic>();
  }

  Map<String, dynamic>? _fieldState(String key) {
    final fields = (_activeScope?['fields'] as Map?)?.cast<String, dynamic>();
    return (fields?[key] as Map?)?.cast<String, dynamic>();
  }

  bool _locked(String key) => _fieldState(key)?['locked'] == true;
  String _sourceOf(String key) =>
      (_fieldState(key)?['source'] ?? 'unset').toString();

  /// A locked field shows what is actually in force, not the editor's own blank.
  String _displayValue(String key) => _locked(key)
      ? (_fieldState(key)?['effective'] ?? '').toString()
      : (_draft[key] ?? '');

  // --- seeding --------------------------------------------------------------

  /// The org gate is stored inverted server-side; the API hands it back as
  /// `orgEnabled`. The personal opt-in is `enabled`.
  bool _enableFlagFor(Map<String, dynamic> v, String scope) =>
      scope == 'org' ? v['orgEnabled'] != false : v['enabled'] == true;

  void _seedDraft() {
    _enabled = _enableFlagFor(_view, _scope);
    final resolved = (_activeScope?['fields'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    for (final f in _fields) {
      final key = (f['key'] ?? '').toString();
      final own = ((resolved[key] as Map?)?['own'] ?? '').toString();
      _draft[key] = own;
      if (f['type'] != 'select') {
        (_controllers[key] ??= TextEditingController()).text =
            _locked(key) ? _displayValue(key) : own;
      }
    }
  }

  void _seed(Map<String, dynamic> v) {
    _view = v;
    // An org admin who has switched to the org layer stays there across a save.
    if (_scope == 'org' && _scopes?['org'] == null) _scope = 'user';
    _seedDraft();
  }

  void _selectScope(String next) {
    if (_scope == next) return;
    setState(() {
      _scope = next;
      _error = null;
      _notice = null;
      _health = null;
      _seedDraft();
    });
  }

  // --- actions --------------------------------------------------------------

  Future<void> _save() async {
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
    });
    try {
      final config = <String, dynamic>{};
      for (final f in _fields) {
        final key = (f['key'] ?? '').toString();
        if (_locked(key)) continue;
        config[key] = f['type'] == 'select'
            ? (_draft[key] ?? '')
            : (_controllers[key]?.text ?? '');
      }
      final res = await apiClient.put('/integrations/$_name', {
        'scope': _scope,
        'enabled': _enabled,
        'config': config,
      }) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() {
        _seed(res);
        _notice = _editingOrg ? 'Saved for your organization.' : 'Saved.';
      });
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _test() async {
    setState(() {
      _busy = true;
      _error = null;
      _notice = null;
      _health = null;
    });
    try {
      final out = await apiClient
          .post('/integrations/$_name/health', const {}) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() =>
          _health = (out['health'] as Map?)?.cast<String, dynamic>() ?? const {});
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = describeError(e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  // --- build ----------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    final cg = context.cg;
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: SectionCard(
        title: _title,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ..._body(cg),
            if (_error != null) ...[
              const SizedBox(height: 12),
              MessageBanner(_error!),
            ],
          ],
        ),
      ),
    );
  }

  List<Widget> _body(GsmSemantic cg) {
    if (_view['available'] != true) {
      return [
        MessageBanner(
          'The $_title integration is turned off by your administrator.',
          tone: BannerTone.info,
        ),
      ];
    }
    // An org admin must still reach the form when the org gate is off, or nobody
    // could ever switch it back on.
    if (_view['orgEnabled'] == false && !_canEditOrg) {
      return [
        MessageBanner(
          'The $_title integration is turned off for your organization.',
          tone: BannerTone.info,
        ),
      ];
    }

    final description = (_spec['description'] ?? '').toString();

    return [
      if (description.isNotEmpty) ...[
        Text(description, style: TextStyle(fontSize: 13, color: cg.textSecondary)),
        const SizedBox(height: 12),
      ],
      if (_view['isSuperadmin'] == true)
        const MessageBanner(
          'You are a superadmin — manage the global settings in the API Server\'s '
          'Plugins panel. The form below is available to regular users.',
          tone: BannerTone.info,
        ),
      if (_canEdit) ...[
        // Layer switch: only an org admin has more than one editable layer.
        if (_canEditOrg) ...[
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: SegmentedTabs<String>(
              value: _scope,
              options: const [
                ('user', 'My settings'),
                ('org', 'Organization'),
              ],
              onChanged: _selectScope,
            ),
          ),
          const SizedBox(height: 12),
        ],
        if (_editingOrg) ...[
          const MessageBanner(
            'These apply to everyone in your organization. A value you set here '
            'is locked for members and overrides what they entered themselves.',
            tone: BannerTone.info,
          ),
          const SizedBox(height: 12),
        ] else if (_canEditOrg && _view['orgEnabled'] == false) ...[
          MessageBanner(
            '$_title is turned off for your organization. Switch to '
            '"Organization" to turn it back on.',
            tone: BannerTone.warning,
          ),
          const SizedBox(height: 12),
        ],

        InkWell(
          onTap: () => setState(() => _enabled = !_enabled),
          child: Row(
            children: [
              Checkbox(
                value: _enabled,
                visualDensity: VisualDensity.compact,
                onChanged: (v) => setState(() => _enabled = v ?? false),
              ),
              Expanded(
                child: Text(
                  _editingOrg
                      ? 'Enable $_title for my organization'
                      : (_spec['enableLabel'] ?? 'Enable $_title').toString(),
                  style: TextStyle(fontSize: 13, color: cg.textSecondary),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 8),

        for (final f in _fields) ...[
          _field(cg, f),
          const SizedBox(height: 14),
        ],

        Row(
          children: [
            Expanded(
              child: FilledButton(
                onPressed: _busy ? null : _save,
                child: Text(_editingOrg ? 'Save for org' : 'Save'),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: OutlinedButton(
                onPressed: _busy ? null : _test,
                child: const Text('Test connection'),
              ),
            ),
          ],
        ),
        if (_health != null) ...[
          const SizedBox(height: 10),
          _healthBadge(cg),
        ],
        if (_editingOrg) ...[
          const SizedBox(height: 6),
          Text(
            '"Test connection" always probes the settings in force for you, not '
            'the organization\'s in isolation.',
            style: TextStyle(fontSize: 11, color: cg.textMuted),
          ),
        ],
        if (_notice != null) ...[
          const SizedBox(height: 10),
          MessageBanner(_notice!, tone: BannerTone.success),
        ],
      ],
    ];
  }

  Widget _field(GsmSemantic cg, Map<String, dynamic> f) {
    final key = (f['key'] ?? '').toString();
    final locked = _locked(key);
    final help = (f['help'] ?? '').toString();

    final Widget input;
    if (f['type'] == 'select') {
      final options = (f['options'] as List?)
              ?.whereType<Map>()
              .map((e) => e.cast<String, dynamic>())
              .toList() ??
          const <Map<String, dynamic>>[];
      final current = _draft[key] ?? '';
      input = GsmDropdown<String>(
        value: options.any((o) => '${o['value']}' == current)
            ? current
            : (options.isEmpty ? '' : '${options.first['value']}'),
        items: [
          if (options.isEmpty) const DropdownMenuItem(value: '', child: Text('—')),
          for (final o in options)
            DropdownMenuItem(
              value: '${o['value']}',
              child: Text('${o['label']}', overflow: TextOverflow.ellipsis),
            ),
        ],
        onChanged: locked ? null : (v) => setState(() => _draft[key] = v ?? ''),
      );
    } else {
      input = TextField(
        controller: _controllers[key] ??= TextEditingController(),
        enabled: !locked,
        obscureText: f['type'] == 'password',
        keyboardType:
            f['type'] == 'number' ? TextInputType.number : TextInputType.text,
        autocorrect: false,
        decoration: InputDecoration(hintText: (f['default'] ?? '').toString()),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Flexible(
              child: Text(
                (f['label'] ?? key).toString(),
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: cg.textSecondary,
                ),
              ),
            ),
            if (locked) ...[
              const SizedBox(width: 6),
              MonoChip('set by ${_sourceOf(key)}'),
            ],
          ],
        ),
        const SizedBox(height: 6),
        input,
        if (help.isNotEmpty) ...[
          const SizedBox(height: 5),
          Text(help, style: TextStyle(fontSize: 11, color: cg.textMuted)),
        ],
      ],
    );
  }

  Widget _healthBadge(GsmSemantic cg) {
    final status = (_health?['status'] ?? '').toString();
    final detail = (_health?['detail'] ?? '').toString();
    final (fg, bg) = switch (status) {
      'ok' => (cg.success, cg.successTint),
      'degraded' => (cg.warning, cg.warningTint),
      _ => (cg.danger, cg.dangerTint),
    };
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text(
          detail.isEmpty ? status : '$status — $detail',
          style: gsmMono(size: 11, color: fg),
        ),
      ),
    );
  }
}
