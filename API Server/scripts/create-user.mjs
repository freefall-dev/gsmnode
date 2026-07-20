// Creates a user in the PocketBase "users" collection to log in with.
//
// Usage (PowerShell):
//   $env:POCKETBASE_URL="http://10.2.1.10:8028"
//   $env:PB_ADMIN_EMAIL="admin@example.com"
//   $env:PB_ADMIN_PASSWORD="admin-password"
//   node scripts/create-user.mjs user@example.com "user-password" "Display Name"

const BASE = (process.env.POCKETBASE_URL || "http://10.2.1.10:8028").replace(/\/$/, "");
const EMAIL = process.env.PB_ADMIN_EMAIL;
const PASSWORD = process.env.PB_ADMIN_PASSWORD;

const [, , userEmail, userPassword, userName, userRole] = process.argv;

const ROLES = ["user", "admin", "superadmin"];

if (!EMAIL || !PASSWORD) {
  console.error("Set PB_ADMIN_EMAIL and PB_ADMIN_PASSWORD environment variables.");
  process.exit(1);
}
if (!userEmail || !userPassword) {
  console.error('Usage: node scripts/create-user.mjs <email> <password> ["name"] [role]');
  console.error(`  role is one of: ${ROLES.join(", ")} (default: user)`);
  process.exit(1);
}
const role = userRole || "user";
if (!ROLES.includes(role)) {
  console.error(`Invalid role "${role}". Must be one of: ${ROLES.join(", ")}`);
  process.exit(1);
}

async function api(method, path, body, token) {
  const res = await fetch(BASE + path, {
    method,
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: token } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  const json = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(`${method} ${path} -> ${res.status}: ${json?.message || text}`);
  return json;
}

const auth = await api("POST", "/api/collections/_superusers/auth-with-password", {
  identity: EMAIL,
  password: PASSWORD,
});

const user = await api("POST", "/api/collections/users/records", {
  email: userEmail,
  password: userPassword,
  passwordConfirm: userPassword,
  name: userName || "",
  role,
  emailVisibility: true,
  verified: true,
}, auth.token);

console.log(`Created user ${user.email} (role: ${role}, id: ${user.id}).`);
