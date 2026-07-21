package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// externalPlugin adapts a remote HTTP service to the Plugin contract. The remote
// side implements a tiny JSON contract:
//
//	GET  {baseURL}/manifest → { provider, version, capabilities, authType, configFields }
//	GET  {baseURL}/health   → 2xx, optionally { status, detail }
//	POST {baseURL}/invoke   → { action, params } → arbitrary JSON   (v1: unused)
//
// This is the "add a plugin without a rebuild" path: register a base URL at
// runtime and the server drives it over HTTP. It is also the sandboxing story —
// a less-trusted plugin runs as its own process/container.
type externalPlugin struct {
	name    string
	baseURL string
	desc    Descriptor
	client  *http.Client
}

func newExternalPlugin(name, baseURL, provider string) *externalPlugin {
	if provider == "" {
		provider = "External"
	}
	return &externalPlugin{
		name:    name,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 8 * time.Second},
		desc: Descriptor{
			Name:     name,
			Provider: provider,
			Version:  "external",
			Kind:     KindExternal,
			Category: CategoryAPIsExternal, // remote HTTP service; a manifest may override
			AuthType: AuthNone,
		},
	}
}

func (e *externalPlugin) Descriptor() Descriptor { return e.desc }

// Init best-effort fetches the remote manifest to enrich the descriptor. A
// missing/broken manifest is non-fatal — the basic descriptor stands.
func (e *externalPlugin) Init(ctx context.Context, _ map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/manifest", nil)
	if err != nil {
		return nil
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var man struct {
		Provider     string        `json:"provider"`
		Version      string        `json:"version"`
		Category     string        `json:"category"`
		Capabilities []Capability  `json:"capabilities"`
		AuthType     AuthType      `json:"authType"`
		ConfigFields []ConfigField `json:"configFields"`
	}
	if json.Unmarshal(data, &man) == nil {
		if man.Provider != "" {
			e.desc.Provider = man.Provider
		}
		if man.Version != "" {
			e.desc.Version = man.Version
		}
		if man.AuthType != "" {
			e.desc.AuthType = man.AuthType
		}
		if man.Category != "" {
			e.desc.Category = man.Category
		}
		e.desc.Capabilities = man.Capabilities
		e.desc.ConfigFields = man.ConfigFields
	}
	return nil
}

func (e *externalPlugin) HealthCheck(ctx context.Context) Health {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/health", nil)
	if err != nil {
		return Health{Status: StatusDown, Detail: err.Error()}
	}
	resp, err := e.client.Do(req)
	lat := time.Since(start).Milliseconds()
	if err != nil {
		return Health{Status: StatusDown, LatencyMs: lat, Detail: err.Error()}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	// Honour an explicit {status, detail} body when present.
	var body struct {
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(data, &body)

	h := Health{LatencyMs: lat, Detail: body.Detail}
	switch {
	case body.Status != "":
		h.Status = body.Status
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		h.Status = StatusOK
	case resp.StatusCode >= 500:
		h.Status = StatusDown
	default:
		h.Status = StatusDegraded
	}
	if h.Detail == "" && h.Status != StatusOK {
		h.Detail = "HTTP " + resp.Status
	}
	return h
}

// Invoke proxies to the remote /invoke endpoint. Part of the contract; no HTTP
// endpoint exposes it in v1.
func (e *externalPlugin) Invoke(ctx context.Context, action string, params json.RawMessage) (json.RawMessage, error) {
	payload, _ := json.Marshal(map[string]any{"action": action, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/invoke", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return data, nil
}

func (e *externalPlugin) Shutdown(context.Context) error { return nil }
