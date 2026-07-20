<script setup>
import { ref, onMounted } from "vue";
import { RefreshCw, PhoneOutgoing, Binary, Image } from "@lucide/vue";
import { api } from "../api";
import { tryDecrypt } from "../crypto";
import PageHeader from "../components/PageHeader.vue";
import StatusBadge from "../components/StatusBadge.vue";

const messages = ref([]);
const loading = ref(true);
const error = ref("");
const statusFilter = ref("");

const statuses = ["", "Pending", "Processed", "Sent", "Delivered", "Failed"];

const typeMeta = {
  data: { label: "Data SMS", icon: Binary },
  mms: { label: "MMS", icon: Image },
  call: { label: "Voice call", icon: PhoneOutgoing },
};

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const q = statusFilter.value ? "?status=" + encodeURIComponent(statusFilter.value) : "";
    const res = await api.get("/messages" + q);
    const raw = res.items || [];
    messages.value = await Promise.all(
      raw.map(async (m) => ({
        ...m,
        phone_numbers: m.encrypted
          ? await Promise.all((m.phone_numbers || []).map((p) => tryDecrypt(p)))
          : m.phone_numbers,
        text_message: m.encrypted ? await tryDecrypt(m.text_message) : m.text_message,
      }))
    );
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

function fmt(ts) {
  return ts ? new Date(ts).toLocaleString() : "—";
}

onMounted(load);
</script>

<template>
  <div>
    <PageHeader title="Messages" subtitle="Outbound message history">
      <template #actions>
        <div class="flex items-center gap-2">
          <select v-model="statusFilter" class="gn-input !h-8 !w-auto !text-xs" @change="load">
            <option v-for="s in statuses" :key="s" :value="s">{{ s || "All statuses" }}</option>
          </select>
          <button class="gn-btn-sec gn-btn-sm" @click="load">
            <RefreshCw class="h-3.5 w-3.5" />Refresh
          </button>
        </div>
      </template>
    </PageHeader>

    <p v-if="error" class="mb-4 rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>

    <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-xs">
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="gn-eyebrow">
            <th class="px-5 py-3 font-medium">To</th>
            <th class="px-5 py-3 font-medium">Message</th>
            <th class="px-5 py-3 font-medium">Status</th>
            <th class="px-5 py-3 font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="4" class="border-t border-subtle px-5 py-10 text-center text-sm text-muted">Loading…</td>
          </tr>
          <tr v-else-if="!messages.length">
            <td colspan="4" class="border-t border-subtle px-5 py-10 text-center text-sm text-muted">
              No messages yet. Send your first with a POST to <span class="rounded-xs bg-sunken px-1.5 py-0.5 font-mono text-xs text-secondary">/api/messages</span>.
            </td>
          </tr>
          <tr v-for="m in messages" :key="m.id" class="align-top transition-colors hover:bg-sunken">
            <td class="border-t border-subtle px-5 py-3 font-mono text-xs text-primary">{{ (m.phone_numbers || []).join(", ") }}</td>
            <td class="border-t border-subtle px-5 py-3 text-secondary">
              <div v-if="typeMeta[m.type]" class="flex items-center gap-1.5">
                <component :is="typeMeta[m.type].icon" class="h-3.5 w-3.5" />
                <span class="gn-eyebrow !text-secondary">{{ typeMeta[m.type].label }}</span>
                <span
                  v-if="m.type === 'data' && m.data_port != null"
                  class="font-mono text-[10px] text-muted"
                >port {{ m.data_port }}</span>
                <span v-if="m.text_message" class="max-w-xs truncate text-secondary">· {{ m.text_message }}</span>
              </div>
              <div v-else class="max-w-md truncate">{{ m.text_message }}</div>
              <div v-if="m.encrypted" class="mt-0.5 font-mono text-[10px] text-muted">🔒 e2e</div>
              <div v-if="m.error" class="mt-0.5 text-xs text-danger">{{ m.error }}</div>
            </td>
            <td class="border-t border-subtle px-5 py-3"><StatusBadge :status="m.status" /></td>
            <td class="border-t border-subtle px-5 py-3 font-mono text-xs text-secondary">{{ fmt(m.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
