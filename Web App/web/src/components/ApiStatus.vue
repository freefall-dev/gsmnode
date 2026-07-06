<script setup>
import { ref, onMounted, onUnmounted } from "vue";
import { api } from "../api";

// Polls the API Server's public /health endpoint (through the configured base
// URL / BFF) and reflects reachability as a colored dot.
const status = ref("checking"); // checking | online | offline
const latency = ref(null);
let timer = null;

async function check() {
  const started = performance.now();
  try {
    await api.get("/health");
    latency.value = Math.round(performance.now() - started);
    status.value = "online";
  } catch {
    latency.value = null;
    status.value = "offline";
  }
}

onMounted(() => {
  check();
  timer = setInterval(check, 10000);
});
onUnmounted(() => clearInterval(timer));

const dotStyle = {
  checking: "background: var(--gray-300)",
  online: "background: var(--success)",
  offline: "background: var(--danger)",
};
const label = {
  checking: "Checking…",
  online: "API online",
  offline: "API offline",
};
</script>

<template>
  <div
    class="flex items-center gap-2"
    :title="
      status === 'online'
        ? `API Server reachable · ${latency} ms`
        : status === 'offline'
          ? 'API Server unreachable'
          : 'Checking API Server'
    "
  >
    <span class="relative flex h-2 w-2">
      <span
        v-if="status === 'online'"
        class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-60"
        style="background: var(--success)"
      ></span>
      <span class="relative inline-flex h-2 w-2 rounded-full" :style="dotStyle[status]"></span>
    </span>
    <span
      class="font-mono text-xs"
      :class="status === 'offline' ? 'text-danger' : 'text-secondary'"
    >
      {{ label[status] }}<template v-if="latency !== null"> · {{ latency }}ms</template>
    </span>
  </div>
</template>
