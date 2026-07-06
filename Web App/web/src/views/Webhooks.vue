<script setup>
import { ref, onMounted } from "vue";
import { Plus } from "@lucide/vue";
import { api } from "../api";
import PageHeader from "../components/PageHeader.vue";

const hooks = ref([]);
const loading = ref(true);
const error = ref("");

const event = ref("sms:received");
const url = ref("");
const creating = ref(false);

const events = ["sms:received", "sms:sent", "sms:delivered", "sms:failed"];

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await api.get("/webhooks");
    hooks.value = res.items || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

async function create() {
  error.value = "";
  if (!url.value.trim()) {
    error.value = "URL is required.";
    return;
  }
  creating.value = true;
  try {
    const hook = await api.post("/webhooks", { event: event.value, url: url.value.trim() });
    hooks.value.unshift(hook);
    url.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    creating.value = false;
  }
}

async function remove(h) {
  if (!confirm("Delete this webhook?")) return;
  try {
    await api.del("/webhooks/" + h.id);
    hooks.value = hooks.value.filter((x) => x.id !== h.id);
  } catch (e) {
    alert("Could not delete: " + e.message);
  }
}

onMounted(load);
</script>

<template>
  <div class="max-w-3xl">
    <PageHeader title="Webhooks" subtitle="Get notified when messages change state or arrive" />

    <form
      class="mb-6 flex flex-wrap items-end gap-3 rounded-lg border border-subtle bg-card p-4 shadow-xs"
      @submit.prevent="create"
    >
      <div>
        <label class="gn-eyebrow mb-1.5 block">Event</label>
        <select v-model="event" class="gn-input !w-auto font-mono !text-xs">
          <option v-for="e in events" :key="e" :value="e">{{ e }}</option>
        </select>
      </div>
      <div class="flex-1">
        <label class="gn-eyebrow mb-1.5 block">Target URL</label>
        <input
          v-model="url"
          type="url"
          placeholder="https://example.com/hook"
          class="gn-input font-mono !text-xs"
        />
      </div>
      <button type="submit" :disabled="creating" class="gn-btn-pri">
        <Plus class="h-4 w-4" />Add webhook
      </button>
    </form>

    <p v-if="error" class="mb-4 rounded-md bg-danger-tint px-3 py-2 text-sm text-danger">{{ error }}</p>

    <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-xs">
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="gn-eyebrow">
            <th class="px-5 py-3 font-medium">Event</th>
            <th class="px-5 py-3 font-medium">URL</th>
            <th class="px-5 py-3"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="3" class="border-t border-subtle px-5 py-10 text-center text-sm text-muted">Loading…</td>
          </tr>
          <tr v-else-if="!hooks.length">
            <td colspan="3" class="border-t border-subtle px-5 py-10 text-center text-sm text-muted">No webhooks.</td>
          </tr>
          <tr v-for="h in hooks" :key="h.id" class="transition-colors hover:bg-sunken">
            <td class="border-t border-subtle px-5 py-3">
              <span class="rounded-sm bg-brand-tint px-2 py-0.5 font-mono text-xs text-brand-active">{{ h.event }}</span>
            </td>
            <td class="border-t border-subtle px-5 py-3 font-mono text-xs text-secondary">{{ h.url }}</td>
            <td class="border-t border-subtle px-5 py-3 text-right">
              <button class="text-sm font-medium text-danger hover:underline" @click="remove(h)">Delete</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
