package api

import (
	"encoding/json"
	"testing"
)

// The Web App renders its form straight from this JSON, so the wire shape is a
// contract: a renamed or dropped key silently produces an empty form.
func TestUserConfigSpecWireShape(t *testing.T) {
	b, err := json.Marshal(e2sSpec())
	if err != nil {
		t.Fatalf("spec does not marshal: %v", err)
	}
	var got struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		EnableLabel string `json:"enableLabel"`
		Fields      []struct {
			Key               string `json:"key"`
			Label             string `json:"label"`
			Type              string `json:"type"`
			Secret            bool   `json:"secret"`
			Help              string `json:"help"`
			Default           string `json:"default"`
			GlobalKey         string `json:"globalKey"`
			Group             string `json:"group"`
			MaskWhenInherited bool   `json:"maskWhenInherited"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("spec json does not match the shape the UI reads: %v", err)
	}
	if got.Title == "" || got.EnableLabel == "" || got.Description == "" {
		t.Errorf("spec header incomplete: %+v", got)
	}
	if len(got.Fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(got.Fields))
	}
	for _, f := range got.Fields {
		if f.Key == "" || f.Label == "" || f.Type == "" {
			t.Errorf("field missing key/label/type: %+v", f)
		}
	}
	// The password field must travel with secret set, or the UI would render it
	// as plain text and the server would not mask it.
	var sawSecret bool
	for _, f := range got.Fields {
		if f.Key == "imap_password" {
			sawSecret = f.Secret
		}
	}
	if !sawSecret {
		t.Error("imap_password did not marshal with secret:true")
	}
}
