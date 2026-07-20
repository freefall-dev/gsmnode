<script setup>
import { ref, computed, onMounted } from "vue";
import { Moon, Sun } from "@lucide/vue";
import { theme, toggleTheme } from "./theme";
import { me, token, restore, logout, isManager, isSuperadmin } from "./api";
import LoginView from "./components/LoginView.vue";
import StatusCard from "./components/StatusCard.vue";
import UsersCard from "./components/UsersCard.vue";
import PocketBaseCard from "./components/PocketBaseCard.vue";
import EndpointTable from "./components/EndpointTable.vue";

const booting = ref(true);
const section = ref("overview");

onMounted(async () => {
  await restore(); // re-validate a token kept from a previous visit
  booting.value = false;
});

// Sections are gated by role: user management needs a manager, PocketBase needs
// a superadmin. The server enforces the same rules — this only hides what the
// caller could not use anyway.
const sections = computed(() => {
  const out = [{ id: "overview", label: "Overview" }];
  if (isManager.value) out.push({ id: "users", label: "Users" });
  if (isSuperadmin.value) out.push({ id: "pocketbase", label: "PocketBase" });
  out.push({ id: "api", label: "API" });
  return out;
});

function signOut() {
  logout();
  section.value = "overview";
}

const roleClass = computed(() =>
  me.value?.role === "superadmin"
    ? "bg-info-tint text-info"
    : me.value?.role === "admin"
      ? "bg-warning-tint text-warning"
      : "bg-sunken text-muted",
);

const authApi = [
  { method: "POST", path: "/api/auth/login", desc: "Exchange email + password for a token" },
  { method: "POST", path: "/api/auth/refresh", desc: "Exchange a valid token for a fresh one" },
  { method: "GET", path: "/api/auth/validate", desc: "Check whether a token is still valid" },
  { method: "GET", path: "/api/auth/me", desc: "Identity of the bearer token (incl. role)" },
];

const clientApi = [
  { method: "GET", path: "/api/devices", desc: "List registered gateway devices" },
  { method: "DELETE", path: "/api/devices/{id}", desc: "Remove a device" },
  { method: "POST", path: "/api/messages", desc: "Queue an outbound SMS" },
  { method: "GET", path: "/api/messages", desc: "Outbound message history" },
  { method: "POST", path: "/api/calls", desc: "Queue an outbound phone call" },
  { method: "GET", path: "/api/inbox", desc: "Messages received by your devices" },
  { method: "GET", path: "/api/webhooks", desc: "List webhook subscriptions" },
  { method: "POST", path: "/api/webhooks", desc: "Register a webhook" },
];

const managementApi = [
  { method: "GET", path: "/api/users", desc: "List users" },
  { method: "POST", path: "/api/users", desc: "Create a user" },
  { method: "PATCH", path: "/api/users/{id}", desc: "Update email / name / role / password" },
  { method: "DELETE", path: "/api/users/{id}", desc: "Delete a user" },
];

const superadminApi = [
  { method: "GET", path: "/api/admin/pb-config", desc: "PocketBase connection + live probe" },
  { method: "POST", path: "/api/admin/pb-config/test", desc: "Probe a candidate connection" },
  { method: "PUT", path: "/api/admin/pb-config", desc: "Apply + persist a connection" },
];

const mobileApi = [
  { method: "POST", path: "/api/mobile/v1/device", desc: "Register this phone as a gateway" },
  { method: "GET", path: "/api/mobile/v1/messages", desc: "Pull pending messages to send" },
  { method: "PATCH", path: "/api/mobile/v1/messages/{id}", desc: "Report sent / delivered / failed" },
  { method: "POST", path: "/api/mobile/v1/inbox", desc: "Push a received SMS" },
  { method: "POST", path: "/api/mobile/v1/ping", desc: "Device heartbeat" },
];
</script>

<template>
  <div class="mx-auto flex max-w-3xl flex-col gap-6 px-6 pt-12 pb-16">
    <!-- Header -->
    <div class="flex items-center gap-4">
      <img
        :src="theme === 'dark' ? '/gsmnode-horizontal-white.png' : '/gsmnode-horizontal.png'"
        alt="gsmnode"
        class="h-8"
      />
      <span class="gn-eyebrow mt-1.5">API server</span>
      <div class="flex-1"></div>

      <span v-if="me" class="hidden font-mono text-xs text-muted sm:inline">{{ me.email }}</span>
      <span
        v-if="me"
        class="inline-flex rounded-sm px-2 py-0.5 font-mono text-xs font-medium"
        :class="roleClass"
      >{{ me.role }}</span>

      <button
        class="gn-btn-sec gn-btn-sm"
        :title="theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'"
        @click="toggleTheme"
      >
        <Sun v-if="theme === 'dark'" class="h-4 w-4" />
        <Moon v-else class="h-4 w-4" />
        Theme
      </button>
      <button v-if="token" class="gn-btn-sec gn-btn-sm" @click="signOut">Sign out</button>
    </div>

    <p v-if="booting" class="py-16 text-center font-mono text-xs text-muted">Loading…</p>

    <!-- Unauthenticated: the login gate is the whole console. -->
    <LoginView v-else-if="!token" />

    <template v-else>
      <!-- Section nav -->
      <nav class="flex flex-wrap gap-1.5">
        <button
          v-for="s in sections"
          :key="s.id"
          class="gn-btn-sec gn-btn-sm"
          :class="section === s.id ? '!border-brand !text-brand-text' : ''"
          @click="section = s.id"
        >
          {{ s.label }}
        </button>
      </nav>

      <StatusCard v-if="section === 'overview'" />
      <UsersCard v-else-if="section === 'users'" />
      <PocketBaseCard v-else-if="section === 'pocketbase'" />

      <template v-else-if="section === 'api'">
        <EndpointTable title="Auth" auth="Public / Bearer" :endpoints="authApi" />
        <EndpointTable title="Client API" auth="Bearer token" :endpoints="clientApi" />
        <EndpointTable title="User management" auth="Manager" :endpoints="managementApi" />
        <EndpointTable title="Superadmin" auth="Superadmin" :endpoints="superadminApi" />
        <EndpointTable title="Mobile API" auth="Device token" :endpoints="mobileApi" />
      </template>
    </template>

    <p class="text-center font-mono text-[11px] text-muted">
      gsmnode — turn any Android phone into an SMS gateway.
    </p>
  </div>
</template>
