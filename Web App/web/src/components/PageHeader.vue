<script setup>
defineProps({ title: String, subtitle: String });
</script>

<template>
  <div class="mb-6 flex items-end justify-between gap-4">
    <div>
      <h2 class="text-2xl font-semibold tracking-[-0.02em] text-primary">{{ title }}</h2>
      <p v-if="subtitle" class="mt-0.5 text-sm text-secondary">{{ subtitle }}</p>
    </div>
    <slot name="actions" />
  </div>
</template>
