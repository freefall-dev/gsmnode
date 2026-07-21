// Package emailtosms is a built-in plugin that turns inbound email into outbound
// SMS, modelled on https://docs.sms-gate.app/services/email-to-sms/. An email
// addressed to {phone}@{domain} is enqueued as an SMS to {phone}.
//
// Two intake modes (selectable, and combinable):
//   - SMTP: the plugin runs an SMTP server. The sender authenticates the session
//     (AUTH PLAIN) with their gsmnode email + password; the SMS is owned by that
//     user. This mirrors the reference service exactly.
//   - IMAP: the plugin polls each user's own mailbox (credentials come from the
//     per-user cascade) and enqueues an SMS owned by that user.
package emailtosms

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"smsgateway/apiserver/internal/plugins"
)

// Name is the plugin's registry key (also the integration key in the cascade).
const Name = "email-to-sms"

func init() {
	plugins.Register(Name, func() plugins.Plugin { return &Plugin{} })
}

// config is the plugin's resolved global settings (set by a superadmin in the
// panel and stored in plugins.json).
type config struct {
	intakeMode   string // "smtp" | "imap" | "both"
	smtpHost     string
	smtpPort     int
	domain       string
	tlsCert      string
	tlsKey       string
	pollInterval time.Duration
	imapMailbox  string
}

// Plugin is the email-to-sms built-in.
type Plugin struct {
	mu      sync.Mutex
	cfg     config
	cancel  context.CancelFunc
	smtpSrv io.Closer // the running SMTP server (nil when not in SMTP mode)

	// runtime state, read by HealthCheck.
	smtpUp   bool
	smtpErr  string
	imapErr  string
	lastPoll time.Time
	pollOK   bool
}

// Descriptor advertises the plugin's config fields; the panel renders the form
// from them. These are the GLOBAL (superadmin) settings. Per-user IMAP mailbox
// credentials live in the cascade, not here.
func (p *Plugin) Descriptor() plugins.Descriptor {
	return plugins.Descriptor{
		Name:     Name,
		Provider: "gsmnode",
		Version:  "1.0.0",
		Kind:     plugins.KindBuiltin,
		Category: plugins.CategoryService,
		AuthType: plugins.AuthBasic,
		Capabilities: []plugins.Capability{
			{ID: "email.to.sms", Description: "Convert inbound email to outbound SMS."},
		},
		ConfigFields: []plugins.ConfigField{
			{Key: "intake_mode", Label: "Intake mode", Type: "select", Required: true, Default: "smtp",
				Help: "How mail is received.", Options: []plugins.SelectOption{
					{Value: "smtp", Label: "SMTP server"},
					{Value: "imap", Label: "IMAP polling"},
					{Value: "both", Label: "Both"},
				}},
			{Key: "domain", Label: "Recipient domain", Type: "text", Required: true, Default: "sms.gsmnode.local",
				Help: "The domain of {phone}@{domain} recipient addresses."},
			{Key: "smtp_host", Label: "SMTP listen host", Type: "text", Default: "127.0.0.1",
				Help: "Interface the SMTP server binds (SMTP / Both mode)."},
			{Key: "smtp_port", Label: "SMTP listen port", Type: "number", Default: "2525"},
			{Key: "smtp_tls_cert", Label: "SMTP TLS cert path", Type: "text",
				Help: "PEM certificate. Leave both TLS fields blank for plaintext (AUTH allowed on the loopback)."},
			{Key: "smtp_tls_key", Label: "SMTP TLS key path", Type: "password", Secret: true},
			{Key: "imap_poll_interval", Label: "IMAP poll interval", Type: "text", Default: "60s",
				Help: "Go duration, e.g. 30s, 2m (IMAP / Both mode)."},
			{Key: "imap_mailbox", Label: "IMAP mailbox", Type: "text", Default: "INBOX",
				Help: "Default folder polled when a user does not set their own."},
		},
	}
}

// parseConfig turns the string config map into a typed config with defaults.
func parseConfig(raw map[string]string) config {
	get := func(k, def string) string {
		if v := strings.TrimSpace(raw[k]); v != "" {
			return v
		}
		return def
	}
	c := config{
		intakeMode:  strings.ToLower(get("intake_mode", "smtp")),
		smtpHost:    get("smtp_host", "127.0.0.1"),
		domain:      strings.ToLower(get("domain", "sms.gsmnode.local")),
		tlsCert:     get("smtp_tls_cert", ""),
		tlsKey:      get("smtp_tls_key", ""),
		imapMailbox: get("imap_mailbox", "INBOX"),
	}
	c.smtpPort, _ = strconv.Atoi(get("smtp_port", "2525"))
	if c.smtpPort == 0 {
		c.smtpPort = 2525
	}
	if d, err := time.ParseDuration(get("imap_poll_interval", "60s")); err == nil && d > 0 {
		c.pollInterval = d
	} else {
		c.pollInterval = 60 * time.Second
	}
	return c
}

// Init (re)starts the intake workers for the configured mode. It first tears down
// any workers from a previous Init so re-enabling with new config is clean.
func (p *Plugin) Init(_ context.Context, raw map[string]string) error {
	p.stop()

	c := parseConfig(raw)
	p.mu.Lock()
	p.cfg = c
	p.smtpErr, p.imapErr = "", ""
	p.pollOK = false
	p.mu.Unlock()

	if host == nil {
		return errors.New("email-to-sms: host adapter not configured")
	}

	// Workers run under a background context cancelled by Shutdown/stop — not the
	// caller's request context, which ends when the enable call returns.
	runCtx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.cancel = cancel
	p.mu.Unlock()

	var errs []string
	if c.intakeMode == "smtp" || c.intakeMode == "both" {
		if err := p.startSMTP(runCtx); err != nil {
			p.setSMTPErr(err.Error())
			errs = append(errs, "smtp: "+err.Error())
		}
	}
	if c.intakeMode == "imap" || c.intakeMode == "both" {
		p.startIMAP(runCtx)
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// HealthCheck reports which intake workers are running.
func (p *Plugin) HealthCheck(_ context.Context) plugins.Health {
	p.mu.Lock()
	defer p.mu.Unlock()

	var parts []string
	status := plugins.StatusOK

	if p.cfg.intakeMode == "smtp" || p.cfg.intakeMode == "both" {
		if p.smtpUp {
			parts = append(parts, "SMTP listening on "+net.JoinHostPort(p.cfg.smtpHost, strconv.Itoa(p.cfg.smtpPort)))
		} else {
			status = plugins.StatusDown
			if p.smtpErr != "" {
				parts = append(parts, "SMTP down: "+p.smtpErr)
			} else {
				parts = append(parts, "SMTP not started")
			}
		}
	}
	if p.cfg.intakeMode == "imap" || p.cfg.intakeMode == "both" {
		switch {
		case p.imapErr != "":
			if status == plugins.StatusOK {
				status = plugins.StatusDegraded
			}
			parts = append(parts, "IMAP: "+p.imapErr)
		case p.pollOK:
			parts = append(parts, "IMAP polled "+p.lastPoll.Format(time.Kitchen))
		default:
			parts = append(parts, "IMAP poller starting")
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "idle")
	}
	return plugins.Health{Status: status, Detail: strings.Join(parts, "; ")}
}

// Invoke is part of the contract; the email-to-sms plugin is a listener, so it
// exposes no callable action in v1.
func (p *Plugin) Invoke(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true}`), nil
}

// Shutdown stops all intake workers.
func (p *Plugin) Shutdown(_ context.Context) error {
	p.stop()
	return nil
}

// stop cancels the worker context and closes the SMTP server, if running.
func (p *Plugin) stop() {
	p.mu.Lock()
	cancel := p.cancel
	p.cancel = nil
	srv := p.smtpSrv
	p.smtpSrv = nil
	p.smtpUp = false
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if srv != nil {
		_ = srv.Close()
	}
}

func (p *Plugin) domain() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg.domain
}

func (p *Plugin) setSMTPErr(msg string) {
	p.mu.Lock()
	p.smtpErr = msg
	p.mu.Unlock()
}
