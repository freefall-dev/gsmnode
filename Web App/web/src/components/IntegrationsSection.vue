<script setup>
import { ref, onMounted } from "vue";
import { api } from "../api";
import IntegrationCard from "./IntegrationCard.vue";

// Every plugin that offers per-user settings, fetched already resolved through
// the server's cascade. One request renders the whole section; the server
// decides which plugins the caller may configure, so adding a plugin needs no
// change here.

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
  <!-- Stay silent until we know there is something to show, so a deployment
       with no user-configurable plugins renders no empty heading. -->
  <template v-if="error || (loaded && integrations.length)">
    <h3 class="gn-eyebrow mb-3">Integrations</h3>
    <p v-if="error" class="mb-6 rounded-lg border border-subtle bg-card p-5 text-sm text-danger shadow-xs">
      {{ error }}
    </p>
    <IntegrationCard
      v-for="i in integrations"
      :key="i.name"
      :integration="i"
    />
  </template>
</template>
