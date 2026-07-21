<script setup>
import { ref, reactive, computed, watch } from "vue";
import { api } from "../api";

// One plugin's per-user settings, rendered entirely from the schema the server
// sends (`spec`). Nothing here knows a specific plugin's fields — a new plugin
// that declares per-user settings shows up as another card with no UI changes.
//
// Values resolve through the server-side cascade (global → org → user): a field
// set by a layer above the user arrives `locked`, with `source` naming who set
// it, and secrets arrive masked.

const props = defineProps({
  integration: { type: Object, required: true },
});

const SECRET_MASK = "••••••••";

const view = ref(props.integration);
const draft = reactive({});
const enabled = ref(false);
const busy = ref(false);
const error = ref("");
const notice = ref("");
const health = ref(null);

const spec = computed(() => view.value?.spec || { fields: [] });
const fields = computed(() => spec.value.fields || []);
const userScope = computed(() => view.value?.scopes?.user || null);
const canEdit = computed(() => view.value && !view.value.isSuperadmin);

function seed(v) {
  view.value = v;
  enabled.value = !!v.enabled;
  const resolved = v.scopes?.user?.fields || {};
  for (const f of fields.value) draft[f.key] = resolved[f.key]?.own ?? "";
}
seed(props.integration);
watch(() => props.integration, seed);

function locked(key) {
  return userScope.value?.fields?.[key]?.locked ?? false;
}
function sourceOf(key) {
  return userScope.value?.fields?.[key]?.source ?? "unset";
}
// A locked field shows what is actually in force, not the user's own blank.
function displayValue(key) {
  return locked(key) ? (userScope.value?.fields?.[key]?.effective ?? "") : draft[key];
}
function inputType(f) {
  if (f.type === "password") return "password";
  if (f.type === "number") return "number";
  return "text";
}

async function save() {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    const config = {};
    for (const f of fields.value) if (!locked(f.key)) config[f.key] = draft[f.key];
    seed(await api.put(`/integrations/${view.value.name}`, {
      scope: "user",
      enabled: enabled.value,
      config,
    }));
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
    const out = await api.post(`/integrations/${view.value.name}/health`, {});
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
    <h3 class="gn-eyebrow mb-4">{{ spec.title || view.name }}</h3>

    <p v-if="!view.available" class="rounded-md bg-sunken px-3 py-2 text-sm text-muted">
      The {{ spec.title || view.name }} integration is turned off by your administrator.
    </p>
    <p v-else-if="!view.orgEnabled" class="rounded-md bg-sunken px-3 py-2 text-sm text-muted">
      The {{ spec.title || view.name }} integration is turned off for your organization.
    </p>

    <template v-else>
      <p v-if="spec.description" class="mb-3 max-w-prose text-sm text-secondary">
        {{ spec.description }}
      </p>

      <p v-if="view.isSuperadmin" class="mb-3 rounded-md bg-sunken px-3 py-2 text-sm text-muted">
        You are a superadmin — manage the global settings in the API Server's
        <span class="font-medium">Plugins</span> panel. The form below is available to
        regular users.
      </p>

      <template v-if="canEdit">
        <label v-if="spec.enableLabel" class="mb-3 flex items-center gap-2 text-sm text-secondary">
          <input type="checkbox" v-model="enabled" class="h-4 w-4" />
          {{ spec.enableLabel }}
        </label>

        <div class="grid max-w-xl gap-3 sm:grid-cols-2">
          <div v-for="f in fields" :key="f.key">
            <label class="mb-1 block text-sm font-medium text-secondary">
              {{ f.label }}
              <span
                v-if="locked(f.key)"
                class="ml-1 rounded-sm bg-sunken px-1.5 py-0.5 font-mono text-[10px] text-muted"
              >set by {{ sourceOf(f.key) }}</span>
            </label>

            <select
              v-if="f.type === 'select'"
              v-model="draft[f.key]"
              class="gn-input"
              :disabled="locked(f.key)"
            >
              <option v-for="o in f.options || []" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
            <input
              v-else
              :value="displayValue(f.key)"
              @input="draft[f.key] = $event.target.value"
              class="gn-input"
              :type="inputType(f)"
              :placeholder="f.default || ''"
              :disabled="locked(f.key)"
              autocomplete="off"
            />

            <p v-if="f.help" class="mt-1 text-xs text-muted">{{ f.help }}</p>
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
