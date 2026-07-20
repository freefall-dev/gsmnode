// Sets up the PocketBase collections the API Server expects.
//
// Usage (PowerShell):
//   $env:POCKETBASE_URL="http://10.2.1.10:8028"
//   $env:PB_ADMIN_EMAIL="you@example.com"
//   $env:PB_ADMIN_PASSWORD="your-password"
//   node scripts/setup-pocketbase.mjs
//
// It is idempotent: existing collections are updated, missing ones created.
// Requires Node 18+ (uses global fetch). No npm install needed.

const BASE = (process.env.POCKETBASE_URL || "http://10.2.1.10:8028").replace(/\/$/, "");
const EMAIL = process.env.PB_ADMIN_EMAIL;
const PASSWORD = process.env.PB_ADMIN_PASSWORD;

if (!EMAIL || !PASSWORD) {
  console.error("Set PB_ADMIN_EMAIL and PB_ADMIN_PASSWORD environment variables.");
  process.exit(1);
}

let token = "";

async function api(method, path, body) {
  const res = await fetch(BASE + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: token } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json = null;
  try { json = text ? JSON.parse(text) : null; } catch { /* non-JSON */ }
  if (!res.ok) {
    const msg = json?.message || text || res.statusText;
    throw new Error(`${method} ${path} -> ${res.status}: ${msg}`);
  }
  return json;
}

async function authenticate() {
  const res = await api("POST", "/api/collections/_superusers/auth-with-password", {
    identity: EMAIL,
    password: PASSWORD,
  });
  token = res.token;
  console.log("Authenticated as superuser.");
}

async function getCollections() {
  const res = await api("GET", "/api/collections?perPage=200");
  const byName = {};
  for (const c of res.items) byName[c.name] = c;
  return byName;
}

// Field builders for PocketBase v0.23+ (collections use a `fields` array).
const f = {
  text: (name, opts = {}) => ({ name, type: "text", required: false, ...opts }),
  number: (name, opts = {}) => ({ name, type: "number", required: false, ...opts }),
  date: (name, opts = {}) => ({ name, type: "date", required: false, ...opts }),
  json: (name, opts = {}) => ({ name, type: "json", required: false, maxSize: 2000000, ...opts }),
  url: (name, opts = {}) => ({ name, type: "url", required: false, ...opts }),
  select: (name, values, opts = {}) => ({
    name, type: "select", required: false, maxSelect: 1, values, ...opts,
  }),
  relation: (name, collectionId, opts = {}) => ({
    name, type: "relation", required: false, collectionId,
    cascadeDelete: false, maxSelect: 1, minSelect: 0, ...opts,
  }),
  autodate: (name, opts) => ({ name, type: "autodate", onCreate: true, onUpdate: false, ...opts }),
};

// Superuser-only API rules: only the API Server (acting as superuser) reads or
// writes these collections. Clients never touch PocketBase directly.
const LOCKED = { listRule: null, viewRule: null, createRule: null, updateRule: null, deleteRule: null };

function definitions(ids) {
  return [
    {
      name: "devices",
      type: "base",
      ...LOCKED,
      fields: [
        f.text("device_id", { required: true }),
        f.text("name"),
        f.text("platform"),
        f.text("app_version"),
        f.text("push_token"),
        f.text("auth_token", { required: true }),
        f.select("status", ["online", "offline"]),
        f.date("last_seen_at"),
        f.json("sims"), // [{slot, subscription_id, carrier, number, display_name}]
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
        f.autodate("updated", { onCreate: true, onUpdate: true }),
      ],
      indexes: [
        "CREATE UNIQUE INDEX idx_devices_auth_token ON devices (auth_token)",
        "CREATE UNIQUE INDEX idx_devices_owner_device ON devices (owner, device_id)",
      ],
    },
    {
      name: "messages",
      type: "base",
      ...LOCKED,
      fields: [
        f.json("phone_numbers", { required: true }),
        f.text("text_message"),
        f.select("type", ["sms", "call"]),
        f.number("sim_number"),
        f.select("status", ["Pending", "Processed", "Sent", "Delivered", "Failed"]),
        f.text("error"),
        f.date("schedule_at"),
        f.date("sent_at"),
        f.date("delivered_at"),
        f.relation("device", ids.devices, { cascadeDelete: false }),
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
        f.autodate("updated", { onCreate: true, onUpdate: true }),
      ],
      indexes: [
        "CREATE INDEX idx_messages_device_status ON messages (device, status)",
        "CREATE INDEX idx_messages_owner ON messages (owner)",
      ],
    },
    {
      name: "inbox",
      type: "base",
      ...LOCKED,
      fields: [
        f.text("phone_number", { required: true }),
        f.text("message"),
        f.date("received_at"),
        f.number("sim_slot"), // 0-based SIM slot the message arrived on
        f.relation("device", ids.devices, { cascadeDelete: false }),
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
      ],
      indexes: ["CREATE INDEX idx_inbox_owner ON inbox (owner)"],
    },
    {
      name: "webhooks",
      type: "base",
      ...LOCKED,
      fields: [
        f.select("event", ["sms:received", "sms:sent", "sms:delivered", "sms:failed"]),
        f.url("url", { required: true }),
        f.relation("device", ids.devices, { cascadeDelete: false }),
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
      ],
      indexes: ["CREATE INDEX idx_webhooks_owner_event ON webhooks (owner, event)"],
    },
  ];
}

// The API Server gates the panel and management endpoints on a users.role
// select field (user | admin | superadmin). It is added to the existing auth
// collection by appending to its current fields, so the built-in email/password
// system fields are preserved. Idempotent: a no-op once role exists.
async function ensureUsersRole(users) {
  if ((users.fields || []).some((fld) => fld.name === "role")) {
    console.log('Collection "users" already has a role field.');
    return;
  }
  const fields = [
    ...users.fields,
    { name: "role", type: "select", required: false, maxSelect: 1, values: ["user", "admin", "superadmin"] },
  ];
  await api("PATCH", `/api/collections/${users.id}`, { fields });
  console.log('Added role field to "users" collection.');
}

async function main() {
  await authenticate();

  let collections = await getCollections();
  if (!collections.users) {
    throw new Error('The default "users" auth collection was not found in PocketBase.');
  }
  await ensureUsersRole(collections.users);

  // Resolve collection ids needed for relation fields. devices must exist before
  // messages/inbox/webhooks reference it, so create in order, refreshing ids.
  const order = ["devices", "messages", "inbox", "webhooks"];
  for (const name of order) {
    const ids = {
      users: collections.users.id,
      devices: collections.devices?.id,
    };
    const def = definitions(ids).find((d) => d.name === name);

    if (collections[name]) {
      await api("PATCH", `/api/collections/${collections[name].id}`, def);
      console.log(`Updated collection: ${name}`);
    } else {
      await api("POST", "/api/collections", def);
      console.log(`Created collection: ${name}`);
    }
    collections = await getCollections(); // refresh so later relations resolve
  }

  console.log("\nPocketBase setup complete.");
  console.log("Collections: users (existing), devices, messages, inbox, webhooks.");
  console.log("\nNext: create a user to log in with, e.g. via the PocketBase admin UI,");
  console.log("or run scripts/create-user.mjs.");
}

main().catch((err) => {
  console.error("\nSetup failed:", err.message);
  process.exit(1);
});
