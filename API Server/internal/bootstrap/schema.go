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

// collectionsSchema is the desired field set per collection.
var collectionsSchema = map[string][]fieldDef{
	// Tenants users belong to. A superadmin spans all of them; an admin manages
	// only their own. Names are unique so the API can rely on PocketBase
	// rejecting a duplicate.
	"organizations": {
		fText("name", true),
		fAutodate("created", true, false),
	},
	// Custom fields layered onto the built-in "users" auth collection. Only role
	// and organization are added — the email/password system fields are kept.
	// Empty role is treated as "user" by the API.
	"users": {
		fSelect("role", []string{"user", "admin", "superadmin"}, false),
		// Organization membership. Non-cascading: deleting an org keeps its people.
		fRelation("organization", "organizations", false, false),
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
	// Outbound SMS / call requests dispatched to devices.
	"messages": {
		fJSON("phone_numbers", true, jsonMaxSize),
		fText("text_message", false),
		fSelect("type", []string{"sms", "call"}, false),
		fNumber("sim_number"),
		fSelect("status", []string{"Pending", "Processed", "Sent", "Delivered", "Failed"}, false),
		fText("error", false),
		fDate("schedule_at", false),
		fDate("sent_at", false),
		fDate("delivered_at", false),
		fRelation("device", "devices", false, false),
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
		fAutodate("updated", true, true),
	},
	// Inbound SMS received on a device.
	"inbox": {
		fText("phone_number", true),
		fText("message", false),
		fDate("received_at", false),
		fNumber("sim_slot"), // 0-based SIM slot the message arrived on
		fRelation("device", "devices", false, false),
		fRelation("owner", "users", true, true),
		fAutodate("created", true, false),
	},
	// Per-owner webhook subscriptions for message lifecycle events.
	"webhooks": {
		fSelect("event", []string{"sms:received", "sms:sent", "sms:delivered", "sms:failed"}, false),
		fURL("url", true),
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
	"webhooks": {"CREATE INDEX idx_webhooks_owner_event ON webhooks (owner, event)"},
}
