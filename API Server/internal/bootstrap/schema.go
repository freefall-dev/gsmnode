package bootstrap

// This file is the Go mirror of the DESIRED schema, create order, reconcile
// order and INDEXES in scripts/setup-pocketbase.mjs. Keep the two in sync: edit
// here and re-run the server (bootstrap applies on startup), or run the script.
//
// Access rules are left null on every collection on purpose — every client goes
// through the API Server, which authenticates as a superuser, so the database is
// never exposed directly.

// jsonMaxSize matches the default json field cap in setup-pocketbase.mjs (2 MB).
const jsonMaxSize = 2000000

// WebhookEvents is the canonical set of events a webhook may subscribe to. It
// backs both the webhooks.event select below and the API Server's registration
// whitelist, so the two cannot drift — an event missing from either one is
// dispatched but unsubscribable.
var WebhookEvents = []string{
	"sms:received", "sms:sent", "sms:delivered", "sms:failed",
	"sms:data-received", "mms:received", "mms:downloaded",
	"call:received", "call:sent", "call:failed",
}

// collectionsSchema is the desired field set per collection.
var collectionsSchema = map[string][]fieldDef{
	// Tenants users belong to. A superadmin spans all of them; an admin manages
	// only their own. Names are unique so the API can rely on PocketBase
	// rejecting a duplicate.
	"organizations": {
		fText("name", true),
		// Org-layer plugin/integration settings (cascade L2). See internal/api/integrations.go.
		fJSON("pluginSettings", false, 100000),
		fAutodate("created", true, false),
	},
	// Custom fields layered onto the built-in "users" auth collection. Only role
	// and organization are added — the email/password system fields are kept.
	// Empty role is treated as "user" by the API.
	"users": {
		fSelect("role", []string{"user", "admin", "superadmin"}, false),
		// Organization membership. Non-cascading: deleting an org keeps its people.
		fRelation("organization", "organizations", false, false),
		// User-layer plugin/integration settings (cascade L3). See internal/api/integrations.go.
		fJSON("pluginSettings", false, 100000),
	},
	// Registered phones. auth_token is the device's bearer credential.
	"devices": {
		fText("device_id", true),
		fText("name", false),
		fText("platform", false),
		fText("app_version", false),
		fText("push_token", false),
		fText("auth_token", true),
		fSelect("status", []string{"online", "offline"}, false),
		fDate("last_seen_at", false),
		// [{slot, subscription_id, carrier, number, display_name}]
		fJSON("sims", false, jsonMaxSize),
		// Owner of this device. Cascades: deleting a user wipes their devices.
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
		fAutodate("updated", true, true),
	},
	// Outbound SMS / call / data-SMS / MMS requests dispatched to devices.
	"messages": {
		fJSON("phone_numbers", true, jsonMaxSize),
		fText("text_message", false),
		fSelect("type", []string{"sms", "call", "data", "mms"}, false),
		fNumber("sim_number"),
		fSelect("status", []string{"Pending", "Processed", "Sent", "Delivered", "Failed"}, false),
		fText("error", false),
		// Data SMS: base64-encoded binary payload sent to a destination port.
		fText("data_payload", false),
		fNumber("data_port"),
		// MMS: subject line and attachments [{filename, content_type, data(base64)}].
		fText("subject", false),
		fJSON("attachments", false, jsonMaxSize),
		// When true, phone_numbers and text_message hold client-encrypted
		// ciphertext (E2E); the server stores/relays them without reading them.
		fBool("encrypted"),
		fDate("schedule_at", false),
		fDate("sent_at", false),
		fDate("delivered_at", false),
		fRelation("device", "devices", false, false),
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
		fAutodate("updated", true, true),
	},
	// Inbound SMS / data-SMS / MMS received on a device.
	"inbox": {
		fText("phone_number", true),
		fText("message", false),
		fSelect("type", []string{"sms", "data", "mms"}, false),
		fDate("received_at", false),
		fNumber("sim_slot"), // 0-based SIM slot the message arrived on
		// Data SMS payload/port and MMS subject/attachments (mirrors messages).
		fText("data_payload", false),
		fNumber("data_port"),
		fText("subject", false),
		fJSON("attachments", false, jsonMaxSize),
		// When true, phone_number and message hold client-encrypted ciphertext.
		fBool("encrypted"),
		fRelation("device", "devices", false, false),
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
	},
	// Inbound / outbound call log, reported by devices.
	"calls": {
		fText("phone_number", true),
		fSelect("direction", []string{"incoming", "outgoing"}, false),
		fSelect("status", []string{"ringing", "missed", "answered", "completed", "rejected", "failed"}, false),
		fNumber("sim_slot"),
		fNumber("duration"), // seconds, when known
		fDate("started_at", false),
		fRelation("device", "devices", false, false),
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
	},
	// Per-owner webhook subscriptions for message/call lifecycle events.
	"webhooks": {
		fSelect("event", WebhookEvents, false),
		fURL("url", true),
		// Shared secret the subscriber supplies at registration; deliveries to
		// this URL are HMAC-signed with it. Optional, so an unsigned webhook
		// registered before this existed keeps working.
		fText("secret", false),
		fRelation("device", "devices", false, false),
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
	},
}

// createOrder is the dependency order for creating missing collections.
// "users" is PocketBase's built-in auth collection and is never created here.
var createOrder = []string{
	"organizations",
	"devices",
	"messages",
	"inbox",
	"calls",
	"webhooks",
}

// reconcileOrder additionally includes "users" so its custom fields (role,
// organization) are added to the built-in collection.
var reconcileOrder = []string{
	"organizations",
	"users",
	"devices",
	"messages",
	"inbox",
	"calls",
	"webhooks",
}

// indexes are extra SQL indexes applied at collection-create time.
var indexes = map[string][]string{
	"organizations": {"CREATE UNIQUE INDEX idx_organizations_name ON organizations (name)"},
	"devices": {
		"CREATE UNIQUE INDEX idx_devices_auth_token ON devices (auth_token)",
		"CREATE UNIQUE INDEX idx_devices_owner_device ON devices (owner, device_id)",
	},
	"messages": {
		"CREATE INDEX idx_messages_device_status ON messages (device, status)",
		"CREATE INDEX idx_messages_owner ON messages (owner)",
	},
	"inbox":    {"CREATE INDEX idx_inbox_owner ON inbox (owner)"},
	"calls":    {"CREATE INDEX idx_calls_owner ON calls (owner)"},
	"webhooks": {"CREATE INDEX idx_webhooks_owner_event ON webhooks (owner, event)"},
}
