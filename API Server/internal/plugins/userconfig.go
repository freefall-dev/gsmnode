package plugins

import (
	"context"
	"errors"
	"sort"
)

// errNoUserConfig is returned for a plugin that offers no per-user settings.
var errNoUserConfig = errors.New("plugin does not offer per-user settings")

// NoGlobalKey marks a UserField that has no global layer — the field resolves
// from the org and user layers only. Borrowed from the "-" of a struct tag.
const NoGlobalKey = "-"

// Per-user plugin settings.
//
// A plugin may offer settings that each end user fills in for themselves — an
// IMAP mailbox, an account API key — rather than one global value set by a
// superadmin. Those settings resolve through a three-layer cascade:
//
//	global (L1) → the plugin's own config in plugins.json (superadmin)
//	org    (L2) → pluginSettings[<plugin>] on the caller's organization
//	user   (L3) → pluginSettings[<plugin>] on the caller's user record
//
// The top layer wins and a lower layer only fills a field the layers above left
// blank, so an operator can impose a value and users fill in the rest. The
// cascade itself lives in internal/api/integrations.go; a plugin only declares
// what it accepts, by implementing UserConfigurable.

// UserField declares one per-user setting. It is a ConfigField plus the metadata
// the cascade needs to place the field across layers.
type UserField struct {
	ConfigField

	// GlobalKey names the key in the plugin's global config that seeds this
	// field when no org or user layer sets it. Empty means the field's own Key.
	// It exists because a global default is often named differently from the
	// per-user value it seeds (imap_default_host → imap_host). Set it to
	// NoGlobalKey for a field with no global layer at all, such as a personal
	// credential that an operator must not be able to set for everyone.
	GlobalKey string `json:"globalKey,omitempty"`

	// Group ties fields that must resolve together from a single layer. Every
	// field sharing a non-empty Group resolves from the highest layer that sets
	// the group's *first* field, so the halves of a credential (username and
	// password) can never be mixed across layers.
	Group string `json:"group,omitempty"`

	// MaskWhenInherited masks this field's value toward the client when it was
	// inherited from a layer the caller cannot edit. Use it for non-secret but
	// still private values, such as a username imposed by an operator.
	MaskWhenInherited bool `json:"maskWhenInherited,omitempty"`
}

// UserConfigSpec is everything a client needs to render a plugin's per-user
// settings form. It travels to the browser as-is, so the plugin owns its own
// copy — no UI knows a specific plugin's field names.
type UserConfigSpec struct {
	// Title heads the settings card ("Email to SMS").
	Title string `json:"title"`
	// Description explains the integration to an end user. Plain text.
	Description string `json:"description"`
	// EnableLabel labels the per-user opt-in checkbox.
	EnableLabel string `json:"enableLabel"`
	// Fields are rendered in order.
	Fields []UserField `json:"fields"`
}

// UserContext identifies the caller a per-user config was resolved for.
type UserContext struct {
	OwnerID string
	Role    string
	OrgID   string
}

// conditionallyUserConfigurable is implemented by a plugin whose per-user
// support is only known at runtime rather than from its Go type — an external
// plugin offers per-user settings only if the manifest it fetched said so.
// Implementing it overrides a plain UserConfigurable type assertion.
type conditionallyUserConfigurable interface {
	// UserConfigurableNow reports whether per-user settings are on offer right
	// now, and the contract to use if so.
	UserConfigurableNow() (UserConfigurable, bool)
}

// UserConfigurable is the optional contract a plugin implements to offer
// per-user settings. A plugin that does not implement it is superadmin-only.
type UserConfigurable interface {
	// UserConfig declares the per-user settings this plugin accepts.
	UserConfig() UserConfigSpec
	// UserHealthCheck probes one caller's fully-resolved config (secrets in
	// clear). It must not touch the plugin's global/live state — the caller
	// runs it against a transient instance.
	UserHealthCheck(ctx context.Context, uc UserContext, cfg map[string]string) Health
}

// userPlugin resolves the per-user contract for a plugin, if it offers one.
//
// A live instance answers first: an external plugin only learns its spec from
// the manifest fetched at Init, so an un-live external plugin has none. A
// builtin's declaration is static, so a bare factory instance serves it — and
// deliberately without Init, which for a builtin like email-to-sms would start
// its listeners.
func (m *Manager) userPlugin(name string) (UserConfigurable, bool) {
	m.mu.Lock()
	p := m.live[name]
	if p == nil {
		if f := m.factories[name]; f != nil {
			p = f()
		}
	}
	m.mu.Unlock()

	if p == nil {
		return nil, false
	}
	if c, ok := p.(conditionallyUserConfigurable); ok {
		return c.UserConfigurableNow()
	}
	uc, ok := p.(UserConfigurable)
	return uc, ok
}

// UserSpec returns a plugin's per-user settings declaration.
func (m *Manager) UserSpec(name string) (UserConfigSpec, bool) {
	if uc, ok := m.userPlugin(name); ok {
		return uc.UserConfig(), true
	}
	return UserConfigSpec{}, false
}

// UserConfigurableNames lists every plugin offering per-user settings, sorted.
func (m *Manager) UserConfigurableNames() []string {
	m.mu.Lock()
	names := map[string]bool{}
	for n := range m.factories {
		names[n] = true
	}
	for n := range m.records {
		names[n] = true
	}
	m.mu.Unlock()

	out := make([]string, 0, len(names))
	for n := range names {
		if _, ok := m.userPlugin(n); ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// UserHealthCheckWith probes a plugin against one caller's resolved config
// (secrets in clear). Unlike HealthCheckWith it neither constructs nor Inits an
// instance for the probe — UserHealthCheck is contracted to be self-contained,
// so the global instance, its listeners and its cached health are untouched.
func (m *Manager) UserHealthCheckWith(ctx context.Context, name string, uc UserContext, cfg map[string]string) (Health, error) {
	c, ok := m.userPlugin(name)
	if !ok {
		return Health{}, errNoUserConfig
	}
	return c.UserHealthCheck(ctx, uc, cfg), nil
}
