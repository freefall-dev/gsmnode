<script setup>
import { ref, onMounted } from "vue";
import { request } from "../api";

// Superadmin-only: retarget the PocketBase this server talks to. The change is
// applied at runtime AND persisted to the server's .env, so it survives a
// restart. A blank password means "keep the stored one".
const cfg = ref(null);
const form = ref({ url: "", adminEmail: "", adminPassword: "" });
const probe = ref(null);
const error = ref("");
const notice = ref("");
const busy = ref(false);

async function load() {
  try {
    cfg.value = await request("/api/admin/pb-config");
    form.value = { url: cfg.value.url, adminEmail: cfg.value.adminEmail, adminPassword: "" };
    probe.value = cfg.value.probe;
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
    probe.value = await request("/api/admin/pb-config/test", { method: "POST", body: form.value });
    notice.value = probe.value.superuser
      ? "Connected — superuser authenticated."
      : probe.value.reachable
        ? "Reachable, but the superuser credentials failed."
        : "PocketBase is unreachable at that URL.";
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
    const out = await request("/api/admin/pb-config", { method: "PUT", body: form.value });
    cfg.value = out.config;
    probe.value = out.config.probe;
    form.value.adminPassword = "";
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
        <div class="text-base font-semibold text-primary">PocketBase</div>
        <p class="mt-0.5 text-xs text-muted">The database this server proxies. Superadmin only.</p>
      </div>
      <span
        v-if="probe"
        class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1 font-mono text-xs font-medium"
        :class="probe.superuser
          ? 'bg-success-tint text-success'
          : probe.reachable
            ? 'bg-warning-tint text-warning'
            : 'bg-danger-tint text-danger'"
      >
        <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
        {{ probe.superuser ? "connected" : probe.reachable ? "no superuser" : "unreachable" }}
      </span>
    </div>

    <div class="flex flex-col gap-4 px-5 py-4">
      <div>
        <label class="mb-1 block text-xs font-medium text-secondary" for="pb-url">Base URL</label>
        <input id="pb-url" v-model="form.url" class="gn-input" placeholder="http://localhost:8028" />
      </div>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary" for="pb-email">Superuser email</label>
          <input id="pb-email" v-model="form.adminEmail" class="gn-input" autocomplete="off" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary" for="pb-password">Superuser password</label>
          <input
            id="pb-password"
            v-model="form.adminPassword"
            class="gn-input"
            type="password"
            autocomplete="new-password"
            :placeholder="cfg?.adminConfigured ? 'unchanged' : 'not set'"
          />
        </div>
      </div>

      <p v-if="probe?.detail" class="font-mono text-xs text-muted">{{ probe.detail }}</p>
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
