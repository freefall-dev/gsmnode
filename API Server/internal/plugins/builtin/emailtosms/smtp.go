package emailtosms

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// startSMTP binds and serves the SMTP intake server. It binds synchronously so a
// bad address surfaces as an Init error, then serves in a goroutine until the
// worker context is cancelled (Shutdown) or the server is closed.
func (p *Plugin) startSMTP(ctx context.Context) error {
	p.mu.Lock()
	c := p.cfg
	p.mu.Unlock()

	be := &smtpBackend{plugin: p}
	srv := smtp.NewServer(be)
	srv.Addr = net.JoinHostPort(c.smtpHost, strconv.Itoa(c.smtpPort))
	srv.Domain = c.domain
	srv.ReadTimeout = 30 * time.Second
	srv.WriteTimeout = 30 * time.Second
	srv.MaxMessageBytes = maxBody
	srv.MaxRecipients = 50

	if c.tlsCert != "" && c.tlsKey != "" {
		cert, err := tls.LoadX509KeyPair(c.tlsCert, c.tlsKey)
		if err != nil {
			return errors.New("load TLS keypair: " + err.Error())
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	} else {
		// No TLS: allow AUTH over the plaintext connection. Intended for a
		// loopback bind; put a TLS terminator in front for remote exposure.
		srv.AllowInsecureAuth = true
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	// Implicit TLS (SMTPS): wrap the listener so every connection is encrypted.
	// (STARTTLS on a plaintext connection also works whenever TLSConfig is set.)
	if srv.TLSConfig != nil {
		ln = tls.NewListener(ln, srv.TLSConfig)
	}

	p.mu.Lock()
	p.smtpSrv = srv
	p.smtpUp = true
	p.smtpErr = ""
	p.mu.Unlock()

	go func() {
		serveErr := srv.Serve(ln)
		p.mu.Lock()
		p.smtpUp = false
		if serveErr != nil && !errors.Is(serveErr, smtp.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
			p.smtpErr = serveErr.Error()
		}
		p.mu.Unlock()
	}()

	// Belt-and-braces: close the server when the worker context ends.
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return nil
}

// smtpBackend hands out a fresh session per connection.
type smtpBackend struct{ plugin *Plugin }

func (b *smtpBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{plugin: b.plugin, domain: b.plugin.domain()}, nil
}

// smtpSession is one SMTP conversation. It authenticates the sender to a gsmnode
// user (AUTH PLAIN), collects recipients as phone numbers, and on DATA enqueues
// one SMS per recipient owned by the authenticated user.
type smtpSession struct {
	plugin  *Plugin
	domain  string
	ownerID string
	rcpts   []string
}

func (s *smtpSession) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *smtpSession) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, username, password string) error {
		if host == nil {
			return &smtp.SMTPError{Code: 454, Message: "email-to-sms host not configured"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		id, err := host.Authenticate(ctx, username, password)
		if err != nil || id == "" {
			return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "authentication failed"}
		}
		s.ownerID = id
		return nil
	}), nil
}

func (s *smtpSession) Mail(_ string, _ *smtp.MailOptions) error { return nil }

func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	phone, ok := parseRecipient(to, s.domain)
	if !ok {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "recipient must be {phone}@" + s.domain}
	}
	s.rcpts = append(s.rcpts, phone)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	if s.ownerID == "" {
		return &smtp.SMTPError{Code: 530, EnhancedCode: smtp.EnhancedCode{5, 7, 0}, Message: "authentication required"}
	}
	if len(s.rcpts) == 0 {
		return &smtp.SMTPError{Code: 554, Message: "no valid recipients"}
	}
	raw, err := io.ReadAll(io.LimitReader(r, maxBody))
	if err != nil {
		return err
	}
	text := parseBody(raw)
	if text == "" {
		return &smtp.SMTPError{Code: 554, Message: "empty message body"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, phone := range s.rcpts {
		if err := host.EnqueueSMS(ctx, s.ownerID, []string{phone}, text); err != nil {
			return &smtp.SMTPError{Code: 451, Message: "could not enqueue SMS: " + err.Error()}
		}
	}
	return nil
}

func (s *smtpSession) Reset()        { s.rcpts = nil }
func (s *smtpSession) Logout() error { return nil }
