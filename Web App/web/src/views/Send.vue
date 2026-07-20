<script setup>
import { ref, computed, onMounted } from "vue";
import { Send, Lock } from "@lucide/vue";
import { api } from "../api";
import { encryptString, encryptList, encryptionEnabled } from "../crypto";
import PageHeader from "../components/PageHeader.vue";

const devices = ref([]);
const kind = ref("sms"); // "sms" | "data" | "mms"
const phones = ref("");
const text = ref("");
const subject = ref(""); // MMS
const dataPayload = ref(""); // data SMS (base64 or plain text — see toBase64)
const dataPayloadIsText = ref(true); // encode a typed string to base64 on send
const dataPort = ref("0");
const attachments = ref([]); // MMS: [{filename, content_type, data(base64)}]
const deviceId = ref("");
const simSlot = ref(""); // "" = device default SIM; otherwise a 0-based slot
const encrypt = ref(encryptionEnabled());
const sending = ref(false);
const result = ref(null);
const error = ref("");

const canEncrypt = computed(() => encryptionEnabled());

// SIMs advertised by the chosen device (empty when "Auto" or none reported yet).
const deviceSims = computed(() => {
  const d = devices.value.find((x) => x.device_id === deviceId.value);
  return d?.sims || [];
});

function simOptionLabel(sim) {
  const name = sim.carrier || sim.display_name || "SIM";
  return sim.number
    ? `Slot ${sim.slot} · ${name} · ${sim.number}`
    : `Slot ${sim.slot} · ${name}`;
}

onMounted(async () => {
  try {
    const res = await api.get("/devices");
    devices.value = res.items || [];
  } catch {
    /* listing failure is non-fatal for the form */
  }
});

// Read a File into a base64 string (no data: prefix).
function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const res = reader.result || "";
      const comma = res.indexOf(",");
      resolve(comma >= 0 ? res.slice(comma + 1) : res);
    };
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

async function onFiles(e) {
  const files = Array.from(e.target.files || []);
  for (const f of files) {
    attachments.value.push({
      filename: f.name,
      content_type: f.type || "application/octet-stream",
      data: await fileToBase64(f),
      _size: f.size,
    });
  }
  e.target.value = "";
}

function removeAttachment(i) {
  attachments.value.splice(i, 1);
}

function utf8ToBase64(s) {
  return btoa(String.fromCharCode(...new TextEncoder().encode(s)));
}

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
  if (kind.value === "sms" && !text.value.trim()) {
    error.value = "Message text is required.";
    return;
  }
  if (kind.value === "data" && !dataPayload.value.trim()) {
    error.value = "Data payload is required.";
    return;
  }
  if (kind.value === "mms" && !text.value.trim() && !attachments.value.length) {
    error.value = "MMS needs text or at least one attachment.";
    return;
  }

  sending.value = true;
  try {
    const useEnc = encrypt.value && canEncrypt.value;
    // Recipients and text can be end-to-end encrypted. Data payload and MMS
    // attachments are left as-is (binary already opaque to the server).
    const body = {
      type: kind.value,
      phone_numbers: useEnc ? await encryptList(phoneList) : phoneList,
      encrypted: useEnc,
    };
    if (kind.value === "sms" || kind.value === "mms") {
      body.text_message = useEnc ? await encryptString(text.value) : text.value;
    }
    if (kind.value === "mms") {
      body.subject = subject.value;
      body.attachments = attachments.value.map((a) => ({
        filename: a.filename,
        content_type: a.content_type,
        data: a.data,
      }));
    }
    if (kind.value === "data") {
      body.data_payload = dataPayloadIsText.value
        ? utf8ToBase64(dataPayload.value)
        : dataPayload.value.trim();
      body.data_port = Number(dataPort.value) || 0;
    }
    if (deviceId.value) body.device_id = deviceId.value;
    if (simSlot.value !== "") body.sim_number = Number(simSlot.value);

    result.value = await api.post("/messages", body);
    text.value = "";
    dataPayload.value = "";
    attachments.value = [];
  } catch (e) {
    error.value = e.message;
  } finally {
    sending.value = false;
  }
}

const tabs = [
  { id: "sms", label: "SMS" },
  { id: "data", label: "Data SMS" },
  { id: "mms", label: "MMS" },
];
</script>

<template>
  <div>
    <PageHeader title="Send message" subtitle="Queue an SMS, data SMS, or MMS for a device" />

    <form class="rounded-lg border border-subtle bg-card shadow-sm" @submit.prevent="send">
      <div class="border-b border-subtle px-6 py-4">
        <div class="flex items-center justify-between gap-3">
          <div class="inline-flex rounded-md border border-subtle bg-sunken p-0.5">
            <button
              v-for="t in tabs"
              :key="t.id"
              type="button"
              class="rounded-[5px] px-3 py-1 text-sm font-medium transition-colors"
              :class="kind === t.id ? 'bg-card text-primary shadow-xs' : 'text-muted hover:text-primary'"
              @click="kind = t.id"
            >{{ t.label }}</button>
          </div>
          <div class="font-mono text-[11px] text-muted">POST /api/messages</div>
        </div>
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

        <!-- SMS / MMS text -->
        <div v-if="kind === 'sms' || kind === 'mms'">
          <label class="mb-1.5 block text-sm font-medium text-primary">
            {{ kind === 'mms' ? 'Message (optional)' : 'Message' }}
          </label>
          <textarea v-model="text" rows="4" class="gn-textarea" placeholder="Your message…"></textarea>
        </div>

        <!-- MMS subject + attachments -->
        <template v-if="kind === 'mms'">
          <div>
            <label class="mb-1.5 block text-sm font-medium text-primary">Subject (optional)</label>
            <input v-model="subject" class="gn-input" placeholder="Subject line" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium text-primary">Attachments</label>
            <input type="file" multiple class="block text-sm text-secondary" @change="onFiles" />
            <ul v-if="attachments.length" class="mt-2 space-y-1">
              <li
                v-for="(a, i) in attachments"
                :key="i"
                class="flex items-center justify-between rounded-sm bg-sunken px-2 py-1 text-xs"
              >
                <span class="font-mono text-secondary">{{ a.filename }} · {{ a.content_type }}</span>
                <button type="button" class="text-danger hover:underline" @click="removeAttachment(i)">remove</button>
              </li>
            </ul>
          </div>
        </template>

        <!-- Data SMS payload -->
        <template v-if="kind === 'data'">
          <div>
            <div class="mb-1.5 flex items-center justify-between">
              <label class="block text-sm font-medium text-primary">Payload</label>
              <label class="flex items-center gap-1.5 text-xs text-muted">
                <input v-model="dataPayloadIsText" type="checkbox" />
                encode text as base64
              </label>
            </div>
            <textarea
              v-model="dataPayload"
              rows="3"
              class="gn-textarea font-mono !text-xs"
              :placeholder="dataPayloadIsText ? 'Any text (will be base64-encoded)…' : 'Base64-encoded bytes…'"
            ></textarea>
          </div>
          <div class="w-40">
            <label class="mb-1.5 block text-sm font-medium text-primary">Destination port</label>
            <input v-model="dataPort" type="number" min="0" class="gn-input" placeholder="0" />
          </div>
        </template>

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
            <label class="mb-1.5 block text-sm font-medium text-primary">SIM (optional)</label>
            <select v-if="deviceSims.length" v-model="simSlot" class="gn-input">
              <option value="">Default SIM</option>
              <option v-for="s in deviceSims" :key="s.slot" :value="String(s.slot)">
                {{ simOptionLabel(s) }}
              </option>
            </select>
            <input v-else v-model="simSlot" type="number" min="0" class="gn-input" placeholder="Default" />
            <p class="mt-1.5 text-xs text-muted">0-based slot; blank uses the device default.</p>
          </div>
        </div>

        <!-- Encryption -->
        <label
          v-if="kind !== 'data'"
          class="flex items-center gap-2 text-sm"
          :class="canEncrypt ? 'text-primary' : 'text-muted'"
        >
          <input v-model="encrypt" type="checkbox" :disabled="!canEncrypt" />
          <Lock class="h-3.5 w-3.5" />
          End-to-end encrypt recipients + text
          <span v-if="!canEncrypt" class="text-xs">(set a passphrase in Settings)</span>
        </label>

        <p v-if="error" class="rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>
        <p v-if="result" class="rounded-md bg-success-tint px-3 py-2 text-sm text-success">
          Queued — message <span class="font-mono text-xs">{{ result.id }}</span> ({{ result.status }})
        </p>

        <button type="submit" :disabled="sending" class="gn-btn-pri">
          <Send class="h-4 w-4" />{{ sending ? "Queuing…" : "Send" }}
        </button>
      </div>
    </form>
  </div>
</template>
