<script setup>
import { ref, onMounted, reactive } from "vue";
import { request } from "../api";

// Superadmin-only. Each plugin advertises its own config fields (Descriptor.
// ConfigFields), so the form below is generated rather than hard-coded — that's
// what lets an external plugin be added without touching this panel.
const plugins = ref([]);
const error = ref("");
const busy = ref(false);
const open = ref(null); // name of the expanded plugin
const drafts = reactive({}); // name -> { key: value }
const rowNotice = reactive({}); // name -> string
const showRegister = ref(false);
const reg = ref({ name: "", baseURL: "", provider: "" });

async function load() {
  try {
    const out = await request("/api/admin/plugins");
    plugins.value = out.plugins || [];
    error.value = "";
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

function expand(p) {
  if (open.value === p.name) {
    open.value = null;
    return;
  }
  // Seed the draft from the (secret-masked) stored config plus field defaults.
  const d = {};
  for (const f of p.configFields || []) d[f.key] = p.config?.[f.key] ?? "";
  drafts[p.name] = d;
  open.value = p.name;
}

async function save(p, enabled) {
  busy.value = true;
  rowNotice[p.name] = "";
  try {
    const out = await request(`/api/admin/plugins/${encodeURIComponent(p.name)}`, {
      method: "PUT",
      body: { enabled, config: drafts[p.name] ?? {} },
    });
    // A save can succeed while Init fails (e.g. a port in use) — the server
    // returns the saved plugin plus a warning.
    rowNotice[p.name] = out.warning || "Saved.";
    await load();
  } catch (e) {
    rowNotice[p.name] = e.message;
  } finally {
    busy.value = false;
  }
}

async function health(p) {
  busy.value = true;
  rowNotice[p.name] = "Checking…";
  try {
    const out = await request(`/api/admin/plugins/${encodeURIComponent(p.name)}/health`, {
      method: "POST",
    });
    rowNotice[p.name] = `${out.health.status}${out.health.detail ? " — " + out.health.detail : ""}`;
    await load();
  } catch (e) {
    rowNotice[p.name] = e.message;
  } finally {
    busy.value = false;
  }
}

async function remove(p) {
  if (!confirm(`Remove the external plugin "${p.name}"?`)) return;
  busy.value = true;
  try {
    await request(`/api/admin/plugins/${encodeURIComponent(p.name)}`, { method: "DELETE" });
    if (open.value === p.name) open.value = null;
    await load();
  } catch (e) {
    rowNotice[p.name] = e.message;
  } finally {
    busy.value = false;
  }
}

async function registerExternal() {
  busy.value = true;
  error.value = "";
  try {
    await request("/api/admin/plugins", { method: "POST", body: reg.value });
    reg.value = { name: "", baseURL: "", provider: "" };
    showRegister.value = false;
    await load();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}

const healthClass = (s) =>
  s === "ok"
    ? "bg-success-tint text-success"
    : s === "degraded"
      ? "bg-warning-tint text-warning"
      : "bg-danger-tint text-danger";
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-sm">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div>
        <div class="text-base font-semibold text-primary">Plugins</div>
        <p class="mt-0.5 text-xs text-muted">
          Extension services managed by a superadmin. Built-ins ship with the server; external
          plugins are remote HTTP services registered at runtime.
        </p>
      </div>
      <button class="gn-btn-sec gn-btn-sm" @click="showRegister = !showRegister">
        {{ showRegister ? "Cancel" : "Register external" }}
      </button>
    </div>

    <!-- Register an external (remote HTTP) plugin — the no-rebuild path. -->
    <div v-if="showRegister" class="border-b border-subtle bg-sunken px-5 py-4">
      <div class="grid gap-3 sm:grid-cols-3">
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">Name</label>
          <input v-model="reg.name" class="gn-input" placeholder="acme-cloud" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">Base URL</label>
          <input v-model="reg.baseURL" class="gn-input" placeholder="http://127.0.0.1:9100" />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-secondary">Provider</label>
          <input v-model="reg.provider" class="gn-input" placeholder="ACME Corp" />
        </div>
      </div>
      <button
        class="gn-btn-pri gn-btn-sm mt-3"
        :disabled="busy || !reg.name || !reg.baseURL"
        @click="registerExternal"
      >
        Register
      </button>
    </div>

    <p v-if="error" class="border-b border-subtle px-5 py-3 text-xs text-danger">{{ error }}</p>

    <p v-if="!plugins.length" class="px-5 py-6 text-center text-sm text-muted">
      No plugins available.
    </p>

    <div v-for="p in plugins" :key="p.name" class="border-t border-subtle first:border-t-0">
      <!-- Summary row -->
      <div class="flex flex-wrap items-center gap-3 px-5 py-3">
        <button class="flex flex-1 items-center gap-3 text-left" @click="expand(p)">
          <span class="font-semibold text-primary">{{ p.name }}</span>
          <span class="rounded-sm bg-sunken px-2 py-0.5 font-mono text-xs text-muted">{{ p.kind || "builtin" }}</span>
          <span v-if="p.provider" class="text-xs text-muted">{{ p.provider }}</span>
          <span
            v-if="p.health"
            class="inline-flex items-center gap-1.5 rounded-sm px-2 py-0.5 font-mono text-xs font-medium"
            :class="healthClass(p.health.status)"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-current"></span>{{ p.health.status }}
          </span>
        </button>
        <span
          class="inline-flex rounded-sm px-2 py-0.5 font-mono text-xs font-medium"
          :class="p.enabled ? 'bg-success-tint text-success' : 'bg-sunken text-muted'"
        >
          {{ p.enabled ? "enabled" : "disabled" }}
        </span>
        <button class="gn-btn-sec gn-btn-sm" :disabled="busy" @click="health(p)">Health</button>
        <button class="gn-btn-sec gn-btn-sm" :disabled="busy" @click="expand(p)">
          {{ open === p.name ? "Close" : "Configure" }}
        </button>
      </div>

      <!-- Expanded config: generated from the plugin's declared fields. -->
      <div v-if="open === p.name" class="bg-sunken px-5 py-4">
        <div v-if="p.baseURL" class="mb-3 font-mono text-xs text-muted">{{ p.baseURL }}</div>

        <div v-if="(p.configFields || []).length" class="grid gap-3 sm:grid-cols-2">
          <div v-for="f in p.configFields" :key="f.key">
            <label class="mb-1 block text-xs font-medium text-secondary">
              {{ f.label || f.key }}<span v-if="f.required" class="text-danger"> *</span>
            </label>
            <select v-if="f.type === 'select'" v-model="drafts[p.name][f.key]" class="gn-input">
              <option v-if="!f.required" value="">Not set</option>
              <option v-for="o in f.options || []" :key="o.value" :value="o.value">
                {{ o.label || o.value }}
              </option>
            </select>
            <input
              v-else
              v-model="drafts[p.name][f.key]"
              class="gn-input"
              :type="f.type === 'password' ? 'password' : f.type === 'number' ? 'number' : 'text'"
              :placeholder="f.default || ''"
              autocomplete="off"
            />
            <p v-if="f.help" class="mt-1 text-xs text-muted">{{ f.help }}</p>
          </div>
        </div>
        <p v-else class="text-xs text-muted">This plugin has no configurable settings.</p>

        <p v-if="rowNotice[p.name]" class="mt-3 font-mono text-xs text-secondary">{{ rowNotice[p.name] }}</p>

        <div class="mt-4 flex items-center gap-2">
          <button class="gn-btn-pri gn-btn-sm" :disabled="busy" @click="save(p, true)">
            {{ p.enabled ? "Save" : "Save & enable" }}
          </button>
          <button v-if="p.enabled" class="gn-btn-sec gn-btn-sm" :disabled="busy" @click="save(p, false)">
            Disable
          </button>
          <span class="flex-1"></span>
          <button
            v-if="p.kind === 'external'"
            class="gn-btn-sec gn-btn-sm !border-danger !text-danger"
            :disabled="busy"
            @click="remove(p)"
          >
            Remove
          </button>
        </div>
        <p v-if="p.kind !== 'external'" class="gn-eyebrow mt-2">Built-in — can be disabled, not removed.</p>
      </div>
    </div>
  </div>
</template>
