<script setup>
import { ref, computed, onMounted } from "vue";
import { Phone, PhoneIncoming, PhoneOutgoing, PhoneMissed, RefreshCw } from "@lucide/vue";
import { api } from "../api";
import PageHeader from "../components/PageHeader.vue";

const devices = ref([]);
const phone = ref("");
const deviceId = ref("");
const calling = ref(false);
const result = ref(null);
const error = ref("");

// Call log (incoming + outgoing), reported by devices.
const calls = ref([]);
const logLoading = ref(true);
const logError = ref("");
const filter = ref("all"); // all | incoming | outgoing

const visibleCalls = computed(() =>
  filter.value === "all"
    ? calls.value
    : calls.value.filter((c) => c.direction === filter.value)
);

onMounted(async () => {
  try {
    const res = await api.get("/devices");
    devices.value = res.items || [];
  } catch {
    /* listing failure is non-fatal for the form */
  }
  loadLog();
});

async function loadLog() {
  logLoading.value = true;
  logError.value = "";
  try {
    const res = await api.get("/calls");
    calls.value = res.items || [];
  } catch (e) {
    logError.value = e.message;
  } finally {
    logLoading.value = false;
  }
}

async function makeCall() {
  error.value = "";
  result.value = null;
  const number = phone.value.trim();
  if (!number) {
    error.value = "Enter a phone number to call.";
    return;
  }
  calling.value = true;
  try {
    const body = { phone_number: number };
    if (deviceId.value) body.device_id = deviceId.value;
    result.value = await api.post("/calls", body);
    phone.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    calling.value = false;
  }
}

function fmt(ts) {
  return ts ? new Date(ts).toLocaleString() : "—";
}

function callIcon(c) {
  if (c.status === "missed" || c.status === "rejected" || c.status === "failed") return PhoneMissed;
  return c.direction === "outgoing" ? PhoneOutgoing : PhoneIncoming;
}

function callTone(c) {
  if (c.status === "missed" || c.status === "rejected" || c.status === "failed")
    return "bg-danger-tint text-danger";
  return c.direction === "outgoing" ? "bg-brand-tint text-brand-active" : "bg-success-tint text-success";
}

function duration(secs) {
  if (secs == null) return "";
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}
</script>

<template>
  <div class="space-y-8">
    <PageHeader title="Calls" subtitle="Place calls and review your device call log" />

    <!-- Make a call -->
    <form class="rounded-lg border border-subtle bg-card shadow-sm" @submit.prevent="makeCall">
      <div class="border-b border-subtle px-6 py-4">
        <div class="text-base font-semibold text-primary">New call</div>
        <div class="mt-0.5 font-mono text-[11px] text-muted">POST /api/calls</div>
      </div>

      <div class="space-y-5 p-6">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Phone number</label>
          <input v-model="phone" type="tel" class="gn-input font-mono !text-xs" placeholder="+15551234567" />
        </div>
        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Device</label>
          <select v-model="deviceId" class="gn-input">
            <option value="">Auto (most recent)</option>
            <option v-for="d in devices" :key="d.id" :value="d.device_id">
              {{ d.name || d.device_id }}
            </option>
          </select>
        </div>

        <p v-if="error" class="rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>
        <p v-if="result" class="rounded-md bg-success-tint px-3 py-2 text-sm text-success">
          Queued — call <span class="font-mono text-xs">{{ result.id }}</span> ({{ result.status }})
        </p>

        <button type="submit" :disabled="calling" class="gn-btn-pri">
          <Phone class="h-4 w-4" />{{ calling ? "Queuing…" : "Place call" }}
        </button>
      </div>
    </form>

    <!-- Call log -->
    <div>
      <div class="mb-3 flex items-center justify-between">
        <div class="inline-flex rounded-md border border-subtle bg-sunken p-0.5">
          <button
            v-for="f in ['all', 'incoming', 'outgoing']"
            :key="f"
            type="button"
            class="rounded-[5px] px-3 py-1 text-sm font-medium capitalize transition-colors"
            :class="filter === f ? 'bg-card text-primary shadow-xs' : 'text-muted hover:text-primary'"
            @click="filter = f"
          >{{ f }}</button>
        </div>
        <button class="gn-btn-sec gn-btn-sm" @click="loadLog">
          <RefreshCw class="h-3.5 w-3.5" />Refresh
        </button>
      </div>

      <p v-if="logError" class="mb-4 rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ logError }}</p>

      <div v-if="logLoading" class="text-sm text-muted">Loading…</div>
      <div
        v-else-if="!visibleCalls.length"
        class="rounded-lg border border-dashed border-strong p-10 text-center text-sm text-muted"
      >
        No calls logged yet.
      </div>
      <div v-else class="space-y-2">
        <div
          v-for="c in visibleCalls"
          :key="c.id"
          class="flex items-center gap-3 rounded-lg border border-subtle bg-card p-3 shadow-xs"
        >
          <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-sm" :class="callTone(c)">
            <component :is="callIcon(c)" class="h-4 w-4" />
          </span>
          <div class="min-w-0 flex-1">
            <div class="flex items-baseline gap-2">
              <span class="font-mono text-sm font-medium text-primary">{{ c.phone_number }}</span>
              <span class="rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] uppercase text-muted">{{ c.direction }}</span>
              <span v-if="c.status" class="text-xs capitalize text-muted">{{ c.status }}</span>
              <span v-if="c.sim_slot != null" class="font-mono text-[10px] text-muted">SIM {{ c.sim_slot }}</span>
            </div>
          </div>
          <span v-if="c.duration != null" class="shrink-0 font-mono text-xs text-muted">{{ duration(c.duration) }}</span>
          <span class="shrink-0 font-mono text-[11px] text-muted">{{ fmt(c.started_at || c.created_at) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
