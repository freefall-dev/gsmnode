// Sets up the PocketBase collections the API Server expects.
//
// Usage (PowerShell):
//   $env:POCKETBASE_URL="http://localhost:8028"
//   $env:PB_ADMIN_EMAIL="you@example.com"
//   $env:PB_ADMIN_PASSWORD="your-password"
//   node scripts/setup-pocketbase.mjs
//
// It is idempotent: existing collections are updated, missing ones created.
// Requires Node 18+ (uses global fetch). No npm install needed.

const BASE = (process.env.POCKETBASE_URL || "http://localhost:8028").replace(/\/$/, "");
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
  bool: (name, opts = {}) => ({ name, type: "bool", required: false, ...opts }),
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
        f.select("type", ["sms", "call", "data", "mms"]),
        f.number("sim_number"),
        f.select("status", ["Pending", "Processed", "Sent", "Delivered", "Failed"]),
        f.text("error"),
        f.text("data_payload"), // base64 binary payload for data SMS
        f.number("data_port"),
        f.text("subject"), // MMS subject
        f.json("attachments"), // MMS [{filename, content_type, data(base64)}]
        f.bool("encrypted"), // phone_numbers + text_message are E2E ciphertext
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
        f.select("type", ["sms", "data", "mms"]),
        f.date("received_at"),
        f.number("sim_slot"), // 0-based SIM slot the message arrived on
        f.text("data_payload"), // base64 binary payload for data SMS
        f.number("data_port"),
        f.text("subject"), // MMS subject
        f.json("attachments"), // MMS [{filename, content_type, data(base64)}]
        f.bool("encrypted"), // phone_number + message are E2E ciphertext
        f.relation("device", ids.devices, { cascadeDelete: false }),
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
      ],
      indexes: ["CREATE INDEX idx_inbox_owner ON inbox (owner)"],
    },
    {
      name: "calls",
      type: "base",
      ...LOCKED,
      fields: [
        f.text("phone_number", { required: true }),
        f.select("direction", ["incoming", "outgoing"]),
        f.select("status", ["ringing", "missed", "answered", "completed", "rejected", "failed"]),
        f.number("sim_slot"),
        f.number("duration"), // seconds, when known
        f.date("started_at"),
        f.relation("device", ids.devices, { cascadeDelete: false }),
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
      ],
      indexes: ["CREATE INDEX idx_calls_owner ON calls (owner)"],
    },
    {
      name: "webhooks",
      type: "base",
      ...LOCKED,
      fields: [
        f.select("event", [
          "sms:received", "sms:sent", "sms:delivered", "sms:failed",
          "sms:data-received", "mms:received", "mms:downloaded",
          "call:received", "call:sent", "call:failed",
        ]),
        f.url("url", { required: true }),
        f.relation("device", ids.devices, { cascadeDelete: false }),
        f.relation("owner", ids.users, { required: true, cascadeDelete: true }),
        f.autodate("created", { onCreate: true, onUpdate: false }),
      ],
      indexes: ["CREATE INDEX idx_webhooks_owner_event ON webhooks (owner, event)"],
    },
  ];
}

// The tenants users belong to. A superadmin spans all of them; an admin manages
// only their own. Names are unique so the API can rely on PocketBase rejecting a
// duplicate. Superuser-locked like every other collection.
const orgDefinition = {
  name: "organizations",
  type: "base",
  ...LOCKED,
  fields: [
    f.text("name", { required: true }),
    // Org-layer plugin/integration settings (cascade L2). See internal/api/integrations.go.
    f.json("pluginSettings", { maxSize: 100000 }),
    f.autodate("created", { onCreate: true, onUpdate: false }),
  ],
  indexes: ["CREATE UNIQUE INDEX idx_organizations_name ON organizations (name)"],
};

// The API Server gates the panel and management endpoints on a users.role select
// field (user | admin | superadmin) and scopes admins by a users.organization
// relation. Both are appended to the existing auth collection, preserving the
// built-in email/password system fields. Idempotent: a no-op once both exist.
async function ensureUserFields(users, orgCollectionId) {
  const have = new Set((users.fields || []).map((fld) => fld.name));
  const additions = [];
  if (!have.has("role")) {
    additions.push({ name: "role", type: "select", required: false, maxSelect: 1, values: ["user", "admin", "superadmin"] });
  }
  if (!have.has("organization")) {
    additions.push(f.relation("organization", orgCollectionId));
  }
  if (!have.has("pluginSettings")) {
    // User-layer plugin/integration settings (cascade L3). See internal/api/integrations.go.
    additions.push(f.json("pluginSettings", { maxSize: 100000 }));
  }
  if (additions.length === 0) {
    console.log('Collection "users" already has role + organization + pluginSettings fields.');
    return;
  }
  await api("PATCH", `/api/collections/${users.id}`, { fields: [...users.fields, ...additions] });
  console.log(`Added to "users": ${additions.map((a) => a.name).join(", ")}.`);
}

async function main() {
  await authenticate();

  let collections = await getCollections();
  if (!collections.users) {
    throw new Error('The default "users" auth collection was not found in PocketBase.');
  }

  // Organizations must exist before users can reference it, so reconcile it
  // first, then refresh so its id is available for the relation field.
  if (collections.organizations) {
    await api("PATCH", `/api/collections/${collections.organizations.id}`, orgDefinition);
    console.log("Updated collection: organizations");
  } else {
    await api("POST", "/api/collections", orgDefinition);
    console.log("Created collection: organizations");
  }
  collections = await getCollections();

  await ensureUserFields(collections.users, collections.organizations.id);

  // Resolve collection ids needed for relation fields. devices must exist before
  // messages/inbox/webhooks reference it, so create in order, refreshing ids.
  const order = ["devices", "messages", "inbox", "calls", "webhooks"];
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
  console.log("Collections: users (existing), organizations, devices, messages, inbox, calls, webhooks.");
  console.log("\nNext: create a user to log in with, e.g. via the PocketBase admin UI,");
  console.log("or run scripts/create-user.mjs.");
}

main().catch((err) => {
  console.error("\nSetup failed:", err.message);
  process.exit(1);
});
