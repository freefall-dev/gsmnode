package emailtosms

import (
	"context"
	"strconv"
	"strings"

	"smsgateway/apiserver/internal/plugins"
)

// Per-user settings for email-to-sms: each user connects their own IMAP mailbox,
// which the plugin polls and turns into SMS owned by them. A superadmin can seed
// the host/port/mailbox globally (the imap_default_* settings in Descriptor) and
// an org admin can impose them for their organization; whatever is left blank a
// user fills in. See internal/plugins/userconfig.go for the cascade.

// Per-user field keys. They are also the keys stored under
// pluginSettings["email-to-sms"].config on a user or organization record.
const (
	FieldHost     = "imap_host"
	FieldPort     = "imap_port"
	FieldUser     = "imap_user"
	FieldPassword = "imap_password"
	FieldMailbox  = "imap_mailbox"
)

// credentialGroup ties the username and password so they always resolve from
// one layer — a user's password must never be paired with an inherited username.
const credentialGroup = "credentials"

// UserConfig declares the per-user mailbox settings.
func (p *Plugin) UserConfig() plugins.UserConfigSpec {
	return plugins.UserConfigSpec{
		Title: "Email to SMS",
		Description: "Send an SMS by email. Address a message to <phone>@<domain> — the body " +
			"becomes the text. Connect your mailbox below to have the server poll it (IMAP), " +
			"or send directly to the gateway's SMTP server authenticating with your gsmnode login.",
		EnableLabel: "Poll my mailbox and turn incoming email into SMS",
		Fields: []plugins.UserField{
			{
				ConfigField: plugins.ConfigField{
					Key: FieldHost, Label: "IMAP host", Type: "text",
					Help: "Your mail provider's IMAP server, e.g. imap.gmail.com.",
				},
				GlobalKey: "imap_default_host",
			},
			{
				ConfigField: plugins.ConfigField{
					Key: FieldPort, Label: "Port", Type: "number", Default: "993",
				},
				GlobalKey: "imap_default_port",
			},
			{
				ConfigField: plugins.ConfigField{
					Key: FieldUser, Label: "Username", Type: "text",
					Help: "Usually your full email address.",
				},
				// No global layer: credentials are personal, never seeded globally.
				GlobalKey:         plugins.NoGlobalKey,
				Group:             credentialGroup,
				MaskWhenInherited: true,
			},
			{
				ConfigField: plugins.ConfigField{
					Key: FieldPassword, Label: "Password", Type: "password", Secret: true,
					Help: "An app password, if your provider issues them.",
				},
				GlobalKey: plugins.NoGlobalKey,
				Group:     credentialGroup,
			},
			{
				ConfigField: plugins.ConfigField{
					Key: FieldMailbox, Label: "Mailbox", Type: "text", Default: "INBOX",
				},
				// Seeded by the same global default the poller falls back to.
				GlobalKey: "imap_mailbox",
			},
		},
	}
}

// UserHealthCheck probes one caller's resolved mailbox. It builds no state and
// touches no worker, so it is safe to run against a non-live instance.
func (p *Plugin) UserHealthCheck(ctx context.Context, uc plugins.UserContext, cfg map[string]string) plugins.Health {
	t := IMAPTargetFrom(uc.OwnerID, cfg)
	if t.Host == "" || t.Username == "" || t.Password == "" {
		return plugins.Health{
			Status: plugins.StatusDown,
			Detail: "Enter your IMAP host, username and password to connect",
		}
	}
	return ProbeMailbox(ctx, t)
}

// IMAPTargetFrom builds a poll/probe target from a resolved per-user config. It
// is the one place the imap_* keys become an IMAPTarget, shared by the health
// probe and the poller's target list (see internal/api/integrations.go).
func IMAPTargetFrom(ownerID string, cfg map[string]string) IMAPTarget {
	port, _ := strconv.Atoi(strings.TrimSpace(cfg[FieldPort]))
	if port <= 0 {
		port = 993
	}
	return IMAPTarget{
		OwnerID:  ownerID,
		Host:     strings.TrimSpace(cfg[FieldHost]),
		Port:     port,
		Username: strings.TrimSpace(cfg[FieldUser]),
		Password: cfg[FieldPassword],
		Mailbox:  strings.TrimSpace(cfg[FieldMailbox]),
	}
}
