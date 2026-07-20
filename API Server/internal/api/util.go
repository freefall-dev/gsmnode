package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// asString coerces an arbitrary JSON value to a string.
func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return ""
	}
}

// asBool coerces a JSON value to a bool (PocketBase returns bool fields as bool).
func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	}
	return false
}

// asAttachments coerces a stored JSON attachments value into typed attachments.
func asAttachments(v any) []attachment {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []attachment
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// asInt coerces a JSON number/string to an int.
func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n
		}
	}
	return 0
}

// SIM slots are 0-based, but they are stored 1-based (slot+1) in PocketBase
// number fields. PocketBase returns 0 (not null) for an unset number field, so a
// raw 0-based slot at rest can't be told apart from "not selected". Encoding as
// slot+1 makes an absent/zero stored value unambiguously mean "unset", while the
// external API stays 0-based.

// packSlot encodes a 0-based slot for storage.
func packSlot(slot int) int { return slot + 1 }

// unpackSlot decodes a stored slot back to a 0-based slot, or nil when unset.
func unpackSlot(v any) *int {
	n := asInt(v)
	if n <= 0 {
		return nil
	}
	slot := n - 1
	return &slot
}

// asStringSlice coerces a JSON array (or single string) to []string.
func asStringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	}
	return nil
}

// queryInt parses a query parameter as an int, falling back to def.
func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
