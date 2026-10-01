<script setup lang="ts">
import { computed, ref } from 'vue';
import { terminalBus } from '../lib/terminalBus';
import { api, ApiError } from '../lib/api';
import { handleShortcut } from '../lib/shortcuts';
import { TEXT } from '../lib/types';
import { useSessions } from '../stores/sessions';
import AppIcon from './AppIcon.vue';
import StateBadge from './StateBadge.vue';
import StopDialog from './StopDialog.vue';
import TerminalView from './TerminalView.vue';

const props = defineProps<{ slot: number }>();
const store = useSessions();

const info = computed(() => store.slots[props.slot - 1]);
const maximized = computed(() => store.maximized === props.slot);
const focused = computed(() => store.focusedSlot === props.slot);
const showStarting = computed(() => info.value?.state === 'starting' && !store.hasOutput[props.slot - 1]);
const live = computed(() => !!info.value && ['starting', 'running', 'needs-input'].includes(info.value.state));

const stopOpen = ref(false);
const busy = ref(false);
const error = ref('');

async function run(fn: () => Promise<unknown>) {
  busy.value = true;
  error.value = '';
  try {
    await fn();
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : 'The controller did not respond. Try again.';
  } finally {
    busy.value = false;
  }
}

async function confirmStop() {
  stopOpen.value = false;
  await run(() => api.stop(props.slot));
}

async function startFresh() {
  await run(async () => {
    const r = await api.fresh(props.slot);
    if (r?.info) store.applySlot(props.slot, r.info);
  });
}

async function closePane() {
  await run(async () => {
    await api.close(props.slot);
    store.applySlot(props.slot, null);
  });
}

const fileInput = ref<HTMLInputElement | null>(null);
const uploading = ref(false);
let errorTimer: ReturnType<typeof setTimeout> | null = null;

function onAttach(s: { busy: boolean; error: string }) {
  uploading.value = s.busy;
  if (errorTimer) clearTimeout(errorTimer);
  if (s.error) {
    error.value = s.error;
    errorTimer = setTimeout(() => (error.value = ''), 8000);
  }
}

function pickFiles() {
  const input = fileInput.value;
  if (!input) return;
  terminalBus.attach(props.slot, Array.from(input.files ?? []));
  input.value = '';
}

function onFocus() {
  store.focusedSlot = props.slot;
}
function onBlur() {
  if (store.focusedSlot === props.slot) store.focusedSlot = null;
}
</script>

<template>
  <section
    class="pane"
    :class="{
      'pane--free': !info,
      'pane--wait': info?.state === 'needs-input',
      'pane--focused': focused,
      'pane--dead': info && !live,
    }"
    :data-slot="slot"
    :data-state="info?.state ?? 'free'"
    :aria-label="info ? `Slot ${slot}: ${info.task}` : TEXT.free(slot)"
  >
    <template v-if="info">
      <header class="phead" data-testid="pane-header">
        <span class="chip mono" aria-hidden="true">{{ slot }}</span>
        <span class="task mono" :title="info.task">{{ info.task }}</span>
        <span class="meta mono" :title="info.cwd">
          <span>{{ info.repo }}</span
          ><template v-if="info.branch"><span class="dim"> · </span>{{ info.branch }}</template>
        </span>
        <span v-if="focused && live" class="typing">typing here</span>
        <StateBadge :state="info.state" />
        <div class="pactions">
          <template v-if="live">
            <input
              ref="fileInput"
              type="file"
              multiple
              hidden
              :aria-label="`Choose files to send to slot ${slot}`"
              data-testid="attach-input"
              @change="pickFiles"
            />
            <button
              type="button"
              class="icon-btn"
              :aria-label="`Send a file to ${info.task}`"
              title="Send a file or image (or paste or drop one on the terminal)"
              data-testid="attach"
              :disabled="uploading"
              @click="fileInput?.click()"
            >
              <AppIcon name="attach" :size="16" />
            </button>
          </template>
          <button
            type="button"
            class="icon-btn"
            :aria-label="maximized ? `Restore slot ${slot}` : `Maximize slot ${slot}`"
            :title="maximized ? 'Restore' : 'Maximize'"
            data-testid="maximize"
            @click="store.toggleMaximize(slot)"
          >
            <AppIcon :name="maximized ? 'restore' : 'maximize'" :size="16" />
          </button>
          <button
            v-if="live"
            type="button"
            class="icon-btn"
            :aria-label="`Stop ${info.task}`"
            title="Stop"
            data-testid="stop"
            @click="stopOpen = true"
          >
            <AppIcon name="stop" :size="16" />
          </button>
          <button
            v-else
            type="button"
            class="icon-btn"
            :aria-label="`Close slot ${slot}`"
            title="Close pane"
            @click="closePane"
          >
            <AppIcon name="close" :size="16" />
          </button>
        </div>
      </header>

      <div class="well">
        <TerminalView :slot="slot" @focus="onFocus" @blur="onBlur" @shortcut="handleShortcut" @attach="onAttach" />

        <div v-if="showStarting" class="starting" role="status">
          <span class="spinner" aria-hidden="true" />
          <span class="mono">{{ TEXT.startingIn(info.cwd) }}</span>
        </div>

        <div v-if="info.state === 'resume-failed'" class="failed" role="alert">
          <p class="failed-msg">{{ info.message || `Could not resume ${info.task}.` }}</p>
          <div class="failed-actions">
            <button type="button" class="btn btn--primary btn--sm" :disabled="busy" @click="startFresh">
              <AppIcon name="refresh" :size="14" />Start fresh
            </button>
            <button type="button" class="btn btn--sm" :disabled="busy" @click="closePane">Close</button>
          </div>
        </div>

        <div v-if="info.state === 'exited'" class="exited" data-testid="exited-bar">
          <span class="mono" :title="info.message">{{ info.message }}</span>
          <button type="button" class="btn btn--sm" :disabled="busy" @click="closePane">Close pane</button>
        </div>

        <p v-if="uploading" class="pnote mono" role="status">Sending file…</p>
        <p v-if="error" class="perror" role="alert">{{ error }}</p>
      </div>

      <StopDialog
        v-if="stopOpen"
        :task="info.task"
        :slot="slot"
        :busy="busy"
        @cancel="stopOpen = false"
        @confirm="confirmStop"
      />
    </template>

    <div v-else class="free">
      <span class="free-num mono" aria-hidden="true">{{ slot }}</span>
      <p class="free-text">{{ TEXT.free(slot) }}</p>
      <button type="button" class="btn" :disabled="!store.canLaunch" @click="store.openLaunch(slot)">
        <AppIcon name="plus" :size="16" />Start a session
      </button>
    </div>
  </section>
</template>

<style scoped>
.pane {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  border-radius: var(--radius-lg);
  background: var(--well);
  border: 1px solid #0b0c0f;
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.6),
    0 10px 24px -14px rgba(28, 27, 24, 0.45);
  overflow: hidden;
  transition: box-shadow 140ms ease;
}
.pane--wait {
  border-color: var(--wait-rule);
  box-shadow:
    0 0 0 1px var(--wait-rule),
    0 10px 28px -12px rgba(217, 138, 0, 0.45);
}
.pane--focused {
  box-shadow:
    0 0 0 2px var(--paper),
    0 0 0 4px var(--accent),
    0 10px 24px -14px rgba(28, 27, 24, 0.45);
}
.phead {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 36px;
  flex: none;
  padding: 0 6px 0 8px;
  background: var(--panel);
  border-bottom: 1px solid var(--rule);
  color: var(--ink);
}
.pane--wait .phead {
  background: var(--wait-bg);
  color: var(--wait-fg);
  border-top: 3px solid var(--wait-rule);
  border-bottom-color: var(--wait-rule);
  height: 39px;
}
.chip {
  display: inline-grid;
  place-items: center;
  width: 20px;
  height: 20px;
  flex: none;
  border-radius: 4px;
  background: var(--ink);
  color: var(--paper);
  font-size: 12px;
  font-weight: 600;
}
.pane--wait .chip {
  background: var(--wait-fg);
}
.task {
  flex: none;
  max-width: 34%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
  font-size: 13px;
}
.meta {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--ink-2);
}
.pane--wait .meta {
  color: var(--wait-fg);
}
.dim {
  opacity: 0.6;
}
.typing {
  flex: none;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--accent);
}
.pactions {
  display: flex;
  gap: 2px;
  flex: none;
  color: var(--ink-2);
}
.well {
  position: relative;
  flex: 1 1 auto;
  min-height: 0;
  background: var(--well);
}
.pane--dead .well :deep(.term) {
  opacity: 0.72;
}
.starting {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 16px;
  background: var(--well);
  color: var(--well-fg);
  font-size: 13px;
}
.spinner {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid rgba(231, 229, 223, 0.25);
  border-top-color: #f2b544;
  animation: spin 0.8s linear infinite;
}
.failed {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  width: min(420px, calc(100% - 32px));
  padding: 18px;
  border-radius: var(--radius-lg);
  background: var(--panel);
  border: 1px solid #efb8b2;
  border-top: 3px solid var(--err);
  box-shadow: 0 16px 40px -12px rgba(0, 0, 0, 0.55);
  color: var(--ink);
}
.failed-msg {
  margin: 0 0 14px;
  font-weight: 600;
}
.failed-actions {
  display: flex;
  gap: 8px;
}
.exited {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 8px 8px 12px;
  background: var(--panel);
  border-top: 1px solid var(--rule);
  font-size: 12px;
  color: var(--ink);
}
.exited .mono {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pnote {
  position: absolute;
  right: 8px;
  bottom: 8px;
  margin: 0;
  padding: 4px 8px;
  border-radius: var(--radius);
  background: var(--panel);
  color: var(--ink-2);
  font-size: 12px;
}
.perror {
  position: absolute;
  left: 8px;
  right: 8px;
  top: 8px;
  margin: 0;
  padding: 8px 10px;
  border-radius: var(--radius);
  background: var(--err-soft);
  color: #7a1a12;
  font-size: 12px;
}

/* Free slot: a light dashed card — reads as "empty" at a glance next to dark wells. */
.pane--free {
  background: rgba(255, 255, 255, 0.55);
  border: 1.5px dashed var(--rule-2);
  box-shadow: none;
  align-items: center;
  justify-content: center;
}
.free {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}
.free-num {
  font-size: 44px;
  font-weight: 500;
  line-height: 1;
  color: var(--rule-2);
}
.free-text {
  margin: 0 0 4px;
  color: var(--ink-2);
  font-weight: 500;
}
</style>
