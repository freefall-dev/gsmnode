// Package plugins is the API Server's plugin system: a uniform contract for
// integrating services that extend gsmnode (email-to-SMS bridges, external
// notification sinks, …).
//
// Two plugin kinds share one contract:
//   - "builtin"  — a Go connector compiled into the server (type-safe, first-party).
//     Adding a new builtin requires a rebuild. See builtin/builtin.go.
//   - "external" — a remote service registered at runtime (no rebuild) that speaks
//     a small JSON contract over HTTP. See external.go.
//
// Enable-state and per-plugin config (including secrets) are persisted to a local
// plugins.json by the Manager, mirroring how the PocketBase connection persists to
// .env. See doc.go for the deliberately-deferred extension points.
package plugins

import (
	"context"
	"encoding/json"
)

// Plugin kinds.
const (
	KindBuiltin  = "builtin"
	KindExternal = "external"
)

// AuthType describes how a plugin authenticates to its upstream. It is metadata
// for the UI/operators; each plugin implements the mechanics itself.
type AuthType string

const (
	AuthNone    AuthType = "none"
	AuthAPIKey  AuthType = "apikey"
	AuthBasic   AuthType = "basic"
	AuthOAuth2  AuthType = "oauth2"
	AuthWebhook AuthType = "webhook"
)

// Health status values.
const (
	StatusOK       = "ok"
	StatusDegraded = "degraded"
	StatusDown     = "down"
)

// Plugin categories group a plugin in the admin panel. A plugin with an empty
// category is treated as CategoryService by the panel.
const (
	CategoryService      = "service"       // in-process services (listeners, bridges)
	CategoryAPIsExternal = "apis-external" // remote HTTP APIs (external plugins)
)

// SelectOption is one choice for a ConfigField of Type "select".
type SelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ConfigField declares one configurable setting a plugin accepts. It drives the
// panel's generated config form and controls secret masking.
type ConfigField struct {
	Key      string         `json:"key"`
	Label    string         `json:"label"`
	Type     string         `json:"type"` // "text" | "password" | "number" | "select"
	Required bool           `json:"required"`
	Secret   bool           `json:"secret"` // never echoed back to clients in clear
	Help     string         `json:"help,omitempty"`
	Default  string         `json:"default,omitempty"` // effective default when unset
	Options  []SelectOption `json:"options,omitempty"` // for Type "select"
}

// Capability is one operation a plugin exposes. It maps a stable id to the
// upstream endpoint it calls and a human description shown in the panel.
type Capability struct {
	ID          string `json:"id"`
	Method      string `json:"method,omitempty"`   // e.g. "GET"
	Endpoint    string `json:"endpoint,omitempty"` // upstream path
	Description string `json:"description,omitempty"`
}

// UnmarshalJSON accepts either a bare string ("states.all") or a full object, so
// external manifests can advertise capabilities in either form.
func (c *Capability) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		c.ID = s
		return nil
	}
	type alias Capability
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*c = Capability(a)
	return nil
}

// Descriptor is the static metadata a plugin advertises about itself.
type Descriptor struct {
	Name         string        `json:"name"`
	Provider     string        `json:"provider"`
	Version      string        `json:"version"`
	Kind         string        `json:"kind"`     // KindBuiltin | KindExternal
	Category     string        `json:"category"` // one of Category* — groups the plugin in the panel
	Capabilities []Capability  `json:"capabilities"`
	AuthType     AuthType      `json:"authType"`
	ConfigFields []ConfigField `json:"configFields"`
}

// Health is the outcome of a plugin's HealthCheck.
type Health struct {
	Status    string `json:"status"` // StatusOK | StatusDegraded | StatusDown
	LatencyMs int64  `json:"latencyMs,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Plugin is the contract every plugin (builtin or external) implements.
type Plugin interface {
	// Descriptor returns the plugin's static metadata. It may be enriched after
	// Init (e.g. an external plugin fetching its manifest).
	Descriptor() Descriptor
	// Init prepares the plugin with its resolved config (secrets included). It is
	// called when the plugin is enabled or its config changes.
	Init(ctx context.Context, config map[string]string) error
	// HealthCheck probes the plugin and classifies the result.
	HealthCheck(ctx context.Context) Health
	// Invoke runs a named capability. Part of the contract for future use; v1
	// exposes no HTTP endpoint for it.
	Invoke(ctx context.Context, action string, params json.RawMessage) (json.RawMessage, error)
	// Shutdown releases any resources held by the plugin.
	Shutdown(ctx context.Context) error
}

// Factory builds a fresh instance of a builtin plugin.
type Factory func() Plugin

// registry holds the builtin plugin factories keyed by descriptor name.
var registry = map[string]Factory{}

// Register adds a builtin plugin factory. Called from a builtin package's init().
// Panics on a duplicate name so wiring mistakes surface at startup.
func Register(name string, f Factory) {
	if _, dup := registry[name]; dup {
		panic("plugins: duplicate registration for " + name)
	}
	registry[name] = f
}

// builtinFactories returns a copy of the registered builtin factories.
func builtinFactories() map[string]Factory {
	out := make(map[string]Factory, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}
