import { ref, computed } from "vue";

// The panel authenticates exactly like any other gsmnode client: it posts to
// /api/auth/login and keeps the PocketBase token the API Server relays back. The
// panel never talks to PocketBase directly — it doesn't even know its address.
const TOKEN_KEY = "gsmnode-panel-token";

function storedToken() {
  try {
    return localStorage.getItem(TOKEN_KEY) || "";
  } catch {
    return ""; // private mode
  }
}

export const token = ref(storedToken());
export const me = ref(null);

export const isSuperadmin = computed(() => me.value?.role === "superadmin");
export const isManager = computed(
  () => me.value?.role === "admin" || me.value?.role === "superadmin",
);

function setToken(value) {
  token.value = value;
  try {
    if (value) localStorage.setItem(TOKEN_KEY, value);
    else localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* private mode — the session just won't survive a reload */
  }
}

/** ApiError carries the HTTP status so callers can branch on 401/403/503. */
export class ApiError extends Error {
  constructor(message, status, body) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

// messageFrom digs a human-readable message out of the error shapes in play:
// this server's {error}, and PocketBase's {message, data:{field:{message}}}
// which the user endpoints relay verbatim on a validation failure.
function messageFrom(body, status) {
  if (!body || typeof body !== "object") return `HTTP ${status}`;
  if (body.error) return body.error;
  const fieldErrors = Object.entries(body.data || {})
    .map(([field, e]) => `${field}: ${e?.message || e}`)
    .filter(Boolean);
  if (fieldErrors.length) return fieldErrors.join("; ");
  return body.message || `HTTP ${status}`;
}

/**
 * request calls the API Server, attaching the panel's token. A 401 clears the
 * session so the shell falls back to the login screen.
 */
export async function request(path, { method = "GET", body, auth = true } = {}) {
  const headers = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (auth && token.value) headers.Authorization = token.value;

  const resp = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const text = await resp.text();
  let parsed = null;
  try {
    parsed = text ? JSON.parse(text) : null;
  } catch {
    parsed = null; // non-JSON body — don't explode on it
  }

  if (!resp.ok) {
    if (resp.status === 401 && auth) logout();
    throw new ApiError(messageFrom(parsed, resp.status), resp.status, parsed);
  }
  return parsed;
}

/** login exchanges credentials for a PocketBase token via the API Server. */
export async function login(email, password) {
  const out = await request("/api/auth/login", {
    method: "POST",
    body: { email, password },
    auth: false,
  });
  setToken(out.access_token || out.token);
  await loadMe();
  return me.value;
}

/** loadMe resolves the current identity, including role. */
export async function loadMe() {
  me.value = await request("/api/auth/me");
  return me.value;
}

/** logout drops the local session. PocketBase tokens are stateless, so there is
 *  nothing to revoke server-side. */
export function logout() {
  setToken("");
  me.value = null;
}

/** restore re-validates a token kept from a previous visit. */
export async function restore() {
  if (!token.value) return null;
  try {
    return await loadMe();
  } catch {
    logout(); // expired, revoked, or the server now points at another PocketBase
    return null;
  }
}
