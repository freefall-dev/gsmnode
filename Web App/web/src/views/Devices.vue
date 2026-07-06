<script setup>
import { ref, onMounted } from "vue";
import { RefreshCw } from "@lucide/vue";
import { api } from "../api";
import PageHeader from "../components/PageHeader.vue";
import StatusBadge from "../components/StatusBadge.vue";

const devices = ref([]);
const loading = ref(true);
const error = ref("");

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await api.get("/devices");
    devices.value = res.items || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

async function remove(d) {
  if (!confirm(`Remove device "${d.name || d.device_id}"?`)) return;
  try {
    await api.del("/devices/" + d.id);
    devices.value = devices.value.filter((x) => x.id !== d.id);
  } catch (e) {
    alert("Could not remove device: " + e.message);
  }
}

function fmt(ts) {
  if (!ts) return "—";
  return new Date(ts).toLocaleString();
}

onMounted(load);
</script>

<template>
  <div>
    <PageHeader title="Devices" subtitle="Phones connected to your gateway">
      <template #actions>
        <button class="gn-btn-sec gn-btn-sm" @click="load">
          <RefreshCw class="h-3.5 w-3.5" />Refresh
        </button>
      </template>
    </PageHeader>

    <p v-if="error" class="mb-4 rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>

    <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-xs">
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="gn-eyebrow">
            <th class="px-5 py-3 font-medium">Name</th>
            <th class="px-5 py-3 font-medium">Device ID</th>
            <th class="px-5 py-3 font-medium">Platform</th>
            <th class="px-5 py-3 font-medium">Status</th>
            <th class="px-5 py-3 font-medium">Last seen</th>
            <th class="px-5 py-3"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="6" class="border-t border-subtle px-5 py-10 text-center text-sm text-muted">Loading…</td>
          </tr>
          <tr v-else-if="!devices.length">
            <td colspan="6" class="border-t border-subtle px-5 py-10 text-center text-sm text-muted">
              No devices yet. Register one from the phone app.
            </td>
          </tr>
          <tr v-for="d in devices" :key="d.id" class="transition-colors hover:bg-sunken">
            <td class="border-t border-subtle px-5 py-3 font-medium text-primary">{{ d.name || "—" }}</td>
            <td class="border-t border-subtle px-5 py-3 font-mono text-xs text-secondary">{{ d.device_id }}</td>
            <td class="border-t border-subtle px-5 py-3 text-secondary">{{ d.platform }} {{ d.app_version }}</td>
            <td class="border-t border-subtle px-5 py-3"><StatusBadge :status="d.status" /></td>
            <td class="border-t border-subtle px-5 py-3 font-mono text-xs text-secondary">{{ fmt(d.last_seen_at) }}</td>
            <td class="border-t border-subtle px-5 py-3 text-right">
              <button class="text-sm font-medium text-danger hover:underline" @click="remove(d)">Remove</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
