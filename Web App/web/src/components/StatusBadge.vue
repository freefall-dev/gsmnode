<script setup>
// Mono badge with a status dot, per the design system: semantic tints only,
// color carries state — no emoji.
const props = defineProps({ status: String });

const styles = {
  Pending: "bg-subtle text-secondary",
  Processed: "bg-brand-tint text-brand-active",
  Sent: "bg-warning-tint text-warning",
  Delivered: "bg-success-tint text-success",
  Failed: "bg-danger-tint text-danger",
  online: "bg-success-tint text-success",
  offline: "bg-subtle text-muted",
};
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-sm px-2 py-0.5 font-mono text-[11px] font-medium"
    :class="styles[props.status] || 'bg-subtle text-secondary'"
  >
    <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
    {{ status || "—" }}
  </span>
</template>
