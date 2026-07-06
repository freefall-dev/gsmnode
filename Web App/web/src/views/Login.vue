<script setup>
import { ref } from "vue";
import { useRouter, useRoute } from "vue-router";
import { Settings2, ChevronDown, ChevronUp } from "@lucide/vue";
import { auth } from "../store/auth";
import { getApiBase, setApiBase } from "../api";
import { theme } from "../theme";

const router = useRouter();
const route = useRoute();

const email = ref("");
const password = ref("");
const error = ref("");
const loading = ref(false);

// Server settings — point the app at a specific API Server, like the phone app.
// Blank means "use this site's built-in server" (the Web App BFF proxy).
const serverUrl = ref(getApiBase());
const showServer = ref(!!getApiBase());

async function submit() {
  error.value = "";
  loading.value = true;
  try {
    setApiBase(serverUrl.value);
    await auth.login(email.value.trim(), password.value);
    router.push(route.query.redirect || "/");
  } catch (e) {
    if (e.status === 401) {
      error.value = "Invalid email or password.";
    } else if (e.status === undefined) {
      error.value = "Cannot reach the API Server. Check the server URL.";
    } else {
      error.value = e.message;
    }
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-page p-4">
    <div class="w-full max-w-sm rounded-xl border border-subtle bg-card p-8 shadow-lg">
      <div class="mb-7 text-center">
        <img
          :src="theme === 'dark' ? '/gsmnode-horizontal-white.png' : '/gsmnode-horizontal.png'"
          alt="gsmnode"
          class="mx-auto h-8"
        />
        <p class="gn-eyebrow mt-4">Sign in to your gateway</p>
      </div>

      <form class="space-y-4" @submit.prevent="submit">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Email</label>
          <input
            v-model="email"
            type="email"
            required
            autocomplete="username"
            class="gn-input"
            placeholder="you@example.com"
          />
        </div>
        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Password</label>
          <input
            v-model="password"
            type="password"
            required
            autocomplete="current-password"
            class="gn-input"
            placeholder="••••••••"
          />
        </div>

        <!-- Server settings (editable, like the phone app) -->
        <div class="rounded-md border border-subtle">
          <button
            type="button"
            class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm font-medium text-secondary transition-colors hover:bg-sunken"
            @click="showServer = !showServer"
          >
            <span class="flex items-center gap-2">
              <Settings2 class="h-4 w-4" />Server settings
            </span>
            <ChevronUp v-if="showServer" class="h-4 w-4 text-muted" />
            <ChevronDown v-else class="h-4 w-4 text-muted" />
          </button>
          <div v-show="showServer" class="border-t border-subtle p-3">
            <label class="mb-1.5 block text-sm font-medium text-primary">API Server URL</label>
            <input
              v-model="serverUrl"
              type="url"
              autocomplete="off"
              class="gn-input font-mono !text-xs"
              placeholder="http://10.2.1.101:8080"
            />
            <p class="mt-1.5 text-xs text-muted">
              Leave blank to use this site's built-in server.
            </p>
          </div>
        </div>

        <p v-if="error" class="rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>

        <button type="submit" :disabled="loading" class="gn-btn-pri w-full">
          {{ loading ? "Signing in…" : "Sign in" }}
        </button>
      </form>
    </div>
  </div>
</template>
