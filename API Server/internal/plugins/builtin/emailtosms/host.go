package emailtosms

import "context"

// Host is the callback contract the email-to-sms plugin uses to reach back into
// the API Server. The api package implements it (see internal/api/plugin_host.go)
// and injects it via UseHost at server construction. Defining it here — rather
// than importing the api package — keeps the dependency one-way (api → plugin)
// and avoids an import cycle.
type Host interface {
	// Authenticate verifies a gsmnode user's email + password (the SMTP AUTH
	// credentials) and returns their user record id. This is how an SMTP session
	// is attributed to an owner, mirroring the reference service's per-user auth.
	Authenticate(ctx context.Context, email, password string) (ownerID string, err error)
	// EnqueueSMS queues an outbound SMS owned by ownerID for one of that user's
	// devices, exactly as POST /api/messages would.
	EnqueueSMS(ctx context.Context, ownerID string, phones []string, text string) error
	// IMAPTargets returns the per-user mailboxes to poll in IMAP intake mode,
	// resolved through the plugin cascade (global → org → user). Only users who
	// enabled the integration and supplied mailbox credentials appear.
	IMAPTargets(ctx context.Context) ([]IMAPTarget, error)
}

// IMAPTarget is one user's resolved mailbox connection for IMAP intake.
type IMAPTarget struct {
	OwnerID  string
	Host     string
	Port     int
	Username string
	Password string
	Mailbox  string // "" → the plugin's configured default (INBOX)
}

// host is the process-wide host adapter, set once at startup by UseHost.
var host Host

// UseHost installs the callback adapter. Called by api.New before StartPlugins.
func UseHost(h Host) { host = h }
