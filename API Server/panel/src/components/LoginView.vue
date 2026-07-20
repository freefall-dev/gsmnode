<script setup>
import { ref, computed } from "vue";
import { login } from "../api";

const email = ref("");
const password = ref("");
const error = ref("");
const busy = ref(false);

const canSubmit = computed(
  () => email.value.trim() !== "" && password.value !== "" && !busy.value,
);

async function submit() {
  if (!canSubmit.value) return;
  error.value = "";
  busy.value = true;
  try {
    await login(email.value.trim(), password.value);
  } catch (e) {
    // 400/401/404 all mean "bad credentials" — don't leak which.
    error.value =
      e.status === 400 || e.status === 401 || e.status === 404
        ? "Invalid email or password."
        : e.message || "Sign-in failed.";
    password.value = "";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="mx-auto flex w-full max-w-sm flex-col gap-5 pt-20">
    <div class="rounded-lg border border-subtle bg-card p-6 shadow-sm">
      <h1 class="text-lg font-semibold text-primary">Sign in</h1>
      <p class="mt-1 mb-5 text-sm text-secondary">
        Use your gsmnode account to reach the console.
      </p>

      <form class="flex flex-col gap-4" @submit.prevent="submit">
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary" for="login-email">Email</label>
          <input
            id="login-email"
            v-model="email"
            class="gn-input"
            type="email"
            autocomplete="username"
            autofocus
            placeholder="you@example.com"
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary" for="login-password">Password</label>
          <input
            id="login-password"
            v-model="password"
            class="gn-input"
            type="password"
            autocomplete="current-password"
            placeholder="••••••••"
          />
        </div>

        <p v-if="error" class="rounded-sm bg-danger-tint px-3 py-2 text-xs text-danger">
          {{ error }}
        </p>

        <button class="gn-btn-pri w-full" type="submit" :disabled="!canSubmit">
          {{ busy ? "Signing in…" : "Sign in" }}
        </button>
      </form>
    </div>

    <p class="text-center font-mono text-[11px] text-muted">
      Authentication is proxied to PocketBase by the API Server.
    </p>
  </div>
</template>
