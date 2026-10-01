<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';
import { terminalBus, type TerminalSink } from '../lib/terminalBus';
import { useSessions } from '../stores/sessions';
import { copyText, parseOsc52 } from '../lib/clipboard';
import { filesOf, uploadForPaste } from '../lib/attach';
import { ApiError } from '../lib/api';

const props = defineProps<{ slot: number }>();
const emit = defineEmits<{
  focus: [];
  blur: [];
  shortcut: [e: KeyboardEvent];
  /** Upload progress: busy while files are going up; error is set when one fails. */
  attach: [state: { busy: boolean; error: string }];
}>();

const store = useSessions();
const host = ref<HTMLElement | null>(null);
const encoder = new TextEncoder();

let term: Terminal | null = null;
let fit: FitAddon | null = null;
let ro: ResizeObserver | null = null;
let sink: TerminalSink | null = null;
let resizeTimer: ReturnType<typeof setTimeout> | null = null;
let lastSize = '';

const THEME = {
  background: '#121317',
  foreground: '#e7e5df',
  cursor: '#f2b544',
  cursorAccent: '#121317',
  selectionBackground: 'rgba(242, 181, 68, 0.28)',
  black: '#1b1d23',
  red: '#ef6b5f',
  green: '#7fcf8f',
  yellow: '#f2c566',
  blue: '#79a8ff',
  magenta: '#d197f0',
  cyan: '#6fd0d6',
  white: '#d9d6cf',
  brightBlack: '#6c6f78',
  brightRed: '#ff8a7e',
  brightGreen: '#9be3a8',
  brightYellow: '#ffd98a',
  brightBlue: '#9cc0ff',
  brightMagenta: '#e3b5ff',
  brightCyan: '#95e6ea',
  brightWhite: '#ffffff',
};

function sendSize() {
  if (!term) return;
  const key = `${term.rows}x${term.cols}`;
  if (key === lastSize) return;
  lastSize = key;
  store.resize(props.slot, term.rows, term.cols);
}

function refit() {
  if (!fit || !host.value || host.value.clientWidth === 0 || host.value.clientHeight === 0) return;
  try {
    fit.fit();
  } catch {
    return;
  }
  if (resizeTimer) clearTimeout(resizeTimer);
  resizeTimer = setTimeout(sendSize, 80);
}

function bufferText(): string {
  if (!term) return '';
  const buf = term.buffer.active;
  let out = '';
  for (let i = 0; i < buf.length; i++) {
    const line = buf.getLine(i);
    if (!line) continue;
    if (i > 0 && !line.isWrapped) out += '\n';
    out += line.translateToString(true);
  }
  return out;
}

onMounted(async () => {
  try {
    await document.fonts?.load?.('13px "IBM Plex Mono"');
  } catch {
    /* fall back to metrics of the fallback font */
  }
  if (!host.value) return;
  term = new Terminal({
    fontFamily: '"IBM Plex Mono", ui-monospace, Menlo, monospace',
    fontSize: 13,
    lineHeight: 1.12,
    cursorBlink: true,
    allowProposedApi: true,
    scrollback: 5000,
    screenReaderMode: store.srMode,
    macOptionIsMeta: false,
    macOptionClickForcesSelection: true,
    theme: THEME,
  });
  fit = new FitAddon();
  term.loadAddon(fit);
  const uni = new Unicode11Addon();
  term.loadAddon(uni);
  term.unicode.activeVersion = '11';
  term.open(host.value);
  try {
    const gl = new WebglAddon();
    gl.onContextLoss(() => gl.dispose());
    term.loadAddon(gl);
  } catch {
    /* DOM renderer fallback */
  }

  term.attachCustomKeyEventHandler((e) => {
    if (e.type === 'keydown' && e.ctrlKey && e.shiftKey && /^(Digit[0-6]|KeyM)$/.test(e.code)) {
      e.preventDefault();
      emit('shortcut', e);
      return false;
    }
    return true;
  });
  // Claude Code copies its selections with OSC 52. If the browser refuses the
  // write (it wants a user gesture), retry on the next click or keypress.
  let pendingCopy: string | null = null;
  term.parser.registerOscHandler(52, (data) => {
    const text = parseOsc52(data);
    if (text === null) return true;
    void copyText(text).then((ok) => {
      pendingCopy = ok ? null : text;
    });
    return true;
  });
  const flushCopy = () => {
    const text = pendingCopy;
    if (text === null) return;
    pendingCopy = null;
    void copyText(text).then((ok) => {
      if (!ok) pendingCopy = text;
    });
  };
  host.value.addEventListener('pointerup', flushCopy);
  host.value.addEventListener('keydown', flushCopy);
  // Files pasted or dropped on the pane go to the host, and their paths are
  // typed into the session (Claude Code attaches an image given by path).
  // Plain-text paste is left to xterm.
  async function attach(files: File[]) {
    if (!term || files.length === 0) return;
    emit('attach', { busy: true, error: '' });
    try {
      const text = await uploadForPaste(props.slot, files);
      term?.paste(text);
      emit('attach', { busy: false, error: '' });
    } catch (e) {
      emit('attach', { busy: false, error: e instanceof ApiError ? e.message : 'The upload did not reach the controller. Try again.' });
    }
  }
  host.value.addEventListener(
    'paste',
    (e) => {
      const files = filesOf(e.clipboardData);
      if (files.length === 0) return;
      e.preventDefault();
      e.stopPropagation();
      void attach(files);
    },
    true,
  );
  host.value.addEventListener('dragover', (e) => {
    if (e.dataTransfer?.types.includes('Files')) e.preventDefault();
  });
  host.value.addEventListener('drop', (e) => {
    const files = filesOf(e.dataTransfer);
    if (files.length === 0) return;
    e.preventDefault();
    void attach(files);
    term?.focus();
  });
  term.onData((d) => store.sendInput(props.slot, encoder.encode(d)));
  term.onBinary((d) => {
    const bytes = new Uint8Array(d.length);
    for (let i = 0; i < d.length; i++) bytes[i] = d.charCodeAt(i) & 0xff;
    store.sendInput(props.slot, bytes);
  });
  term.textarea?.addEventListener('focus', () => emit('focus'));
  term.textarea?.addEventListener('blur', () => emit('blur'));

  sink = {
    write: (data) => term?.write(data),
    reset: () => term?.reset(),
    focus: () => term?.focus(),
    text: bufferText,
    attach: (files) => void attach(files),
  };
  terminalBus.register(props.slot, sink);

  ro = new ResizeObserver(() => refit());
  ro.observe(host.value);
  refit();
});

watch(
  () => store.srMode,
  (on) => {
    if (term) term.options.screenReaderMode = on;
  },
);

onBeforeUnmount(() => {
  ro?.disconnect();
  if (resizeTimer) clearTimeout(resizeTimer);
  if (sink) terminalBus.unregister(props.slot, sink);
  term?.dispose();
  term = null;
});
</script>

<template>
  <div ref="host" class="term" :aria-label="`Terminal for slot ${slot}`" role="group" />
</template>

<style scoped>
.term {
  position: absolute;
  inset: 6px 4px 4px 8px;
}
.term :deep(.xterm) {
  height: 100%;
}
.term :deep(.xterm-viewport) {
  background-color: transparent !important;
  scrollbar-width: thin;
  scrollbar-color: #3a3d46 transparent;
}
</style>
