// End-to-end encryption for message content and recipient numbers.
//
// When a passphrase is set, the Web App encrypts text_message + phone numbers
// before they leave the browser, and decrypts inbox items on the way in. The API
// Server and PocketBase only ever see ciphertext; the passphrase never leaves
// this device (it lives in localStorage only).
//
// Deliberately *not* encrypted: the MMS subject, data-SMS payloads and MMS
// attachments. The phone leaves them alone in both directions too, so don't add
// encryption on one end without the other.
//
// Wire format of an encrypted value:  "gsmenc:v1:" + base64( salt || iv || ct )
//   salt = 16 random bytes   (PBKDF2)
//   iv   = 12 random bytes   (AES-GCM nonce)
//   ct   = AES-GCM ciphertext+tag over the UTF-8 plaintext
//
// The Phone Agent implements the identical scheme (see Phone Agent/lib/services/
// crypto_service.dart) so the two ends interoperate: PBKDF2-HMAC-SHA256,
// 150000 iterations, AES-256-GCM.

const PASS_KEY = "sms_gw_enc_pass";
const PREFIX = "gsmenc:v1:";
const ITERATIONS = 150000;

export function getPassphrase() {
  return localStorage.getItem(PASS_KEY) || "";
}

export function setPassphrase(pass) {
  if (pass) localStorage.setItem(PASS_KEY, pass);
  else localStorage.removeItem(PASS_KEY);
}

export function encryptionEnabled() {
  return getPassphrase().length > 0;
}

// True when a value looks like something this module produced.
export function isEncrypted(value) {
  return typeof value === "string" && value.startsWith(PREFIX);
}

function b64encode(bytes) {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}

function b64decode(str) {
  const bin = atob(str);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

async function deriveKey(pass, salt) {
  const enc = new TextEncoder();
  const baseKey = await crypto.subtle.importKey(
    "raw",
    enc.encode(pass),
    "PBKDF2",
    false,
    ["deriveKey"]
  );
  return crypto.subtle.deriveKey(
    { name: "PBKDF2", salt, iterations: ITERATIONS, hash: "SHA-256" },
    baseKey,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"]
  );
}

// Encrypts a plaintext string with the current passphrase. Returns the plaintext
// unchanged when no passphrase is set. Empty strings are passed through.
export async function encryptString(plain) {
  const pass = getPassphrase();
  if (!pass || !plain) return plain;
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const key = await deriveKey(pass, salt);
  const ct = new Uint8Array(
    await crypto.subtle.encrypt(
      { name: "AES-GCM", iv },
      key,
      new TextEncoder().encode(plain)
    )
  );
  const packed = new Uint8Array(salt.length + iv.length + ct.length);
  packed.set(salt, 0);
  packed.set(iv, salt.length);
  packed.set(ct, salt.length + iv.length);
  return PREFIX + b64encode(packed);
}

// Decrypts a value produced by encryptString. Non-encrypted values are returned
// as-is. Throws if the passphrase is wrong or missing for an encrypted value.
export async function decryptString(value) {
  if (!isEncrypted(value)) return value;
  const pass = getPassphrase();
  if (!pass) throw new Error("encrypted; set the passphrase in Settings");
  const packed = b64decode(value.slice(PREFIX.length));
  const salt = packed.slice(0, 16);
  const iv = packed.slice(16, 28);
  const ct = packed.slice(28);
  const key = await deriveKey(pass, salt);
  const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ct);
  return new TextDecoder().decode(plain);
}

// Encrypts each item of an array of strings (e.g. recipient phone numbers).
export async function encryptList(list) {
  return Promise.all((list || []).map((v) => encryptString(v)));
}

// Best-effort decrypt: returns the decrypted text, or a marker when the
// passphrase can't open it, so a viewer never shows a raw ciphertext blob.
export async function tryDecrypt(value) {
  if (!isEncrypted(value)) return value;
  try {
    return await decryptString(value);
  } catch {
    return "🔒 encrypted (wrong or missing passphrase)";
  }
}
