<script setup lang="ts">
import { STATE_LABEL, type SessionState } from '../lib/types';
defineProps<{ state: SessionState }>();
</script>

<template>
  <span class="badge" :class="`badge--${state}`" data-testid="state-badge">
    <span class="dot" aria-hidden="true" />{{ STATE_LABEL[state] }}
  </span>
</template>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 22px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 500;
  letter-spacing: 0.01em;
  white-space: nowrap;
  border: 1px solid var(--rule);
  background: var(--panel);
  color: var(--ink-2);
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}
.badge--running {
  color: var(--ok);
  border-color: #b9dcc8;
  background: #eef7f1;
}
.badge--starting {
  color: var(--start);
  border-color: #c5d0e8;
  background: #eef2fa;
}
.badge--starting .dot {
  animation: pulse 1.1s ease-in-out infinite;
}
.badge--needs-input {
  color: var(--wait-fg);
  border-color: var(--wait-rule);
  background: #fff6e0;
  font-weight: 600;
}
.badge--needs-input .dot {
  background: var(--wait-rule);
  animation: pulse 1.4s ease-in-out infinite;
}
.badge--exited {
  color: var(--muted);
}
.badge--resume-failed {
  color: var(--err);
  border-color: #efb8b2;
  background: var(--err-soft);
}
@keyframes pulse {
  50% {
    opacity: 0.35;
  }
}
</style>
