<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api";
import IntegrationCard from "./IntegrationCard.vue";

// Every plugin that offers per-user settings, fetched already resolved through
// the server's cascade. One request renders the whole section; the server
// decides which plugins the caller may configure, so adding a plugin needs no
// change here.
//
// Rendered inside the Settings page's Integrations tab, which supplies the
// heading — hence none here.

const integrations = ref([]);
const loaded = ref(false);
const error = ref("");

onMounted(async () => {
  try {
    const res = await api.get("/integrations");
    integrations.value = res.integrations || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loaded.value = true;
  }
});
</script>

<template>
  <div>
    <p v-if="!loaded" class="text-sm text-muted">Loading…</p>

    <p
      v-else-if="error"
      class="rounded-lg border border-subtle bg-card p-5 text-sm text-danger shadow-xs"
    >{{ error }}</p>

    <p
      v-else-if="!integrations.length"
      class="rounded-lg border border-subtle bg-card p-5 text-sm text-muted shadow-xs"
    >
      No integrations are available. A superadmin enables them in the API Server's
      Plugins panel.
    </p>

    <template v-else>
      <IntegrationCard v-for="i in integrations" :key="i.name" :integration="i" />
    </template>
  </div>
</template>
