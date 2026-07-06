<script setup>
import { ref, onMounted, onUnmounted } from "vue";
import { Moon, Sun } from "@lucide/vue";
import { theme, toggleTheme } from "./theme";
import EndpointTable from "./components/EndpointTable.vue";

// Live health poll against this server.
const status = ref("checking"); // checking | ok | error | unreachable
const httpStatus = ref(null);
const latency = ref(null);
const checkedAt = ref(null);
let timer = null;

async function check() {
  const started = performance.now();
  try {
    const r = await fetch("/api/health");
    latency.value = Math.round(performance.now() - started);
    httpStatus.value = r.status;
    status.value = r.ok ? "ok" : "error";
  } catch {
    latency.value = null;
    httpStatus.value = null;
    status.value = "unreachable";
  }
  checkedAt.value = new Date();
}

onMounted(() => {
  check();
  timer = setInterval(check, 10000);
});
onUnmounted(() => clearInterval(timer));

const badge = {
  checking: { label: "checking", cls: "bg-sunken text-secondary" },
  ok: { label: "operational", cls: "bg-success-tint text-success" },
  error: { label: "error", cls: "bg-danger-tint text-danger" },
  unreachable: { label: "unreachable", cls: "bg-danger-tint text-danger" },
};

const clientApi = [
  { method: "POST", path: "/api/auth/login", desc: "Exchange email + password for a JWT" },
  { method: "GET", path: "/api/devices", desc: "List registered gateway devices" },
  { method: "DELETE", path: "/api/devices/{id}", desc: "Remove a device" },
  { method: "POST", path: "/api/messages", desc: "Queue an outbound SMS" },
  { method: "GET", path: "/api/messages", desc: "Outbound message history" },
  { method: "POST", path: "/api/calls", desc: "Queue an outbound phone call" },
  { method: "GET", path: "/api/inbox", desc: "Messages received by your devices" },
  { method: "GET", path: "/api/webhooks", desc: "List webhook subscriptions" },
  { method: "POST", path: "/api/webhooks", desc: "Register a webhook" },
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
      <button
        class="gn-btn-sec gn-btn-sm"
        :title="theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'"
        @click="toggleTheme"
      >
        <Sun v-if="theme === 'dark'" class="h-4 w-4" />
        <Moon v-else class="h-4 w-4" />
        Theme
      </button>
    </div>

    <!-- Status -->
    <div class="rounded-lg border border-subtle bg-card shadow-sm">
      <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
        <div class="text-base font-semibold text-primary">Status</div>
        <span
          class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1 font-mono text-xs font-medium"
          :class="badge[status].cls"
        >
          <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
          {{ badge[status].label }}<template v-if="status === 'error'"> {{ httpStatus }}</template>
        </span>
      </div>
      <div class="flex flex-wrap items-center gap-x-6 gap-y-2 px-5 py-4 font-mono text-xs text-secondary">
        <span>GET /api/health</span>
        <span>{{ latency !== null ? latency + "ms" : "—" }}</span>
        <span>{{ checkedAt ? "checked " + checkedAt.toLocaleTimeString() : "—" }}</span>
      </div>
    </div>

    <EndpointTable title="Client API" auth="Bearer JWT" :endpoints="clientApi" />
    <EndpointTable title="Mobile API" auth="Device token" :endpoints="mobileApi" />

    <p class="text-center font-mono text-[11px] text-muted">
      gsmnode — turn any Android phone into an SMS gateway.
    </p>
  </div>
</template>
