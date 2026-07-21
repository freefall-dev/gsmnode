package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// secretMask is what a set secret value is echoed back as. On save, a field that
// still equals the mask is left unchanged (mirrors the pb-config password flow).
const secretMask = "••••••••"

// record is the persisted state for one plugin. For builtins, Kind/BaseURL are
// omitted (the descriptor comes from the registry); external plugins set them.
type record struct {
	Kind     string            `json:"kind,omitempty"`
	BaseURL  string            `json:"baseURL,omitempty"`
	Provider string            `json:"provider,omitempty"`
	Enabled  bool              `json:"enabled"`
	Config   map[string]string `json:"config,omitempty"`
}

// View is the plugin shape returned to the panel (secrets masked).
type View struct {
	Descriptor
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config"`
	BaseURL string            `json:"baseURL,omitempty"`
	Health  *Health           `json:"health,omitempty"`
}

// Manager owns the plugin registry, persisted state, and live instances.
type Manager struct {
	path      string
	mu        sync.Mutex
	factories map[string]Factory
	records   map[string]*record
	live      map[string]Plugin
	health    map[string]*Health
	client    *http.Client
}

// NewManager builds a Manager backed by the JSON state file at path.
func NewManager(path string) *Manager {
	return &Manager{
		path:      path,
		factories: builtinFactories(),
		records:   map[string]*record{},
		live:      map[string]Plugin{},
		health:    map[string]*Health{},
		client:    &http.Client{Timeout: 12 * time.Second},
	}
}

// Load reads the state file and initialises every enabled plugin. A missing file
// is fine (no plugins configured yet).
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if data, err := os.ReadFile(m.path); err == nil {
		var recs map[string]*record
		if err := json.Unmarshal(data, &recs); err != nil {
			return err
		}
		m.records = recs
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ctx := context.Background()
	for name, rec := range m.records {
		if !rec.Enabled {
			continue
		}
		p := construct(name, m.factories[name], rec)
		if p == nil {
			log.Printf("plugins: cannot construct %q (unknown builtin?)", name)
			continue
		}
		if err := p.Init(ctx, rec.Config); err != nil {
			log.Printf("plugins: init %q failed: %v", name, err)
			continue
		}
		m.live[name] = p
	}
	return nil
}

// construct builds a plugin instance from a builtin factory or an external record.
func construct(name string, f Factory, rec *record) Plugin {
	if f != nil {
		return f()
	}
	if rec != nil && rec.Kind == KindExternal {
		return newExternalPlugin(name, rec.BaseURL, rec.Provider)
	}
	return nil
}

// descriptorFor returns a plugin's descriptor without needing a live instance.
func (m *Manager) descriptorFor(name string, rec *record) Descriptor {
	if p := m.live[name]; p != nil {
		return p.Descriptor()
	}
	if f := m.factories[name]; f != nil {
		return f().Descriptor()
	}
	if rec != nil && rec.Kind == KindExternal {
		return newExternalPlugin(name, rec.BaseURL, rec.Provider).Descriptor()
	}
	return Descriptor{Name: name}
}

// maskConfig echoes config back with secret fields masked when set.
func maskConfig(d Descriptor, cfg map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range cfg {
		out[k] = v
	}
	for _, f := range d.ConfigFields {
		if f.Secret && out[f.Key] != "" {
			out[f.Key] = secretMask
		}
	}
	return out
}

// List returns every known plugin (registry ∪ persisted), sorted by name.
func (m *Manager) List() []View {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := map[string]bool{}
	for n := range m.factories {
		names[n] = true
	}
	for n := range m.records {
		names[n] = true
	}

	out := make([]View, 0, len(names))
	for name := range names {
		rec := m.records[name]
		d := m.descriptorFor(name, rec)
		v := View{Descriptor: d, Health: m.health[name]}
		if rec != nil {
			v.Enabled = rec.Enabled
			v.BaseURL = rec.BaseURL
			v.Config = maskConfig(d, rec.Config)
		} else {
			v.Config = map[string]string{}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns a single plugin view (ok=false when unknown).
func (m *Manager) Get(name string) (View, bool) {
	for _, v := range m.List() {
		if v.Name == name {
			return v, true
		}
	}
	return View{}, false
}

// Upsert enables/disables a plugin and merges its config, then (re)initialises or
// shuts down the live instance to match. Secrets left at the mask are preserved.
func (m *Manager) Upsert(ctx context.Context, name string, enabled bool, incoming map[string]string) (View, error) {
	m.mu.Lock()

	_, isBuiltin := m.factories[name]
	rec := m.records[name]
	if !isBuiltin && (rec == nil || rec.Kind != KindExternal) {
		m.mu.Unlock()
		return View{}, errUnknown
	}
	if rec == nil {
		rec = &record{}
		m.records[name] = rec
	}

	d := m.descriptorFor(name, rec)
	merged := map[string]string{}
	for k, v := range rec.Config {
		merged[k] = v
	}
	// Apply incoming values, honouring the secret-mask keep-current rule.
	secretKeys := map[string]bool{}
	for _, f := range d.ConfigFields {
		if f.Secret {
			secretKeys[f.Key] = true
		}
	}
	for k, v := range incoming {
		if secretKeys[k] && v == secretMask {
			continue // keep existing secret
		}
		merged[k] = strings.TrimSpace(v)
	}
	// Validate required fields when enabling.
	if enabled {
		for _, f := range d.ConfigFields {
			if f.Required && merged[f.Key] == "" {
				m.mu.Unlock()
				return View{}, errors.New("missing required setting: " + f.Label)
			}
		}
	}

	rec.Enabled = enabled
	rec.Config = merged
	if err := m.persistLocked(); err != nil {
		m.mu.Unlock()
		return View{}, err
	}

	// Reconcile the live instance.
	if old := m.live[name]; old != nil {
		_ = old.Shutdown(ctx)
		delete(m.live, name)
	}
	var initErr error
	if enabled {
		p := construct(name, m.factories[name], rec)
		if p != nil {
			if err := p.Init(ctx, merged); err != nil {
				initErr = err
			} else {
				m.live[name] = p
			}
		}
	}
	m.mu.Unlock()

	v, _ := m.Get(name)
	return v, initErr
}

// RegisterExternal adds a new external (remote HTTP) plugin at runtime — the
// "add a plugin without a rebuild" path. It starts disabled.
func (m *Manager) RegisterExternal(name, baseURL, provider string) error {
	name = strings.TrimSpace(name)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if name == "" || baseURL == "" {
		return errors.New("name and baseURL are required")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.factories[name]; dup {
		return errors.New("a builtin plugin already uses that name")
	}
	if _, dup := m.records[name]; dup {
		return errors.New("a plugin with that name already exists")
	}
	m.records[name] = &record{Kind: KindExternal, BaseURL: baseURL, Provider: provider}
	return m.persistLocked()
}

// Remove deletes an external plugin registration. Builtins can only be disabled.
func (m *Manager) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.records[name]
	if rec == nil || rec.Kind != KindExternal {
		return errors.New("only external plugins can be removed")
	}
	if p := m.live[name]; p != nil {
		_ = p.Shutdown(ctx)
		delete(m.live, name)
	}
	delete(m.records, name)
	delete(m.health, name)
	return m.persistLocked()
}

// HealthCheck probes a plugin now, building a transient instance if it is not
// currently live (so disabled plugins can still be tested). Result is cached.
func (m *Manager) HealthCheck(ctx context.Context, name string) (Health, error) {
	m.mu.Lock()
	p := m.live[name]
	transient := false
	var cfg map[string]string
	if p == nil {
		rec := m.records[name]
		if rec != nil {
			cfg = rec.Config
		}
		p = construct(name, m.factories[name], rec)
		transient = true
	}
	m.mu.Unlock()

	if p == nil {
		return Health{}, errUnknown
	}
	if transient {
		_ = p.Init(ctx, cfg)
		defer func() { _ = p.Shutdown(context.Background()) }()
	}
	h := p.HealthCheck(ctx)

	m.mu.Lock()
	hc := h
	m.health[name] = &hc
	m.mu.Unlock()
	return h, nil
}

// HealthCheckWith probes a plugin against a caller-resolved config rather than
// the stored global config. It builds a transient instance, Inits it with cfg,
// probes, and tears it down — so a per-user cascade (see internal/api/
// integrations.go) can health-check under the credentials in force for that
// caller without disturbing the global instance or its cached health.
func (m *Manager) HealthCheckWith(ctx context.Context, name string, cfg map[string]string) (Health, error) {
	m.mu.Lock()
	rec := m.records[name]
	p := construct(name, m.factories[name], rec)
	m.mu.Unlock()

	if p == nil {
		return Health{}, errUnknown
	}
	_ = p.Init(ctx, cfg)
	defer func() { _ = p.Shutdown(context.Background()) }()
	return p.HealthCheck(ctx), nil
}

// InvokeWith runs a capability against a caller-resolved config. Like
// HealthCheckWith, it uses a transient instance Inited with cfg so per-user
// credentials drive the call. Returns the plugin's raw JSON result.
func (m *Manager) InvokeWith(ctx context.Context, name string, cfg map[string]string, action string, payload json.RawMessage) (json.RawMessage, error) {
	m.mu.Lock()
	rec := m.records[name]
	p := construct(name, m.factories[name], rec)
	m.mu.Unlock()

	if p == nil {
		return nil, errUnknown
	}
	_ = p.Init(ctx, cfg)
	defer func() { _ = p.Shutdown(context.Background()) }()
	return p.Invoke(ctx, action, payload)
}

// RawConfig returns a copy of a plugin's stored (global) config and its enabled
// flag. ok is false for an unknown plugin. This is the top layer (L1) of the
// per-user cascade: the config a superadmin set in the panel, which lower layers
// inherit blank fields from. Secrets are returned in clear — callers must mask
// before returning anything to a client.
func (m *Manager) RawConfig(name string) (cfg map[string]string, enabled, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, isBuiltin := m.factories[name]
	rec := m.records[name]
	if !isBuiltin && rec == nil {
		return nil, false, false
	}
	out := map[string]string{}
	if rec != nil {
		for k, v := range rec.Config {
			out[k] = v
		}
		enabled = rec.Enabled
	}
	return out, enabled, true
}

// Shutdown tears down every live plugin instance. Wire into graceful shutdown.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, p := range m.live {
		_ = p.Shutdown(ctx)
		delete(m.live, name)
	}
}

// persistLocked writes the state file. Caller must hold m.mu.
func (m *Manager) persistLocked() error {
	data, err := json.MarshalIndent(m.records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, append(data, '\n'), 0o600)
}

var errUnknown = errors.New("unknown plugin")

// IsUnknown reports whether err came from addressing a plugin that doesn't exist.
func IsUnknown(err error) bool { return errors.Is(err, errUnknown) }
