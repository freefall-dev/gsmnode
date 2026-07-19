<script setup>
import { ref, onMounted } from "vue";
import { RefreshCw, ArrowDownLeft } from "@lucide/vue";
import { api } from "../api";
import PageHeader from "../components/PageHeader.vue";

const items = ref([]);
const loading = ref(true);
const error = ref("");

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await api.get("/inbox");
    items.value = res.items || [];
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
    <PageHeader title="Inbox" subtitle="Incoming messages received by your devices">
      <template #actions>
        <button class="gn-btn-sec gn-btn-sm" @click="load">
          <RefreshCw class="h-3.5 w-3.5" />Refresh
        </button>
      </template>
    </PageHeader>

    <p v-if="error" class="mb-4 rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>

    <div v-if="loading" class="text-sm text-muted">Loading…</div>
    <div
      v-else-if="!items.length"
      class="rounded-lg border border-dashed border-strong p-10 text-center text-sm text-muted"
    >
      No incoming messages yet.
    </div>
    <div v-else class="space-y-3">
      <div
        v-for="m in items"
        :key="m.id"
        class="flex items-start gap-3 rounded-lg border border-subtle bg-card p-4 shadow-xs"
      >
        <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-sm bg-success-tint text-success">
          <ArrowDownLeft class="h-4 w-4" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="flex items-baseline justify-between gap-3">
            <div class="flex items-baseline gap-2">
              <span class="font-mono text-sm font-medium text-primary">{{ m.phone_number }}</span>
              <span
                v-if="m.sim_slot != null"
                class="rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-muted"
              >SIM {{ m.sim_slot }}</span>
            </div>
            <span class="shrink-0 font-mono text-[11px] text-muted">{{ fmt(m.received_at) }}</span>
          </div>
          <p class="mt-1 text-sm text-secondary">{{ m.message }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
