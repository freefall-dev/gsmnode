package config

import (
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

// EnvFile is the .env path the settings endpoints read at startup and persist
// runtime changes back to.
var EnvFile = ".env"

// Config holds all runtime configuration for the API Server.
type Config struct {
	Addr          string
	PocketBaseURL string
	PBAdminEmail  string
	PBAdminPass   string
	JWTSecret     []byte
	JWTAccessTTL  time.Duration
	WebAppURL     string
	AllowOrigins  []string
	MessageTTL    time.Duration

	// PluginsFile is the local JSON store for plugin enable-state + config
	// (see internal/plugins). It may hold secrets, so it is gitignored.
	PluginsFile string

	// Bootstrap, when true, has the server reconcile the PocketBase schema and
	// ensure the super-admin on startup (see internal/bootstrap).
	Bootstrap          bool
	SuperAdminEmail    string
	SuperAdminPassword string
	SuperAdminName     string
}

// AdminConfigured reports whether the PocketBase service-account credentials are
// set. Bootstrap and every privileged flow authenticate with them.
func (c Config) AdminConfigured() bool {
	return c.PBAdminEmail != "" && c.PBAdminPass != ""
}

// Load reads configuration from environment variables, applying sensible
// defaults. A .env file, if present in the working directory, is loaded first.
func Load() Config {
	loadDotEnv(".env")

	cfg := Config{
		Addr:          getenv("API_ADDR", ":8080"),
		PocketBaseURL: strings.TrimRight(getenv("POCKETBASE_URL", "http://10.2.1.10:8028"), "/"),
		PBAdminEmail:  getenv("PB_ADMIN_EMAIL", ""),
		PBAdminPass:   getenv("PB_ADMIN_PASSWORD", ""),
		JWTSecret:     []byte(getenv("JWT_SECRET", "dev-insecure-change-me-please")),
		JWTAccessTTL:  getdur("JWT_ACCESS_TTL", 24*time.Hour),
		WebAppURL:     strings.TrimRight(getenv("WEBAPP_URL", "http://localhost:8090"), "/"),
		AllowOrigins:  splitCSV(getenv("CORS_ALLOW_ORIGINS", "*")),
		MessageTTL:    getdur("MESSAGE_TTL", 5*time.Minute),
		PluginsFile:   getenv("PLUGINS_FILE", "plugins.json"),

		// Schema + super-admin bootstrap on boot (idempotent). The super-admin is
		// created only when both email and password are set.
		Bootstrap:          boolEnv("PB_BOOTSTRAP", true),
		SuperAdminEmail:    firstEnv("GSMNODE_SUPERADMIN_EMAIL", "SUPERADMIN_EMAIL"),
		SuperAdminPassword: firstEnv("GSMNODE_SUPERADMIN_PASSWORD", "SUPERADMIN_PASSWORD"),
		SuperAdminName:     getenv("GSMNODE_SUPERADMIN_NAME", "Administrator"),
	}

	if cfg.PBAdminEmail == "" || cfg.PBAdminPass == "" {
		log.Println("WARNING: PB_ADMIN_EMAIL / PB_ADMIN_PASSWORD are not set; PocketBase calls will fail")
	}
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// firstEnv returns the value of the first set (non-empty) key, else "".
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// boolEnv reads a boolean environment variable, accepting the common truthy and
// falsey spellings and falling back to def when unset or unrecognized.
func boolEnv(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func getdur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("invalid duration for %s=%q, using default %s", key, v, def)
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// UpdateEnvFile merges key/value updates into a .env file, preserving unrelated
// lines and comments. Existing keys are rewritten in place; new keys are
// appended. The file is created if it does not exist.
func UpdateEnvFile(path string, updates map[string]string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		// Drop a single trailing empty line so we don't accumulate blanks.
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	seen := make(map[string]bool, len(updates))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if val, found := updates[key]; found {
			lines[i] = key + "=" + val
			seen[key] = true
		}
	}
	// Append any keys that weren't already present, in a stable order.
	for _, key := range sortedKeys(updates) {
		if !seen[key] {
			lines = append(lines, key+"="+updates[key])
		}
	}

	out := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(out), 0o644)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// loadDotEnv loads KEY=VALUE pairs from a .env file into the process env if they
// are not already set. It is intentionally minimal (no quoting rules beyond
// trimming surrounding quotes).
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
