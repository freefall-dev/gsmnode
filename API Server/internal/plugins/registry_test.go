package plugins_test

import (
	"path/filepath"
	"testing"

	"smsgateway/apiserver/internal/plugins"
	_ "smsgateway/apiserver/internal/plugins/builtin" // register built-ins
)

// TestBuiltinsRegistered verifies that blank-importing the builtin package makes
// the email-to-sms plugin discoverable through a fresh Manager, with its config
// form fields present — the contract the panel relies on.
func TestBuiltinsRegistered(t *testing.T) {
	m := plugins.NewManager(filepath.Join(t.TempDir(), "plugins.json"))

	v, ok := m.Get("email-to-sms")
	if !ok {
		t.Fatal("email-to-sms not registered")
	}
	if v.Kind != plugins.KindBuiltin {
		t.Errorf("kind = %q, want builtin", v.Kind)
	}
	if v.Enabled {
		t.Error("a freshly registered plugin should be disabled")
	}

	want := map[string]bool{"intake_mode": false, "domain": false, "smtp_port": false}
	for _, f := range v.ConfigFields {
		if _, tracked := want[f.Key]; tracked {
			want[f.Key] = true
		}
	}
	for key, seen := range want {
		if !seen {
			t.Errorf("config field %q missing from descriptor", key)
		}
	}
}
