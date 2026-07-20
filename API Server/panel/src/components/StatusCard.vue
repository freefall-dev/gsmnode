<script setup>
import { ref, onMounted, onUnmounted, computed } from "vue";
import { request } from "../api";

// /api/status probes PocketBase and the Web App server-side, so the browser
// never has to reach either directly.
const status = ref(null);
const error = ref("");
let timer = null;

async function check() {
  try {
    status.value = await request("/api/status", { auth: false });
    error.value = "";
  } catch (e) {
    status.value = null;
    error.value = e.message || "unreachable";
  }
}

const rows = computed(() => [
  { key: "apiServer", label: "API Server", h: status.value?.apiServer },
  { key: "pocketBase", label: "PocketBase", h: status.value?.pocketBase },
  { key: "webApp", label: "Web App", h: status.value?.webApp },
]);

onMounted(() => {
  check();
  timer = setInterval(check, 10000);
});
onUnmounted(() => clearInterval(timer));

const pillFor = (s) =>
  s === "ok" ? "bg-success-tint text-success" : "bg-danger-tint text-danger";
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div class="text-base font-semibold text-primary">Status</div>
      <span v-if="error" class="inline-flex items-center gap-1.5 rounded-sm bg-danger-tint px-2.5 py-1 font-mono text-xs font-medium text-danger">
        <span class="h-1.5 w-1.5 rounded-full bg-current"></span>unreachable
      </span>
    </div>

    <div v-if="error" class="px-5 py-4 text-sm text-danger">{{ error }}</div>

    <table v-else-if="status" class="w-full text-left text-sm">
      <tbody>
        <tr v-for="row in rows" :key="row.key" class="border-t border-subtle first:border-t-0">
          <td class="px-5 py-3 font-medium text-primary">{{ row.label }}</td>
          <td class="px-5 py-3 font-mono text-xs text-muted">{{ row.h?.url || "this process" }}</td>
          <td class="px-5 py-3 text-right font-mono text-xs text-muted">
            {{ row.h?.latencyMs != null ? row.h.latencyMs + "ms" : "—" }}
          </td>
          <td class="px-5 py-3 text-right">
            <span
              class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1 font-mono text-xs font-medium"
              :class="pillFor(row.h?.status)"
            >
              <span class="h-1.5 w-1.5 rounded-full bg-current"></span>{{ row.h?.status || "—" }}
            </span>
          </td>
        </tr>
      </tbody>
    </table>

    <div v-else class="px-5 py-4 text-sm text-muted">checking…</div>
  </div>
</template>
