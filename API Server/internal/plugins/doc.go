package plugins

// Deferred extension points (deliberately NOT in v1 — the "Management MVP").
// The contract and manager are shaped so these can be added without a redesign:
//
//   - Invocation API: the Plugin.Invoke method already exists; a
//     POST /api/admin/plugins/{name}/action endpoint + a normalized request/
//     response envelope would expose it. Add a mapper layer so core logic never
//     depends on a provider's schema.
//   - Resilience: wrap plugin calls with retry/backoff + a circuit breaker, and
//     record per-plugin latency/error metrics for the panel.
//   - Audit logging: record which plugin did what and when.
//   - Sandboxing: the "external" plugin kind is the isolation story — run less
//     trusted plugins as separate processes/containers behind the HTTP contract.
//   - Hot-adding builtin Go code without a rebuild is intentionally unsupported
//     (Go .so plugins are Linux-only and toolchain-fragile); use the external
//     HTTP kind to add plugins at runtime instead.
//
// Per-tenant credentials already exist for builtins via the cascade in
// internal/api/integrations.go (global → org → user), backed by a pluginSettings
// JSON field on the users and organizations collections.
