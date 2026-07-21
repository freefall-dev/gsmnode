package api

import (
	"encoding/json"
	"testing"

	"smsgateway/apiserver/internal/plugins"
	"smsgateway/apiserver/internal/plugins/builtin/emailtosms"
)

// The cascade is exercised against the real email-to-sms declaration rather than
// a fixture, so a change to its fields that would break the live IMAP poller
// fails here first.
func e2sSpec() plugins.UserConfigSpec { return (&emailtosms.Plugin{}).UserConfig() }

func TestGlobalLayerHonoursGlobalKeys(t *testing.T) {
	// The global config names the seeds differently from the per-user fields,
	// and credentials have no global layer at all.
	got := globalLayer(e2sSpec(), map[string]string{
		"imap_default_host": "imap.example.com",
		"imap_default_port": "993",
		"imap_mailbox":      "INBOX",
		"imap_user":         "operator@example.com", // must be ignored
		"imap_password":     "hunter2",              // must be ignored
	})
	want := map[string]string{
		emailtosms.FieldHost:    "imap.example.com",
		emailtosms.FieldPort:    "993",
		emailtosms.FieldMailbox: "INBOX",
	}
	if len(got) != len(want) {
		t.Fatalf("global layer = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("global layer[%q] = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got[emailtosms.FieldUser]; ok {
		t.Error("credentials must not resolve from the global layer")
	}
}

func TestResolveLayers(t *testing.T) {
	spec := e2sSpec()

	tests := []struct {
		name       string
		gc, oc, uc map[string]string
		hasOrg     bool
		wantEff    map[string]string
		wantSource map[string]string
	}{
		{
			name:   "user fills what global left blank",
			gc:     map[string]string{emailtosms.FieldHost: "imap.corp.com", emailtosms.FieldMailbox: "INBOX"},
			uc:     map[string]string{emailtosms.FieldUser: "me@corp.com", emailtosms.FieldPassword: "pw"},
			hasOrg: false,
			wantEff: map[string]string{
				emailtosms.FieldHost:     "imap.corp.com",
				emailtosms.FieldMailbox:  "INBOX",
				emailtosms.FieldUser:     "me@corp.com",
				emailtosms.FieldPassword: "pw",
			},
			wantSource: map[string]string{
				emailtosms.FieldHost:     "global",
				emailtosms.FieldMailbox:  "global",
				emailtosms.FieldPort:     "unset",
				emailtosms.FieldUser:     "user",
				emailtosms.FieldPassword: "user",
			},
		},
		{
			name:       "a higher layer wins per field",
			gc:         map[string]string{emailtosms.FieldHost: "global.example.com", emailtosms.FieldMailbox: "INBOX"},
			oc:         map[string]string{emailtosms.FieldHost: "org.example.com"},
			uc:         map[string]string{emailtosms.FieldMailbox: "Archive"},
			hasOrg:     true,
			wantEff:    map[string]string{emailtosms.FieldHost: "global.example.com", emailtosms.FieldMailbox: "INBOX"},
			wantSource: map[string]string{emailtosms.FieldHost: "global", emailtosms.FieldMailbox: "global"},
		},
		{
			name:       "org fills a blank the global layer left",
			gc:         map[string]string{emailtosms.FieldMailbox: "INBOX"},
			oc:         map[string]string{emailtosms.FieldHost: "org.example.com"},
			uc:         map[string]string{emailtosms.FieldHost: "mine.example.com"},
			hasOrg:     true,
			wantEff:    map[string]string{emailtosms.FieldHost: "org.example.com"},
			wantSource: map[string]string{emailtosms.FieldHost: "org"},
		},
		{
			name:       "an org-less caller never resolves through an org layer",
			oc:         map[string]string{emailtosms.FieldHost: "org.example.com"},
			uc:         map[string]string{emailtosms.FieldHost: "mine.example.com"},
			hasOrg:     false,
			wantEff:    map[string]string{emailtosms.FieldHost: "mine.example.com"},
			wantSource: map[string]string{emailtosms.FieldHost: "user"},
		},
		{
			name:       "whitespace counts as unset",
			gc:         map[string]string{emailtosms.FieldHost: "   "},
			uc:         map[string]string{emailtosms.FieldHost: "mine.example.com"},
			wantEff:    map[string]string{emailtosms.FieldHost: "mine.example.com"},
			wantSource: map[string]string{emailtosms.FieldHost: "user"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eff, source := resolveLayers(spec, tc.gc, tc.oc, tc.uc, tc.hasOrg)
			for k, want := range tc.wantEff {
				if eff[k] != want {
					t.Errorf("eff[%q] = %q, want %q", k, eff[k], want)
				}
			}
			for k, want := range tc.wantSource {
				if source[k] != want {
					t.Errorf("source[%q] = %q, want %q", k, source[k], want)
				}
			}
		})
	}
}

// Credential halves must never come from different layers: a user who supplies
// only a password must not have it paired with an org's username.
func TestResolveLayersKeepsCredentialsTogether(t *testing.T) {
	spec := e2sSpec()

	t.Run("org username wins the whole pair", func(t *testing.T) {
		eff, source := resolveLayers(spec,
			nil,
			map[string]string{emailtosms.FieldUser: "shared@corp.com", emailtosms.FieldPassword: "orgpw"},
			map[string]string{emailtosms.FieldPassword: "mypw"},
			true)
		if eff[emailtosms.FieldUser] != "shared@corp.com" {
			t.Errorf("user = %q, want the org's", eff[emailtosms.FieldUser])
		}
		if eff[emailtosms.FieldPassword] != "orgpw" {
			t.Errorf("password = %q, want the org's — halves must not mix", eff[emailtosms.FieldPassword])
		}
		if source[emailtosms.FieldPassword] != "org" {
			t.Errorf("password source = %q, want org", source[emailtosms.FieldPassword])
		}
	})

	t.Run("a user setting the username owns the pair", func(t *testing.T) {
		eff, _ := resolveLayers(spec,
			nil,
			map[string]string{emailtosms.FieldUser: "shared@corp.com", emailtosms.FieldPassword: "orgpw"},
			map[string]string{emailtosms.FieldUser: "me@corp.com", emailtosms.FieldPassword: "mypw"},
			false) // org-less: the org layer is not consulted
		if eff[emailtosms.FieldUser] != "me@corp.com" || eff[emailtosms.FieldPassword] != "mypw" {
			t.Errorf("got %q/%q, want the user's own pair", eff[emailtosms.FieldUser], eff[emailtosms.FieldPassword])
		}
	})

	t.Run("a password alone resolves to nothing", func(t *testing.T) {
		eff, source := resolveLayers(spec, nil, nil,
			map[string]string{emailtosms.FieldPassword: "mypw"}, false)
		if eff[emailtosms.FieldPassword] != "" {
			t.Errorf("password = %q, want empty without a username", eff[emailtosms.FieldPassword])
		}
		if source[emailtosms.FieldPassword] != "unset" {
			t.Errorf("password source = %q, want unset", source[emailtosms.FieldPassword])
		}
	})
}

func TestLockedFor(t *testing.T) {
	tests := []struct {
		source, editable string
		want             bool
	}{
		{"global", "user", true}, // imposed from above
		{"org", "user", true},    // imposed from above
		{"user", "user", false},  // the caller's own
		{"unset", "user", false}, // nobody set it — the caller may be first
		{"global", "org", true},  // imposed on an org admin
		{"org", "org", false},    // the org admin's own layer
		{"user", "org", false},   // below the org admin
		// A superadmin edits the global layer in the Plugins panel, never here.
		{"unset", "none", true},
		{"user", "none", true},
	}
	for _, tc := range tests {
		if got := lockedFor(tc.source, tc.editable); got != tc.want {
			t.Errorf("lockedFor(%q, %q) = %v, want %v", tc.source, tc.editable, got, tc.want)
		}
	}
}

func TestScopeViewMasking(t *testing.T) {
	spec := e2sSpec()
	res := integrationResolution{
		spec: spec,
		eff: map[string]string{
			emailtosms.FieldHost:     "imap.corp.com",
			emailtosms.FieldUser:     "shared@corp.com",
			emailtosms.FieldPassword: "orgpw",
		},
		userOwn: map[string]string{},
		orgOwn: map[string]string{
			emailtosms.FieldUser:     "shared@corp.com",
			emailtosms.FieldPassword: "orgpw",
		},
		source: map[string]string{
			emailtosms.FieldHost:     "global",
			emailtosms.FieldUser:     "org",
			emailtosms.FieldPassword: "org",
			emailtosms.FieldPort:     "unset",
			emailtosms.FieldMailbox:  "unset",
		},
	}

	fields := scopeView(res, "user")["fields"].(map[string]fieldView)

	if got := fields[emailtosms.FieldPassword].Effective; got != integrationMask {
		t.Errorf("secret effective = %q, want the mask", got)
	}
	// An inherited username is private too: masked, not echoed in clear.
	if got := fields[emailtosms.FieldUser].Effective; got != integrationMask {
		t.Errorf("inherited username effective = %q, want the mask", got)
	}
	// A non-secret field imposed from above is still shown, so the user can see
	// what is in force.
	if got := fields[emailtosms.FieldHost].Effective; got != "imap.corp.com" {
		t.Errorf("host effective = %q, want it in clear", got)
	}
	if !fields[emailtosms.FieldHost].Locked {
		t.Error("a globally-set host must be locked for a user")
	}
	if fields[emailtosms.FieldMailbox].Locked {
		t.Error("an unset field must not be locked")
	}
}

// A superadmin edits the global layer in the Plugins panel, so every field is
// locked here and no secret leaks through the view.
func TestScopeViewSuperadminLocksEverything(t *testing.T) {
	spec := e2sSpec()
	res := integrationResolution{
		spec:    spec,
		eff:     map[string]string{emailtosms.FieldHost: "imap.corp.com"},
		userOwn: map[string]string{},
		orgOwn:  map[string]string{},
		source:  map[string]string{emailtosms.FieldHost: "global"},
	}
	for key, f := range scopeView(res, "none")["fields"].(map[string]fieldView) {
		if !f.Locked {
			t.Errorf("field %q not locked for a superadmin", key)
		}
	}
}

// Saving one plugin's settings must not disturb another's entry in the shared
// pluginSettings blob.
func TestMergeIntegrationPreservesOtherPlugins(t *testing.T) {
	existing := json.RawMessage(`{"other-plugin":{"config":{"k":"v"},"enabled":true}}`)

	out := mergeIntegration(existing, emailtosms.Name, func(st *integrationStored) {
		st.Enabled = true
		st.Config = map[string]string{emailtosms.FieldHost: "imap.corp.com"}
	})

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("merged blob is not valid json: %v", err)
	}
	if _, ok := doc["other-plugin"]; !ok {
		t.Fatal("other-plugin entry was dropped")
	}
	var other integrationStored
	if err := json.Unmarshal(doc["other-plugin"], &other); err != nil {
		t.Fatalf("other-plugin entry corrupted: %v", err)
	}
	if other.Config["k"] != "v" || !other.Enabled {
		t.Errorf("other-plugin entry changed: %+v", other)
	}

	got := decodeIntegration(out, emailtosms.Name)
	if !got.Enabled || got.Config[emailtosms.FieldHost] != "imap.corp.com" {
		t.Errorf("round-trip lost our own settings: %+v", got)
	}
}

// The stored shape predates the generic cascade; records written by the previous
// hardcoded implementation must still decode.
func TestDecodeIntegrationReadsLegacyRecords(t *testing.T) {
	legacy := json.RawMessage(`{"email-to-sms":{"config":{"imap_host":"imap.old.com","imap_user":"me@old.com","imap_password":"pw","imap_port":"993","imap_mailbox":"INBOX"},"enabled":true}}`)

	got := decodeIntegration(legacy, emailtosms.Name)
	if !got.Enabled {
		t.Error("legacy enabled flag lost")
	}
	if got.Config[emailtosms.FieldHost] != "imap.old.com" || got.Config[emailtosms.FieldUser] != "me@old.com" {
		t.Errorf("legacy config lost: %+v", got.Config)
	}

	// And it still resolves into a usable poll target.
	eff, _ := resolveLayers(e2sSpec(), nil, nil, got.Config, false)
	target := emailtosms.IMAPTargetFrom("user123", eff)
	if target.Host != "imap.old.com" || target.Username != "me@old.com" || target.Password != "pw" || target.Port != 993 {
		t.Errorf("legacy record does not resolve to a poll target: %+v", target)
	}
}

func TestDecodeIntegrationTolerance(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, {}, json.RawMessage(`null`), json.RawMessage(`not json`), json.RawMessage(`{}`), json.RawMessage(`{"other":{}}`)} {
		got := decodeIntegration(raw, emailtosms.Name)
		if got.Enabled || len(got.Config) != 0 {
			t.Errorf("decodeIntegration(%s) = %+v, want zero value", raw, got)
		}
	}
}

func TestIMAPTargetFromDefaults(t *testing.T) {
	t.Run("blank port falls back to 993", func(t *testing.T) {
		if got := emailtosms.IMAPTargetFrom("u", map[string]string{}).Port; got != 993 {
			t.Errorf("port = %d, want 993", got)
		}
	})
	t.Run("unparseable port falls back to 993", func(t *testing.T) {
		if got := emailtosms.IMAPTargetFrom("u", map[string]string{emailtosms.FieldPort: "abc"}).Port; got != 993 {
			t.Errorf("port = %d, want 993", got)
		}
	})
	t.Run("a set port is honoured", func(t *testing.T) {
		if got := emailtosms.IMAPTargetFrom("u", map[string]string{emailtosms.FieldPort: "1143"}).Port; got != 1143 {
			t.Errorf("port = %d, want 1143", got)
		}
	})
}
