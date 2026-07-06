<script setup>
import { ref, onMounted } from "vue";
import { Phone } from "@lucide/vue";
import { api } from "../api";
import PageHeader from "../components/PageHeader.vue";

const devices = ref([]);
const phone = ref("");
const deviceId = ref("");
const calling = ref(false);
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
</script>

<template>
  <div class="max-w-2xl">
    <PageHeader title="Make a call" subtitle="Tell a device to place an outbound phone call" />

    <form class="rounded-lg border border-subtle bg-card shadow-sm" @submit.prevent="makeCall">
      <div class="border-b border-subtle px-6 py-4">
        <div class="text-base font-semibold text-primary">New call</div>
        <div class="mt-0.5 font-mono text-[11px] text-muted">POST /api/calls</div>
      </div>

      <div class="space-y-5 p-6">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-primary">Phone number</label>
          <input
            v-model="phone"
            type="tel"
            class="gn-input font-mono !text-xs"
            placeholder="+15551234567"
          />
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

        <p class="text-xs text-muted">
          The selected device must be online with the gateway running and granted
          phone-call permission. The call is placed from that phone.
        </p>
      </div>
    </form>
  </div>
</template>
