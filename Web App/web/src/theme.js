import { ref } from "vue";

// Persisted theme preference, shared key with the design-system kits. The stored
// value is one of "light" | "dark" | "system"; "system" follows the OS setting
// and tracks it live. `theme` is always the resolved/effective value ("light" or
// "dark") that the rest of the app and the CSS react to.
const KEY = "gsmnode-theme";
const mq = typeof window !== "undefined" && window.matchMedia
  ? window.matchMedia("(prefers-color-scheme: dark)")
  : null;

function storedPref() {
  try {
    const v = localStorage.getItem(KEY);
    return v === "light" || v === "dark" || v === "system" ? v : "light";
  } catch {
    return "light";
  }
}

function resolve(pref) {
  if (pref === "system") return mq && mq.matches ? "dark" : "light";
  return pref;
}

// The user's choice ("light" | "dark" | "system") — drives the Settings picker.
export const themePref = ref(storedPref());
// The effective theme actually applied ("light" | "dark") — drives the logo,
// the header toggle icon, and everything reading `theme`.
export const theme = ref(resolve(themePref.value));

function apply() {
  theme.value = resolve(themePref.value);
  document.documentElement.setAttribute("data-gsm-theme", theme.value);
}

export function setThemePref(pref) {
  themePref.value = pref === "light" || pref === "dark" || pref === "system" ? pref : "light";
  try {
    localStorage.setItem(KEY, themePref.value);
  } catch {
    /* private mode — preference just won't persist */
  }
  apply();
}

// Header quick-toggle: flip the effective theme and pin it as an explicit choice
// (so a toggle away from "system" sticks rather than snapping back).
export function toggleTheme() {
  setThemePref(theme.value === "dark" ? "light" : "dark");
}

// Follow the OS while the preference is "system".
mq?.addEventListener?.("change", () => {
  if (themePref.value === "system") apply();
});

apply();
