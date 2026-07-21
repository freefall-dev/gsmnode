<script setup>
import { ref, reactive, computed, onMounted } from "vue";
import { api } from "../api";

// Per-user Email-to-SMS settings, resolved through the server-side cascade
// (global → org → user). A user connects their own IMAP mailbox; the server
// polls it and enqueues each message as an SMS owned by them. SMTP intake needs
// no settings here — the hint block explains how to send by email.

const FIELDS = [
  { key: "imap_host", label: "IMAP host", type: "text", placeholder: "imap.gmail.com" },
  { key: "imap_port", label: "Port", type: "number", placeholder: "993" },
  { key: "imap_user", label: "Username", type: "text", placeholder: "you@example.com" },
  { key: "imap_password", label: "Password", type: "password", placeholder: "app password" },
  { key: "imap_mailbox", label: "Mailbox", type: "text", placeholder: "INBOX" },
];
const SECRET_MASK = "••••••••";

const view = ref(null);
const draft = reactive({});
const enabled = ref(false);
const busy = ref(false);
const error = ref("");
const notice = ref("");
const health = ref(null);

const userScope = computed(() => view.value?.scopes?.user || null);
const canEdit = computed(() => view.value && !view.value.isSuperadmin);

function seed(v) {
  view.value = v;
  enabled.value = !!v.enabled;
  const fields = v.scopes?.user?.fields || {};
  for (const f of FIELDS) draft[f.key] = fields[f.key]?.own ?? "";
}

async function load() {
  error.value = "";
  try {
    seed(await api.get("/integrations/email-to-sms"));
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

function locked(key) {
  return userScope.value?.fields?.[key]?.locked ?? false;
}
function sourceOf(key) {
  return userScope.value?.fields?.[key]?.source ?? "unset";
}

async function save() {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    const config = {};
    for (const f of FIELDS) if (!locked(f.key)) config[f.key] = draft[f.key];
    seed(await api.put("/integrations/email-to-sms", { scope: "user", enabled: enabled.value, config }));
    notice.value = "Saved.";
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function test() {
  busy.value = true;
  error.value = "";
  notice.value = "";
  health.value = null;
  try {
    const out = await api.post("/integrations/email-to-sms/health", {});
    health.value = out.health;
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

const healthClass = computed(() => {
  const s = health.value?.status;
  return s === "ok"
    ? "bg-success-tint text-success"
    : s === "degraded"
      ? "bg-warning-tint text-warning"
      : "bg-danger-tint text-danger";
});
</script>

<template>
  <section class="mb-6 rounded-lg border border-subtle bg-card p-5 shadow-xs">
    <h3 class="gn-eyebrow mb-4">Email to SMS</h3>

    <p v-if="view && !view.available" class="rounded-md bg-sunken px-3 py-2 text-sm text-muted">
      The Email-to-SMS integration is turned off by your administrator.
    </p>
    <p v-else-if="view && !view.orgEnabled" class="rounded-md bg-sunken px-3 py-2 text-sm text-muted">
      The Email-to-SMS integration is turned off for your organization.
    </p>

    <template v-else-if="view">
      <p class="mb-3 max-w-prose text-sm text-secondary">
        Send an SMS by email. Address a message to
        <span class="font-mono text-xs text-primary">&lt;phone&gt;@&lt;domain&gt;</span> — the body
        becomes the text. Connect your mailbox below to have the server poll it (IMAP), or send
        directly to the gateway's SMTP server authenticating with your gsmnode login.
      </p>

      <p v-if="view.isSuperadmin" class="mb-3 rounded-md bg-sunken px-3 py-2 text-sm text-muted">
        You are a superadmin — manage the global Email-to-SMS settings in the API Server's
        <span class="font-medium">Plugins</span> panel. The mailbox form below is available to
        regular users.
      </p>

      <template v-if="canEdit">
        <label class="mb-3 flex items-center gap-2 text-sm text-secondary">
          <input type="checkbox" v-model="enabled" class="h-4 w-4" />
          Poll my mailbox and turn incoming email into SMS
        </label>

        <div class="grid max-w-xl gap-3 sm:grid-cols-2">
          <div v-for="f in FIELDS" :key="f.key">
            <label class="mb-1 block text-sm font-medium text-secondary">
              {{ f.label }}
              <span v-if="locked(f.key)" class="ml-1 rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-muted">
                set by {{ sourceOf(f.key) }}
              </span>
            </label>
            <input
              v-model="draft[f.key]"
              class="gn-input"
              :type="f.type === 'password' ? 'password' : f.type === 'number' ? 'number' : 'text'"
              :placeholder="f.placeholder"
              :disabled="locked(f.key)"
              autocomplete="off"
            />
          </div>
        </div>

        <div class="mt-4 flex flex-wrap items-center gap-2">
          <button class="gn-btn-pri" :disabled="busy" @click="save">Save</button>
          <button class="gn-btn-sec" :disabled="busy" @click="test">Test connection</button>
          <span
            v-if="health"
            class="rounded-sm px-2 py-0.5 font-mono text-xs"
            :class="healthClass"
          >{{ health.status }}{{ health.detail ? " — " + health.detail : "" }}</span>
        </div>
      </template>

      <p v-if="notice" class="mt-2 text-sm text-success">{{ notice }}</p>
    </template>

    <p v-if="error" class="mt-2 text-sm text-danger">{{ error }}</p>
  </section>
</template>
