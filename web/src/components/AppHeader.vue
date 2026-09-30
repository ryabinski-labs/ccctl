<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useSessions } from '../stores/sessions';
import { TEXT } from '../lib/types';
import AppIcon from './AppIcon.vue';

const store = useSessions();
const keysOpen = ref(false);
const keysWrap = ref<HTMLElement | null>(null);
const keysBtn = ref<HTMLButtonElement | null>(null);

// Disclosure popover: close on Escape (focus back to the button), on a click
// outside, and when focus leaves it, so it never sits over the panes.
function onDocKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && keysOpen.value) {
    keysOpen.value = false;
    keysBtn.value?.focus();
  }
}
function onDocPointer(e: PointerEvent) {
  if (keysOpen.value && !keysWrap.value?.contains(e.target as Node)) keysOpen.value = false;
}
function onFocusOut(e: FocusEvent) {
  if (!keysWrap.value?.contains(e.relatedTarget as Node | null)) keysOpen.value = false;
}
onMounted(() => {
  document.addEventListener('keydown', onDocKey, true);
  document.addEventListener('pointerdown', onDocPointer, true);
});
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onDocKey, true);
  document.removeEventListener('pointerdown', onDocPointer, true);
});

const summary = computed(() => {
  const parts: string[] = [];
  if (store.runningCount) parts.push(`${store.runningCount} running`);
  if (store.needsInputCount) parts.push(`${store.needsInputCount} needs input`);
  const starting = store.slots.filter((s) => s?.state === 'starting').length;
  if (starting) parts.push(`${starting} starting`);
  return parts.length ? parts.join(' · ') : 'No sessions';
});

const newTitle = computed(() => (store.canLaunch ? 'Start a session in the next free slot' : TEXT.allSlotsInUse));
</script>

<template>
  <header class="header">
    <div class="brand">
      <span class="mark" aria-hidden="true"><i /><i /><i /><i /></span>
      <span class="word mono">ccctl</span>
      <span class="sep" aria-hidden="true">/</span>
      <span class="host mono" data-testid="host">{{ store.host }}</span>
      <span
        class="conn"
        :class="store.connected ? 'conn--on' : 'conn--off'"
        :title="store.connected ? 'Connected' : 'Not connected'"
      >
        <span class="sr-only">{{ store.connected ? 'Connected' : 'Not connected' }}</span>
      </span>
    </div>

    <p class="summary" data-testid="summary">
      <span v-if="store.needsInputCount" class="summary-wait">{{ summary }}</span>
      <template v-else>{{ summary }}</template>
    </p>

    <div class="tools" data-header-tools>
      <div ref="keysWrap" class="keys-wrap" @focusout="onFocusOut">
        <button
          type="button"
          class="btn btn--ghost btn--sm"
          ref="keysBtn"
          :aria-expanded="keysOpen"
          aria-controls="keys-pop"
          @click="keysOpen = !keysOpen"
        >
          <AppIcon name="keyboard" :size="16" />Keys
        </button>
        <div v-if="keysOpen" id="keys-pop" class="keys-pop" role="region" aria-label="Keyboard shortcuts" @keydown.esc="keysOpen = false">
          <dl>
            <dt><kbd>Ctrl</kbd><kbd>Shift</kbd><kbd>1</kbd>–<kbd>4</kbd></dt>
            <dd>Type into pane 1–4</dd>
            <dt><kbd>Ctrl</kbd><kbd>Shift</kbd><kbd>0</kbd></dt>
            <dd>Leave the terminals (focus this header)</dd>
            <dt><kbd>Ctrl</kbd><kbd>Shift</kbd><kbd>M</kbd></dt>
            <dd>Maximize or restore the focused pane</dd>
          </dl>
        </div>
      </div>

      <button
        type="button"
        class="btn btn--ghost btn--sm toggle"
        role="switch"
        :aria-checked="store.srMode"
        @click="store.setSrMode(!store.srMode)"
      >
        <AppIcon name="reader" :size="16" />Screen reader mode
        <span class="pill" aria-hidden="true">{{ store.srMode ? 'On' : 'Off' }}</span>
      </button>

      <button
        type="button"
        class="btn btn--ghost btn--sm toggle"
        role="switch"
        :aria-checked="!store.muted"
        data-testid="chime-toggle"
        @click="store.setMuted(!store.muted)"
      >
        <AppIcon :name="store.muted ? 'bell-off' : 'bell'" :size="16" />Chime
        <span class="pill" aria-hidden="true">{{ store.muted ? 'Off' : 'On' }}</span>
      </button>

      <button
        type="button"
        class="btn btn--primary"
        data-testid="new-session"
        :disabled="!store.canLaunch"
        :title="newTitle"
        @click="store.openLaunch()"
      >
        <AppIcon name="plus" :size="16" />New session
      </button>
    </div>
  </header>
</template>

<style scoped>
.header {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 16px;
  height: 52px;
  padding: 0 12px 0 14px;
  border-bottom: 1px solid var(--rule);
  background: rgba(245, 243, 238, 0.92);
  /* backdrop-filter makes the header a stacking context, so the Keys popover's
     z-index only counts inside it; lift the whole header above the pane grid. */
  backdrop-filter: blur(6px);
  position: relative;
  z-index: 30;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.mark {
  display: grid;
  grid-template-columns: 7px 7px;
  gap: 2px;
}
.mark i {
  width: 7px;
  height: 7px;
  border-radius: 1.5px;
  background: var(--ink);
}
.mark i:last-child {
  background: var(--wait-rule);
}
.word {
  font-weight: 600;
  font-size: 15px;
  letter-spacing: -0.01em;
}
.sep {
  color: var(--rule-2);
}
.host {
  font-size: 14px;
  color: var(--ink-2);
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.conn {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-left: 2px;
}
.conn--on {
  background: var(--ok);
  box-shadow: 0 0 0 3px rgba(30, 122, 76, 0.15);
}
.conn--off {
  background: var(--err);
  box-shadow: 0 0 0 3px rgba(180, 35, 24, 0.15);
}
.summary {
  margin: 0;
  color: var(--ink-2);
  font-size: 13px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.summary-wait {
  color: var(--wait-fg);
  font-weight: 600;
}
.tools {
  display: flex;
  align-items: center;
  gap: 4px;
}
.toggle {
  color: var(--ink-2);
}
.pill {
  display: inline-block;
  min-width: 28px;
  padding: 0 6px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  line-height: 18px;
  text-align: center;
  background: var(--paper-2);
  color: var(--ink-2);
}
.toggle[aria-checked='true'] .pill {
  background: var(--ink);
  color: var(--paper);
}
.keys-wrap {
  position: relative;
}
.keys-pop {
  position: absolute;
  right: 0;
  top: 38px;
  z-index: 40;
  width: 320px;
  padding: 12px 14px;
  border: 1px solid var(--rule);
  border-radius: var(--radius-lg);
  background: var(--panel);
  box-shadow: 0 12px 32px -8px rgba(28, 27, 24, 0.25);
}
.keys-pop dl {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px 12px;
  margin: 0;
  font-size: 13px;
}
.keys-pop dt {
  white-space: nowrap;
}
.keys-pop dd {
  margin: 0;
  color: var(--ink-2);
}
kbd {
  display: inline-block;
  min-width: 18px;
  margin-right: 2px;
  padding: 0 4px;
  border: 1px solid var(--rule-2);
  border-bottom-width: 2px;
  border-radius: 4px;
  font-family: var(--mono);
  font-size: 11px;
  line-height: 16px;
  text-align: center;
  background: var(--paper);
}
</style>
