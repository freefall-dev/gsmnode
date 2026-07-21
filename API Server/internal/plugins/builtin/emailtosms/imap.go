package emailtosms

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"smsgateway/apiserver/internal/plugins"
)

// ProbeMailbox tries to connect, log in and select a target's mailbox, returning
// a classified Health. It is a synchronous, side-effect-free credential check
// used by the per-user integration health endpoint (see internal/api/integrations.go).
func ProbeMailbox(_ context.Context, t IMAPTarget) plugins.Health {
	port := t.Port
	if port == 0 {
		port = 993
	}
	mailbox := t.Mailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}
	start := time.Now()
	c, err := imapclient.DialTLS(net.JoinHostPort(t.Host, strconv.Itoa(port)), nil)
	if err != nil {
		return plugins.Health{Status: plugins.StatusDown, Detail: "connect: " + err.Error()}
	}
	defer c.Close()
	if err := c.Login(t.Username, t.Password).Wait(); err != nil {
		return plugins.Health{Status: plugins.StatusDown, LatencyMs: time.Since(start).Milliseconds(), Detail: "login failed"}
	}
	defer func() { _ = c.Logout().Wait() }()
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return plugins.Health{Status: plugins.StatusDegraded, LatencyMs: time.Since(start).Milliseconds(), Detail: "mailbox " + mailbox + " not selectable"}
	}
	return plugins.Health{Status: plugins.StatusOK, LatencyMs: time.Since(start).Milliseconds(), Detail: "mailbox reachable"}
}

// startIMAP launches the poll loop. It polls once immediately, then on the
// configured interval, until the worker context is cancelled.
func (p *Plugin) startIMAP(ctx context.Context) {
	p.mu.Lock()
	interval := p.cfg.pollInterval
	p.mu.Unlock()
	if interval <= 0 {
		interval = 60 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		p.pollAll(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.pollAll(ctx)
			}
		}
	}()
}

// pollAll resolves the per-user mailboxes and polls each one.
func (p *Plugin) pollAll(ctx context.Context) {
	if host == nil {
		p.setIMAPErr("host adapter not configured")
		return
	}
	targets, err := host.IMAPTargets(ctx)
	if err != nil {
		p.setIMAPErr(err.Error())
		return
	}
	var lastErr string
	for _, t := range targets {
		if ctx.Err() != nil {
			return
		}
		if err := p.pollMailbox(ctx, t); err != nil {
			lastErr = err.Error()
		}
	}
	p.mu.Lock()
	p.imapErr = lastErr
	p.pollOK = lastErr == ""
	p.lastPoll = time.Now()
	p.mu.Unlock()
}

// pollMailbox connects to one user's mailbox, converts each unseen message into
// an SMS owned by that user, and marks the processed messages seen.
func (p *Plugin) pollMailbox(ctx context.Context, t IMAPTarget) error {
	port := t.Port
	if port == 0 {
		port = 993
	}
	mailbox := t.Mailbox
	if mailbox == "" {
		p.mu.Lock()
		mailbox = p.cfg.imapMailbox
		p.mu.Unlock()
	}
	if mailbox == "" {
		mailbox = "INBOX"
	}
	domain := p.domain()

	addr := net.JoinHostPort(t.Host, strconv.Itoa(port))
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Login(t.Username, t.Password).Wait(); err != nil {
		return err
	}
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}

	criteria := &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}
	sd, err := c.Search(criteria, &imap.SearchOptions{ReturnAll: true}).Wait()
	if err != nil {
		return err
	}
	nums := sd.AllSeqNums()
	if len(nums) == 0 {
		return nil
	}

	seqSet := imap.SeqSetNum(nums...)
	fetchOpts := &imap.FetchOptions{
		Envelope:    true,
		BodySection: []*imap.FetchItemBodySection{{}},
	}
	msgs, err := c.Fetch(seqSet, fetchOpts).Collect()
	if err != nil {
		return err
	}

	var processed []uint32
	for _, m := range msgs {
		phone := recipientFromEnvelope(m.Envelope, domain)
		text := ""
		for _, body := range m.BodySection {
			text = parseBody(body.Bytes)
			break
		}
		if phone == "" || text == "" {
			continue
		}
		if err := host.EnqueueSMS(ctx, t.OwnerID, []string{phone}, text); err != nil {
			continue // leave it unseen so a later poll retries
		}
		processed = append(processed, m.SeqNum)
	}

	if len(processed) > 0 {
		store := &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}}
		_ = c.Store(imap.SeqSetNum(processed...), store, nil).Close()
	}
	return nil
}

// recipientFromEnvelope pulls the first {phone}@{domain} recipient from the
// message envelope's To list.
func recipientFromEnvelope(env *imap.Envelope, domain string) string {
	if env == nil {
		return ""
	}
	for _, a := range env.To {
		if phone, ok := parseRecipient(a.Mailbox+"@"+a.Host, domain); ok {
			return phone
		}
	}
	return ""
}

func (p *Plugin) setIMAPErr(msg string) {
	p.mu.Lock()
	p.imapErr = msg
	p.pollOK = false
	p.lastPoll = time.Now()
	p.mu.Unlock()
}
