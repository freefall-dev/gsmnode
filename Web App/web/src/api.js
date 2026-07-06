// Thin fetch wrapper around the Web App BFF (same-origin /api, proxied to the
// API Server). Handles bearer-token auth and JSON encoding/decoding.

const TOKEN_KEY = "sms_gw_token";
const API_BASE_KEY = "sms_gw_api_base";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || "";
}

export function setToken(token) {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

// Optional API Server base URL. When set, the browser calls that server
// directly (the server sends permissive CORS). When blank, requests go to this
// site's own origin at /api, i.e. through the Web App BFF proxy.
export function getApiBase() {
  return localStorage.getItem(API_BASE_KEY) || "";
}

export function setApiBase(url) {
  const v = (url || "").trim().replace(/\/+$/, "");
  if (v) localStorage.setItem(API_BASE_KEY, v);
  else localStorage.removeItem(API_BASE_KEY);
}

export class ApiError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

async function request(method, path, body) {
  const headers = {};
  const token = getToken();
  if (token) headers.Authorization = "Bearer " + token;
  if (body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(getApiBase() + "/api" + path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  const text = await res.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }

  if (!res.ok) {
    const msg = (data && data.error) || res.statusText || "Request failed";
    throw new ApiError(res.status, msg);
  }
  return data;
}

export const api = {
  get: (p) => request("GET", p),
  post: (p, b) => request("POST", p, b),
  patch: (p, b) => request("PATCH", p, b),
  del: (p) => request("DELETE", p),
};
