<script setup>
import { ref, computed, onMounted } from "vue";
import { RefreshCw, ArrowDownLeft, MessageSquare, Binary, Image } from "@lucide/vue";
import { api } from "../api";
import { tryDecrypt } from "../crypto";
import PageHeader from "../components/PageHeader.vue";

const items = ref([]); // decrypted, all types
const loading = ref(true);
const error = ref("");
const tab = ref("all");

const tabs = [
  { id: "all", label: "All", icon: MessageSquare },
  { id: "sms", label: "SMS", icon: MessageSquare },
  { id: "data", label: "Data SMS", icon: Binary },
  { id: "mms", label: "MMS", icon: Image },
];

const counts = computed(() => ({
  all: items.value.length,
  sms: items.value.filter((m) => (m.type || "sms") === "sms").length,
  data: items.value.filter((m) => m.type === "data").length,
  mms: items.value.filter((m) => m.type === "mms").length,
}));

const visible = computed(() =>
  tab.value === "all"
    ? items.value
    : items.value.filter((m) => (m.type || "sms") === tab.value)
);

function iconFor(type) {
  if (type === "data") return Binary;
  if (type === "mms") return Image;
  return MessageSquare;
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await api.get("/inbox");
    const raw = res.items || [];
    // Decrypt phone number + message for E2E items (best-effort).
    items.value = await Promise.all(
      raw.map(async (m) => ({
        ...m,
        phone_number: m.encrypted ? await tryDecrypt(m.phone_number) : m.phone_number,
        message: m.encrypted ? await tryDecrypt(m.message) : m.message,
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

// Decode a base64 data-SMS payload to a readable preview, falling back to the
// raw base64 when it isn't valid UTF-8 text.
function dataPreview(m) {
  if (!m.data_payload) return "";
  try {
    const bin = atob(m.data_payload);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    return text;
  } catch {
    return m.data_payload;
  }
}

function attachmentHref(a) {
  if (a.url) return a.url;
  return `data:${a.content_type || "application/octet-stream"};base64,${a.data}`;
}

onMounted(load);
</script>

<template>
  <div>
    <PageHeader title="Inbox" subtitle="Incoming SMS, data SMS, and MMS received by your devices">
      <template #actions>
        <button class="gn-btn-sec gn-btn-sm" @click="load">
          <RefreshCw class="h-3.5 w-3.5" />Refresh
        </button>
      </template>
    </PageHeader>

    <!-- Tabs + stats -->
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <button
        v-for="t in tabs"
        :key="t.id"
        class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm font-medium transition-colors"
        :class="tab === t.id
          ? 'border-brand-strong bg-brand-tint text-brand-active'
          : 'border-subtle bg-card text-secondary hover:text-primary'"
        @click="tab = t.id"
      >
        <component :is="t.icon" class="h-3.5 w-3.5" />
        {{ t.label }}
        <span class="rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-muted">{{ counts[t.id] }}</span>
      </button>
    </div>

    <p v-if="error" class="mb-4 rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>

    <div v-if="loading" class="text-sm text-muted">Loading…</div>
    <div
      v-else-if="!visible.length"
      class="rounded-lg border border-dashed border-strong p-10 text-center text-sm text-muted"
    >
      No {{ tab === 'all' ? '' : tab }} messages yet.
    </div>
    <div v-else class="space-y-3">
      <div
        v-for="m in visible"
        :key="m.id"
        class="flex items-start gap-3 rounded-lg border border-subtle bg-card p-4 shadow-xs"
      >
        <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-sm bg-success-tint text-success">
          <component :is="iconFor(m.type)" class="h-4 w-4" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="flex items-baseline justify-between gap-3">
            <div class="flex flex-wrap items-baseline gap-2">
              <span class="font-mono text-sm font-medium text-primary">{{ m.phone_number }}</span>
              <span
                v-if="m.type && m.type !== 'sms'"
                class="rounded-sm bg-brand-tint px-1.5 py-0.5 font-mono text-[10px] uppercase text-brand-active"
              >{{ m.type }}</span>
              <span
                v-if="m.encrypted"
                class="rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-muted"
              >🔒 e2e</span>
              <span
                v-if="m.sim_slot != null"
                class="rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-muted"
              >SIM {{ m.sim_slot }}</span>
            </div>
            <span class="shrink-0 font-mono text-[11px] text-muted">{{ fmt(m.received_at) }}</span>
          </div>

          <!-- SMS / MMS text -->
          <p v-if="m.message" class="mt-1 text-sm text-secondary">{{ m.message }}</p>

          <!-- MMS subject + attachments -->
          <p v-if="m.subject" class="mt-1 text-xs font-medium text-primary">Subject: {{ m.subject }}</p>
          <div v-if="m.attachments && m.attachments.length" class="mt-2 flex flex-wrap gap-2">
            <a
              v-for="(a, i) in m.attachments"
              :key="i"
              :href="attachmentHref(a)"
              target="_blank"
              rel="noopener"
              download
              class="inline-flex items-center gap-1.5 rounded-sm border border-subtle bg-sunken px-2 py-1 text-xs text-secondary hover:text-primary"
            >
              <Image class="h-3.5 w-3.5" />{{ a.filename || a.content_type || "attachment" }}
            </a>
          </div>

          <!-- Data SMS payload -->
          <div v-if="m.type === 'data'" class="mt-1">
            <span
              v-if="m.data_port != null"
              class="mr-2 font-mono text-[11px] text-muted"
            >port {{ m.data_port }}</span>
            <code class="break-all rounded-sm bg-sunken px-2 py-1 font-mono text-xs text-secondary">{{ dataPreview(m) }}</code>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
