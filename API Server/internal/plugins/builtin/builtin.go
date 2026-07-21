// Package builtin blank-imports every built-in plugin so their init() functions
// register them with the plugin registry. Import this package once (from the api
// package) to make all built-in connectors available.
//
// Built-in connectors that ship today: email-to-sms (imported below). Add another
// under internal/plugins/builtin/<name>/ and blank-import it here, e.g.
//
//	import _ "smsgateway/apiserver/internal/plugins/builtin/acme"
//
// Plugins can also be added at runtime as the "external" HTTP kind, which needs
// no rebuild. See ../README.md.
package builtin

import (
	// email-to-sms — turn inbound email into outbound SMS (SMTP server + IMAP poll).
	_ "smsgateway/apiserver/internal/plugins/builtin/emailtosms"
)
