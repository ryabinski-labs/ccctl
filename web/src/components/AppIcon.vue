<script setup lang="ts">
// Outlined 24px stroke icons in one family (2px stroke, round caps) — decorative;
// every icon-only control carries its own aria-label.
defineProps<{ name: string; size?: number }>();
</script>

<template>
  <svg
    :width="size ?? 18"
    :height="size ?? 18"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
  >
    <template v-if="name === 'maximize'">
      <path d="M14 4h6v6" /><path d="M20 4l-7 7" /><path d="M10 20H4v-6" /><path d="M4 20l7-7" />
    </template>
    <template v-else-if="name === 'restore'">
      <path d="M4 14h6v6" /><path d="M10 14l-7 7" /><path d="M20 10h-6V4" /><path d="M14 10l7-7" />
    </template>
    <template v-else-if="name === 'stop'">
      <rect x="6" y="6" width="12" height="12" rx="1.5" />
    </template>
    <template v-else-if="name === 'close'">
      <path d="M6 6l12 12" /><path d="M18 6L6 18" />
    </template>
    <template v-else-if="name === 'plus'">
      <path d="M12 5v14" /><path d="M5 12h14" />
    </template>
    <template v-else-if="name === 'bell'">
      <path d="M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15z" /><path d="M10 20a2 2 0 0 0 4 0" />
    </template>
    <template v-else-if="name === 'bell-off'">
      <path d="M6 16V11a6 6 0 0 1 9.5-4.9" /><path d="M18 11v5l1.5 2H8" /><path d="M10 20a2 2 0 0 0 4 0" /><path d="M3 3l18 18" />
    </template>
    <template v-else-if="name === 'reader'">
      <circle cx="12" cy="4.5" r="1.5" /><path d="M5 8l7 1.5L19 8" /><path d="M12 9.5V14l-3 6" /><path d="M12 14l3 6" />
    </template>
    <template v-else-if="name === 'keyboard'">
      <rect x="3" y="6" width="18" height="12" rx="2" /><path d="M7 10h.01M11 10h.01M15 10h.01M17 10h.01M7 14h10" />
    </template>
    <template v-else-if="name === 'settings'">
      <path d="M4 7h10M18 7h2M4 17h4M12 17h8" /><circle cx="16" cy="7" r="2" /><circle cx="10" cy="17" r="2" />
    </template>
    <template v-else-if="name === 'key'">
      <circle cx="8" cy="15" r="4" /><path d="M11 12l9-9M17 6l3 3M15 8l2 2" />
    </template>
    <template v-else-if="name === 'refresh'">
      <path d="M20 11a8 8 0 1 0-2.3 5.7" /><path d="M20 5v6h-6" />
    </template>
    <template v-else-if="name === 'folder'">
      <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </template>
    <template v-else-if="name === 'branch'">
      <circle cx="6" cy="5" r="2" /><circle cx="6" cy="19" r="2" /><circle cx="18" cy="7" r="2" /><path d="M6 7v10" /><path d="M18 9c0 5-6 4-12 8" />
    </template>
    <template v-else-if="name === 'search'">
      <circle cx="11" cy="11" r="6" /><path d="M20 20l-4.5-4.5" />
    </template>
    <template v-else-if="name === 'attach'">
      <path d="M20 11.5l-7.8 7.8a5 5 0 0 1-7-7l8.5-8.5a3.3 3.3 0 0 1 4.7 4.7l-8.6 8.5a1.7 1.7 0 0 1-2.4-2.4l7.8-7.8" />
    </template>
    <template v-else-if="name === 'alert'">
      <path d="M12 3l9.5 17h-19z" /><path d="M12 10v4" /><path d="M12 17h.01" />
    </template>
  </svg>
</template>
