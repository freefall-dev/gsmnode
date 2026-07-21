package emailtosms

import (
	"bytes"
	"io"
	"strings"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // decode non-UTF-8 bodies
)

// maxBody caps how many bytes of a message we read when extracting the SMS text.
const maxBody = 256 << 10 // 256 KiB

// parseRecipient extracts the phone number from a {phone}@{domain} recipient
// address, validating the domain (case-insensitive). The local part is treated
// as a phone number: everything but digits is stripped, a single leading "+" is
// kept. Returns ("", false) when the domain does not match or no digits remain.
func parseRecipient(addr, domain string) (string, bool) {
	addr = strings.TrimSpace(strings.Trim(addr, "<>"))
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return "", false
	}
	local, host := addr[:at], addr[at+1:]
	if domain != "" && !strings.EqualFold(strings.TrimSpace(host), domain) {
		return "", false
	}
	return normalizePhone(local)
}

// normalizePhone keeps a single leading "+" and the digits of s.
func normalizePhone(s string) (string, bool) {
	s = strings.TrimSpace(s)
	plus := strings.HasPrefix(s, "+")
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "", false
	}
	if plus {
		return "+" + b.String(), true
	}
	return b.String(), true
}

// parseBody extracts the plain-text SMS body from a raw RFC 5322 message. It
// prefers a text/plain part; for a multipart message it walks the parts and
// falls back to the first textual part found. Transfer encodings and charsets
// are decoded by go-message.
func parseBody(raw []byte) string {
	ent, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		// Not a well-formed MIME entity — treat the whole thing as text.
		return strings.TrimSpace(string(raw))
	}
	if t := textFromEntity(ent); t != "" {
		return t
	}
	return ""
}

func textFromEntity(e *message.Entity) string {
	if mr := e.MultipartReader(); mr != nil {
		var firstText string
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			t := textFromEntity(part)
			ct, _, _ := part.Header.ContentType()
			if strings.EqualFold(ct, "text/plain") && strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
			if firstText == "" {
				firstText = strings.TrimSpace(t)
			}
		}
		return firstText
	}
	b, _ := io.ReadAll(io.LimitReader(e.Body, maxBody))
	return strings.TrimSpace(string(b))
}
