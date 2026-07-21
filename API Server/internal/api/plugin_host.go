package api

import (
	"context"

	"smsgateway/apiserver/internal/plugins/builtin/emailtosms"
)

// emailHost implements emailtosms.Host over the API Server, letting the
// email-to-sms plugin authenticate senders, enqueue SMS, and enumerate the
// per-user IMAP mailboxes to poll — without the plugin importing this package.
type emailHost struct{ s *Server }

// Authenticate verifies gsmnode credentials (the SMTP AUTH pair) against the
// users collection and returns the user's record id.
func (h *emailHost) Authenticate(ctx context.Context, email, password string) (string, error) {
	res, err := h.s.pb.AuthWithPassword(ctx, colUsers, email, password)
	if err != nil {
		return "", err
	}
	return asString(res.Record["id"]), nil
}

// EnqueueSMS queues an outbound SMS owned by ownerID (same path as POST /api/messages).
func (h *emailHost) EnqueueSMS(ctx context.Context, ownerID string, phones []string, text string) error {
	_, err := h.s.createSMS(ctx, ownerID, phones, text)
	return err
}

// IMAPTargets resolves the per-user mailboxes to poll (see integrations.go).
func (h *emailHost) IMAPTargets(ctx context.Context) ([]emailtosms.IMAPTarget, error) {
	return h.s.emailToSMSIMAPTargets(ctx)
}
