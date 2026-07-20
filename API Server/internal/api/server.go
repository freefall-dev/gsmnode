package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"smsgateway/apiserver/internal/config"
	"smsgateway/apiserver/internal/pb"
)

// Collection names in PocketBase.
const (
	colUsers    = "users"
	colOrgs     = "organizations"
	colDevices  = "devices"
	colMessages = "messages"
	colInbox    = "inbox"
	colWebhooks = "webhooks"
)

// Server wires together the HTTP handlers and their dependencies.
type Server struct {
	mu  sync.RWMutex // guards the mutable Web App URL + CORS origins in cfg
	cfg config.Config
	pb  *pb.Client
}

// New constructs a Server.
func New(cfg config.Config, client *pb.Client) *Server {
	return &Server{cfg: cfg, pb: client}
}

// pbSettings snapshots the PocketBase connection for the settings endpoints.
func (s *Server) pbSettings() (url, adminEmail, adminPassword string) {
	return s.pb.Settings()
}

// setPBConfig retargets the PocketBase connection at runtime.
func (s *Server) setPBConfig(url, adminEmail, adminPassword string) {
	s.pb.SetConfig(url, adminEmail, adminPassword)
}

// webAppURL returns the Web App address probed by /api/status.
func (s *Server) webAppURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.WebAppURL
}

// webAppSettings snapshots the Web App settings for the settings endpoints.
func (s *Server) webAppSettings() (url string, allowOrigins []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.WebAppURL, append([]string(nil), s.cfg.AllowOrigins...)
}

// setWebAppConfig applies new Web App settings at runtime. The CORS middleware
// reads the origin list per request, so the new list is live immediately.
func (s *Server) setWebAppConfig(url string, allowOrigins []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.WebAppURL = url
	s.cfg.AllowOrigins = append([]string(nil), allowOrigins...)
}

// Handler returns the root HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Web panel (public) — embedded Vue + Tailwind app. Only the explicit
	// panel paths are routed to it so unknown /api/* paths still 404 as JSON.
	panel := panelHandler()
	mux.Handle("GET /{$}", panel)
	mux.Handle("GET /assets/", panel)
	mux.Handle("GET /favicon.svg", panel)
	mux.Handle("GET /favicon-32.png", panel)
	mux.Handle("GET /gsmnode-horizontal.png", panel)
	mux.Handle("GET /gsmnode-horizontal-white.png", panel)

	// Health (public)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/status", s.handleStatus)

	// Auth — proxied to the PocketBase kept behind this server. The token the
	// client receives is PocketBase's own; PocketBase's address is never exposed.
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/refresh", s.handleRefresh)
	mux.HandleFunc("GET /api/auth/validate", s.handleValidate)
	mux.HandleFunc("GET /api/auth/me", s.requireUser(s.handleMe))

	// Self-service account — a signed-in user editing their own name/password.
	mux.HandleFunc("PATCH /api/auth/me", s.requireUser(s.handleUpdateMe))
	mux.HandleFunc("POST /api/auth/change-password", s.requireUser(s.handleChangePassword))

	// User management — gated on the caller being a manager (admin or superadmin).
	// Only a superadmin may create, edit, or delete superadmins.
	mux.HandleFunc("GET /api/users", s.requireManager(s.handleListUsers))
	mux.HandleFunc("POST /api/users", s.requireManager(s.handleCreateUser))
	mux.HandleFunc("PATCH /api/users/{id}", s.requireManager(s.handleUpdateUser))
	mux.HandleFunc("DELETE /api/users/{id}", s.requireManager(s.handleDeleteUser))

	// Organizations — the tenants users belong to. Listing is manager-scoped (an
	// admin sees only their own org). Any org-less user may create an org and
	// becomes its admin; an admin may rename or delete their own org; a superadmin
	// spans every organization.
	mux.HandleFunc("GET /api/orgs", s.requireManager(s.handleListOrgs))
	mux.HandleFunc("POST /api/orgs", s.requireUser(s.handleCreateOrg))
	mux.HandleFunc("PATCH /api/orgs/{id}", s.requireManager(s.handleUpdateOrg))
	mux.HandleFunc("DELETE /api/orgs/{id}", s.requireManager(s.handleDeleteOrg))

	// PocketBase connection settings — superadmin only. These do NOT require the
	// service account to already be configured (they exist to configure it).
	mux.HandleFunc("GET /api/admin/pb-config", s.requireSuperadminAuth(s.handleGetPBConfig))
	mux.HandleFunc("POST /api/admin/pb-config/test", s.requireSuperadminAuth(s.handleTestPBConfig))
	mux.HandleFunc("PUT /api/admin/pb-config", s.requireSuperadminAuth(s.handleUpdatePBConfig))

	// Web App settings — superadmin only. Where the Web App lives (probed by
	// /api/status) and which browser origins CORS admits.
	mux.HandleFunc("GET /api/admin/webapp-config", s.requireSuperadminAuth(s.handleGetWebAppConfig))
	mux.HandleFunc("POST /api/admin/webapp-config/test", s.requireSuperadminAuth(s.handleTestWebAppConfig))
	mux.HandleFunc("PUT /api/admin/webapp-config", s.requireSuperadminAuth(s.handleUpdateWebAppConfig))

	mux.HandleFunc("GET /api/devices", s.requireUser(s.handleListDevices))
	mux.HandleFunc("DELETE /api/devices/{id}", s.requireUser(s.handleDeleteDevice))

	mux.HandleFunc("GET /api/messages", s.requireUser(s.handleListMessages))
	mux.HandleFunc("POST /api/messages", s.requireUser(s.handleEnqueueMessage))
	mux.HandleFunc("GET /api/messages/{id}", s.requireUser(s.handleGetMessage))

	mux.HandleFunc("POST /api/calls", s.requireUser(s.handleEnqueueCall))

	mux.HandleFunc("GET /api/inbox", s.requireUser(s.handleListInbox))

	mux.HandleFunc("GET /api/webhooks", s.requireUser(s.handleListWebhooks))
	mux.HandleFunc("POST /api/webhooks", s.requireUser(s.handleCreateWebhook))
	mux.HandleFunc("DELETE /api/webhooks/{id}", s.requireUser(s.handleDeleteWebhook))

	// Mobile / device API — the phone app registers, pulls work, and reports.
	// Registration is authenticated with the user's JWT; everything else with
	// the opaque device token returned at registration.
	mux.HandleFunc("POST /api/mobile/v1/device", s.requireUser(s.handleRegisterDevice))
	mux.HandleFunc("POST /api/mobile/v1/ping", s.requireDevice(s.handlePing))
	mux.HandleFunc("GET /api/mobile/v1/messages", s.requireDevice(s.handlePullMessages))
	mux.HandleFunc("PATCH /api/mobile/v1/messages/{id}", s.requireDevice(s.handleReportMessage))
	mux.HandleFunc("POST /api/mobile/v1/inbox", s.requireDevice(s.handleReceiveSMS))

	return s.withMiddleware(mux)
}

// withMiddleware applies CORS, request logging, and panic recovery globally.
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return s.recoverer(s.cors(s.logger(next)))
}

func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// originAllowed reports whether origin may call this server, and whether it was
// the wildcard that allowed it. The allow-list is consulted per request rather
// than captured once, so editing it from the panel takes effect without a
// restart.
func (s *Server) originAllowed(origin string) (allowed, wildcard bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, o := range s.cfg.AllowOrigins {
		if o == "*" {
			return true, true
		}
		if o == origin {
			allowed = true
		}
	}
	return allowed, false
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			if allowed, wildcard := s.originAllowed(origin); allowed {
				if wildcard {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusWriter captures the response status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

// --- auth context ---

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxDevice
)

// userFromCtx returns the authenticated user id/email from the request context.
func userFromCtx(ctx context.Context) (id, email string) {
	if c, ok := ctx.Value(ctxUser).(*callerIdentity); ok {
		return c.ID, c.Email
	}
	return "", ""
}

// deviceFromCtx returns the authenticated device record from the request context.
func deviceFromCtx(ctx context.Context) pb.Record {
	if d, ok := ctx.Value(ctxDevice).(pb.Record); ok {
		return d
	}
	return nil
}

// requireUser wraps a handler to require a valid PocketBase user token. The
// resolved identity (id, email, name, role) is stashed on the request context.
func (s *Server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		who, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUser, who)))
	}
}

// requireDevice wraps a handler to require a valid device token. The matching
// device record is loaded and attached to the request context.
func (s *Server) requireDevice(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing device token")
			return
		}
		device, err := s.pb.FindFirst(r.Context(), colDevices,
			"auth_token = "+pbQuote(token), "")
		if err != nil {
			writeUpstreamError(w, err)
			return
		}
		if device == nil {
			writeError(w, http.StatusUnauthorized, "unknown device token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxDevice, device)
		next(w, r.WithContext(ctx))
	}
}

// bearerToken extracts a token from the Authorization header (with or without
// the "Bearer " prefix).
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(h)
}

// pbQuote safely quotes a string value for a PocketBase filter expression.
func pbQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
