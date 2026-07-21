<script setup>
import { ref, onMounted, onUnmounted, computed } from "vue";
import { request } from "../api";

// A live watch list of registered gateway phones. ?scope=all widens the list as
// far as the caller's role allows — a superadmin sees every device, an admin
// their organization's, a plain user their own. The server caps the widening,
// so this is a display concern only.
const devices = ref(null);
const error = ref("");
let timer = null;

async function load() {
  try {
    const out = await request("/api/devices?scope=all");
    devices.value = out.items || [];
    error.value = "";
  } catch (e) {
    devices.value = null;
    error.value = e.message || "unreachable";
  }
}

// Online first, then most recently seen — the phones that matter sit on top.
const rows = computed(() =>
  [...(devices.value || [])].sort((a, b) => {
    if (a.status !== b.status) return a.status === "online" ? -1 : 1;
    return (b.last_seen_at || "").localeCompare(a.last_seen_at || "");
  }),
);

const onlineCount = computed(() => rows.value.filter((d) => d.status === "online").length);

// The server sends PocketBase datetimes ("2006-01-02 15:04:05.000Z"); Safari
// won't parse those without the T, hence the swap.
function seenAgo(value) {
  if (!value) return "never";
  const t = Date.parse(value.replace(" ", "T"));
  if (Number.isNaN(t)) return value;
  const secs = Math.max(0, Math.round((Date.now() - t) / 1000));
  if (secs < 60) return secs + "s ago";
  if (secs < 3600) return Math.round(secs / 60) + "m ago";
  if (secs < 86400) return Math.round(secs / 3600) + "h ago";
  return Math.round(secs / 86400) + "d ago";
}

// Carriers are the useful half of a SIM here; slots without one fall back to
// the slot number so a dual-SIM phone still reads as two entries.
function sims(device) {
  return (device.sims || [])
    .map((s) => s.carrier || s.display_name || "SIM " + s.slot)
    .join(" · ");
}

onMounted(() => {
  load();
  timer = setInterval(load, 10000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div class="text-base font-semibold text-primary">Connected devices</div>
      <span
        v-if="error"
        class="inline-flex items-center gap-1.5 rounded-sm bg-danger-tint px-2.5 py-1 font-mono text-xs font-medium text-danger"
      >
        <span class="h-1.5 w-1.5 rounded-full bg-current"></span>unreachable
      </span>
      <span v-else-if="devices" class="font-mono text-xs text-muted">
        {{ onlineCount }} / {{ rows.length }} online
      </span>
    </div>

    <div v-if="error" class="px-5 py-4 text-sm text-danger">{{ error }}</div>

    <div v-else-if="devices === null" class="px-5 py-4 text-sm text-muted">checking…</div>

    <div v-else-if="!rows.length" class="px-5 py-4 text-sm text-muted">
      No devices registered yet. Pair a phone with the gsmnode agent to see it here.
    </div>

    <table v-else class="w-full text-left text-sm">
      <tbody>
        <tr v-for="d in rows" :key="d.id" class="border-t border-subtle first:border-t-0">
          <td class="px-5 py-3">
            <div class="font-medium text-primary">{{ d.name || d.device_id }}</div>
            <div v-if="d.owner_email" class="font-mono text-xs text-muted">{{ d.owner_email }}</div>
          </td>
          <td class="px-5 py-3 font-mono text-xs text-muted">
            <div>{{ d.platform || "—" }}{{ d.app_version ? " · " + d.app_version : "" }}</div>
            <div v-if="sims(d)">{{ sims(d) }}</div>
          </td>
          <td class="px-5 py-3 text-right font-mono text-xs text-muted">{{ seenAgo(d.last_seen_at) }}</td>
          <td class="px-5 py-3 text-right">
            <span
              class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1 font-mono text-xs font-medium"
              :class="d.status === 'online' ? 'bg-success-tint text-success' : 'bg-sunken text-muted'"
            >
              <span class="h-1.5 w-1.5 rounded-full bg-current"></span>{{ d.status }}
            </span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
