<script setup>
import { ref, onMounted } from "vue";
import { request } from "../api";

// Superadmin-only: where the Web App lives (the address /api/status probes) and
// which browser origins CORS admits. Like the PocketBase card, the change is
// applied at runtime AND persisted to the server's .env, so it survives a
// restart. Origins are edited as a comma-separated list.
const cfg = ref(null);
const form = ref({ url: "", origins: "" });
const probe = ref(null);
const error = ref("");
const notice = ref("");
const busy = ref(false);

// The panel sends the origin list as an array; the field edits it as CSV.
const splitOrigins = (s) => s.split(",").map((o) => o.trim()).filter(Boolean);

function apply(view) {
  cfg.value = view;
  form.value = { url: view.url, origins: (view.allowOrigins || []).join(", ") };
  probe.value = view.probe;
}

async function load() {
  try {
    apply(await request("/api/admin/webapp-config"));
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

async function test() {
  error.value = "";
  notice.value = "";
  busy.value = true;
  try {
    probe.value = await request("/api/admin/webapp-config/test", {
      method: "POST",
      body: { url: form.value.url },
    });
    notice.value =
      probe.value.status === "ok"
        ? "Reachable — the Web App answered its health check."
        : "The Web App is unreachable at that URL.";
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

async function save() {
  error.value = "";
  notice.value = "";
  busy.value = true;
  try {
    const out = await request("/api/admin/webapp-config", {
      method: "PUT",
      body: { url: form.value.url, allowOrigins: splitOrigins(form.value.origins) },
    });
    apply(out.config);
    notice.value = out.warning || "Saved and applied.";
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div>
        <div class="text-base font-semibold text-primary">Web App</div>
        <p class="mt-0.5 text-xs text-muted">Where the Web App lives and which browser origins may call this server. Superadmin only.</p>
      </div>
      <span
        v-if="probe"
        class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1 font-mono text-xs font-medium"
        :class="probe.status === 'ok' ? 'bg-success-tint text-success' : 'bg-danger-tint text-danger'"
      >
        <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
        {{ probe.status === "ok" ? "reachable" : "unreachable" }}
      </span>
    </div>

    <div class="flex flex-col gap-4 px-5 py-4">
      <div>
        <label class="mb-1 block text-xs font-medium text-secondary" for="web-url">Base URL</label>
        <input id="web-url" v-model="form.url" class="gn-input" placeholder="http://localhost:8090" />
      </div>

      <div>
        <label class="mb-1 block text-xs font-medium text-secondary" for="web-origins">Allowed origins</label>
        <input
          id="web-origins"
          v-model="form.origins"
          class="gn-input"
          placeholder="http://localhost:8090, https://app.example.com"
        />
        <p class="mt-1 text-xs text-muted">Comma-separated. Use <span class="font-mono">*</span> to allow any origin.</p>
      </div>

      <p v-if="probe?.error" class="font-mono text-xs text-muted">{{ probe.error }}</p>
      <p v-if="notice" class="rounded-sm bg-info-tint px-3 py-2 text-xs text-info">{{ notice }}</p>
      <p v-if="error" class="rounded-sm bg-danger-tint px-3 py-2 text-xs text-danger">{{ error }}</p>

      <div class="flex items-center gap-2">
        <button class="gn-btn-pri gn-btn-sm" :disabled="busy" @click="save">Save &amp; apply</button>
        <button class="gn-btn-sec gn-btn-sm" :disabled="busy" @click="test">Test connection</button>
        <span class="flex-1"></span>
        <span class="gn-eyebrow">persisted to .env</span>
      </div>
    </div>
  </div>
</template>
