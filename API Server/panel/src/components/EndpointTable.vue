<script setup>
defineProps({
  title: String,
  auth: String,
  endpoints: Array, // [{ method, path, desc }]
});

const methodClass = {
  GET: "text-success",
  POST: "text-brand-text",
  PATCH: "text-warning",
  DELETE: "text-danger",
};
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-subtle bg-card shadow-xs">
    <div class="flex items-center justify-between border-b border-subtle px-5 py-4">
      <div class="text-base font-semibold text-primary">{{ title }}</div>
      <span class="gn-eyebrow">{{ auth }}</span>
    </div>
    <table class="w-full text-left text-sm">
      <thead>
        <tr class="gn-eyebrow">
          <th class="px-5 py-2.5 font-medium">Endpoint</th>
          <th class="px-5 py-2.5 font-medium">Description</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="e in endpoints" :key="e.method + e.path" class="transition-colors hover:bg-sunken">
          <td class="border-t border-subtle px-5 py-2.5 font-mono text-xs whitespace-nowrap">
            <span class="font-semibold" :class="methodClass[e.method]">{{ e.method }}</span>
            <span class="text-primary"> {{ e.path }}</span>
          </td>
          <td class="border-t border-subtle px-5 py-2.5 text-secondary">{{ e.desc }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
