import { ref } from "vue";

// Persisted light/dark theme, shared key with the design-system kits.
const KEY = "gsmnode-theme";

function initial() {
  try {
    return localStorage.getItem(KEY) === "dark" ? "dark" : "light";
  } catch {
    return "light";
  }
}

export const theme = ref(initial());

export function applyTheme(t) {
  theme.value = t;
  document.documentElement.setAttribute("data-gsm-theme", t);
  try {
    localStorage.setItem(KEY, t);
  } catch {
    /* private mode — theme just won't persist */
  }
}

export function toggleTheme() {
  applyTheme(theme.value === "dark" ? "light" : "dark");
}

applyTheme(theme.value);
