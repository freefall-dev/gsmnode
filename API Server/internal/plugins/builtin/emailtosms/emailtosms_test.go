package emailtosms

import (
	"context"
	"strings"
	"testing"
)

func TestParseRecipient(t *testing.T) {
	const domain = "sms.gsmnode.local"
	cases := []struct {
		in       string
		want     string
		wantOK   bool
		domainOK bool
	}{
		{"15551234567@sms.gsmnode.local", "15551234567", true, true},
		{"+15551234567@sms.gsmnode.local", "+15551234567", true, true},
		{"<15551234567@SMS.GSMNODE.LOCAL>", "15551234567", true, true},
		{"(555) 123-4567@sms.gsmnode.local", "5551234567", true, true},
		{"15551234567@other.example", "", false, false},
		{"not-an-address", "", false, false},
		{"noatdigits@sms.gsmnode.local", "", false, true},
	}
	for _, c := range cases {
		got, ok := parseRecipient(c.in, domain)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseRecipient(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestParseBodyPlain(t *testing.T) {
	raw := "From: a@b.com\r\nTo: 1@sms.gsmnode.local\r\nSubject: hi\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nHello SMS world\r\n"
	if got := parseBody([]byte(raw)); got != "Hello SMS world" {
		t.Errorf("parseBody = %q, want %q", got, "Hello SMS world")
	}
}

func TestParseBodyMultipart(t *testing.T) {
	raw := "MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=BB\r\n\r\n" +
		"--BB\r\nContent-Type: text/html\r\n\r\n<p>ignore me</p>\r\n" +
		"--BB\r\nContent-Type: text/plain\r\n\r\nplain wins\r\n" +
		"--BB--\r\n"
	if got := parseBody([]byte(raw)); got != "plain wins" {
		t.Errorf("parseBody multipart = %q, want %q", got, "plain wins")
	}
}

// fakeHost records EnqueueSMS calls for the SMTP session test.
type fakeHost struct {
	users map[string]string // email -> id
	sent  []string
}

func (f *fakeHost) Authenticate(_ context.Context, email, _ string) (string, error) {
	if id, ok := f.users[email]; ok {
		return id, nil
	}
	return "", context.Canceled
}
func (f *fakeHost) EnqueueSMS(_ context.Context, owner string, phones []string, text string) error {
	f.sent = append(f.sent, owner+":"+strings.Join(phones, ",")+":"+text)
	return nil
}
func (f *fakeHost) IMAPTargets(context.Context) ([]IMAPTarget, error) { return nil, nil }

func TestSMTPSessionEnqueues(t *testing.T) {
	fh := &fakeHost{users: map[string]string{"u@x.com": "user123"}}
	UseHost(fh)
	t.Cleanup(func() { host = nil })

	sess := &smtpSession{plugin: &Plugin{}, domain: "sms.gsmnode.local"}
	srv, err := sess.Auth("PLAIN")
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if _, _, err := srv.Next([]byte("\x00u@x.com\x00pw")); err != nil {
		t.Fatalf("PLAIN next: %v", err)
	}
	if sess.ownerID != "user123" {
		t.Fatalf("ownerID = %q, want user123", sess.ownerID)
	}
	if err := sess.Rcpt("15551234567@sms.gsmnode.local", nil); err != nil {
		t.Fatalf("Rcpt: %v", err)
	}
	body := "Content-Type: text/plain\r\n\r\nhi there"
	if err := sess.Data(strings.NewReader(body)); err != nil {
		t.Fatalf("Data: %v", err)
	}
	if len(fh.sent) != 1 || fh.sent[0] != "user123:15551234567:hi there" {
		t.Fatalf("sent = %v", fh.sent)
	}
}
