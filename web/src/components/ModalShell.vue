<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue';

const props = defineProps<{ labelledby: string; describedby?: string; initialFocus?: string; width?: number }>();
const emit = defineEmits<{ close: [] }>();

const root = ref<HTMLElement | null>(null);
let returnTo: HTMLElement | null = null;

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

function focusables(): HTMLElement[] {
  return root.value ? Array.from(root.value.querySelectorAll<HTMLElement>(FOCUSABLE)) : [];
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation();
    emit('close');
    return;
  }
  if (e.key !== 'Tab') return;
  const list = focusables();
  if (!list.length) return;
  const first = list[0];
  const last = list[list.length - 1];
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault();
    first.focus();
  }
}

onMounted(async () => {
  returnTo = document.activeElement as HTMLElement | null;
  await nextTick();
  const target = props.initialFocus ? root.value?.querySelector<HTMLElement>(props.initialFocus) : null;
  (target ?? focusables()[0])?.focus();
});

onBeforeUnmount(() => {
  if (returnTo && document.contains(returnTo)) returnTo.focus();
});
</script>

<template>
  <div class="backdrop" @mousedown.self="emit('close')">
    <div
      ref="root"
      class="modal"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="labelledby"
      :aria-describedby="describedby"
      :style="{ width: `${width ?? 520}px` }"
      @keydown="onKeydown"
    >
      <slot />
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: grid;
  place-items: center;
  background: rgba(28, 27, 24, 0.38);
  animation: fade 140ms ease-out;
}
.modal {
  max-width: calc(100vw - 32px);
  max-height: calc(100vh - 48px);
  overflow: auto;
  background: var(--panel);
  border: 1px solid var(--rule);
  border-radius: var(--radius-lg);
  box-shadow:
    0 1px 0 rgba(28, 27, 24, 0.04),
    0 24px 60px -12px rgba(28, 27, 24, 0.35);
  animation: rise 180ms cubic-bezier(0.2, 0.8, 0.2, 1);
}
@keyframes fade {
  from {
    opacity: 0;
  }
}
@keyframes rise {
  from {
    opacity: 0;
    transform: translateY(8px) scale(0.985);
  }
}
</style>
