<script setup>
import { ref, onMounted } from "vue";
import { Send } from "@lucide/vue";
import { api } from "../api";
import PageHeader from "../components/PageHeader.vue";

const devices = ref([]);
const phones = ref("");
const text = ref("");
const deviceId = ref("");
const simNumber = ref("");
const sending = ref(false);
const result = ref(null);
const error = ref("");

onMounted(async () => {
  try {
    const res = await api.get("/devices");
    devices.value = res.items || [];
  } catch {
    /* listing failure is non-fatal for the form */
  }
});

async function send() {
  error.value = "";
  result.value = null;

  const phoneList = phones.value
    .split(/[\n,;]+/)
    .map((p) => p.trim())
    .filter(Boolean);

  if (!phoneList.length) {
    error.value = "Enter at least one phone number.";
    return;
  }
  if (!text.value.trim()) {
    error.value = "Message text is required.";
    return;
  }

  sending.value = true;
  try {
    const body = { phone_numbers: phoneList, text_message: text.value };
    if (deviceId.value) body.device_id = deviceId.value;
    if (simNumber.value) body.sim_number = Number(simNumber.value);
    result.value = await api.post("/messages", body);
    text.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    sending.value = false;
  }
}
</script>

<template>
  <div class="max-w-2xl">
    <PageHeader title="Send SMS" subtitle="Queue an outbound message for a device" />

    <form class="rounded-lg border border-subtle bg-card shadow-sm" @submit.prevent="send">
      <div class="border-b border-subtle px-6 py-4">
        <div class="text-base font-semibold text-primary">New message</div>
        <div class="mt-0.5 font-mono text-[11px] text-muted">POST /api/messages</div>
      </div>

      <div class="space-y-5 p-6">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Phone numbers</label>
          <textarea
            v-model="phones"
            rows="2"
            class="gn-textarea font-mono !text-xs"
            placeholder="+15551234567, +15559876543"
          ></textarea>
          <p class="mt-1.5 text-xs text-muted">Separate multiple numbers with commas or new lines.</p>
        </div>

        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Message</label>
          <textarea
            v-model="text"
            rows="4"
            class="gn-textarea"
            placeholder="Your message…"
          ></textarea>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="mb-1.5 block text-sm font-medium text-primary">Device</label>
            <select v-model="deviceId" class="gn-input">
              <option value="">Auto (most recent)</option>
              <option v-for="d in devices" :key="d.id" :value="d.device_id">
                {{ d.name || d.device_id }}
              </option>
            </select>
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium text-primary">SIM number (optional)</label>
            <input v-model="simNumber" type="number" min="1" class="gn-input" placeholder="1" />
          </div>
        </div>

        <p v-if="error" class="rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>
        <p v-if="result" class="rounded-md bg-success-tint px-3 py-2 text-sm text-success">
          Queued — message <span class="font-mono text-xs">{{ result.id }}</span> ({{ result.status }})
        </p>

        <button type="submit" :disabled="sending" class="gn-btn-pri">
          <Send class="h-4 w-4" />{{ sending ? "Queuing…" : "Send message" }}
        </button>
      </div>
    </form>
  </div>
</template>
