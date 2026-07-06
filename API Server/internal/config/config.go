package config

import (
	"log"
	"os"
	"strings"
	"time"
)

// Config holds all runtime configuration for the API Server.
type Config struct {
	Addr           string
	PocketBaseURL  string
	PBAdminEmail   string
	PBAdminPass    string
	JWTSecret      []byte
	JWTAccessTTL   time.Duration
	AllowOrigins   []string
	MessageTTL     time.Duration
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
		AllowOrigins:  splitCSV(getenv("CORS_ALLOW_ORIGINS", "*")),
		MessageTTL:    getdur("MESSAGE_TTL", 5*time.Minute),
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
