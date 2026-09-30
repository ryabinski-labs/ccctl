import { defineStore } from 'pinia';
import { markRaw } from 'vue';
import { chime } from '../lib/chime';
import { Connection, wsURL, type Hello } from '../lib/connection';
import { terminalBus } from '../lib/terminalBus';
import { ACTIVE_STATES, MAX_SESSIONS, STATE_LABEL, type SlotInfo } from '../lib/types';

function readPref(key: string): boolean {
  try {
    return localStorage.getItem(key) === '1';
  } catch {
    return false;
  }
}

function writePref(key: string, on: boolean) {
  try {
    localStorage.setItem(key, on ? '1' : '0');
  } catch {
    /* storage unavailable: preference lasts for this page only */
  }
}

export const useSessions = defineStore('sessions', {
  state: () => ({
    host: typeof location !== 'undefined' ? location.hostname : '',
    login: '',
    slots: [null, null, null, null] as (SlotInfo | null)[],
    /** true once the slot's current session produced output (hides the Starting overlay). */
    hasOutput: [false, false, false, false] as boolean[],
    connected: false,
    /** true after a connection attempt failed or dropped; drives the reconnect banner. */
    lost: false,
    /** Version from the first hello this page saw; a different one later means ccctl was upgraded. */
    serverVersion: '' as string,
    updated: false,
    everConnected: false,
    retryIn: 0,
    muted: readPref('ccctl.muted'),
    srMode: readPref('ccctl.sr'),
    maximized: null as number | null,
    focusedSlot: null as number | null,
    launchSlot: null as number | null,
    liveMessage: '',
    conn: null as Connection | null,
  }),

  getters: {
    activeCount: (s) => s.slots.filter((x) => x && ACTIVE_STATES.includes(x.state)).length,
    needsInputCount: (s) => s.slots.filter((x) => x?.state === 'needs-input').length,
    runningCount: (s) => s.slots.filter((x) => x?.state === 'running').length,
    canLaunch(): boolean {
      return this.activeCount < MAX_SESSIONS;
    },
    /** Lowest slot that is empty, exited, or resume-failed (spec DL-016). */
    firstFreeSlot: (s): number | null => {
      for (let i = 0; i < s.slots.length; i++) {
        const x = s.slots[i];
        if (!x || !ACTIVE_STATES.includes(x.state)) return i + 1;
      }
      return null;
    },
  },

  actions: {
    syncTitle() {
      const n = this.needsInputCount;
      const base = `ccctl · ${this.host}`;
      document.title = n > 0 ? `(${n}) ${base}` : base;
    },

    announce(msg: string) {
      // Re-set so repeated identical messages are still announced.
      this.liveMessage = '';
      queueMicrotask(() => (this.liveMessage = msg));
    },

    applyHello(h: Hello) {
      if (h.version) {
        if (this.serverVersion && h.version !== this.serverVersion) this.updated = true;
        else if (!this.serverVersion) this.serverVersion = h.version;
      }
      this.host = h.host || this.host;
      this.login = h.login;
      for (let i = 0; i < MAX_SESSIONS; i++) {
        const prev = this.slots[i];
        const next = h.slots?.[i] ?? null;
        if (!next || prev?.session_id !== next.session_id) this.hasOutput[i] = false;
        this.slots[i] = next;
      }
      this.syncTitle();
    },

    applySlot(slot: number, info: SlotInfo | null) {
      const i = slot - 1;
      if (i < 0 || i >= MAX_SESSIONS) return;
      const prev = this.slots[i];
      if (!info || prev?.session_id !== info.session_id) {
        this.hasOutput[i] = false;
        if (prev && (!info || prev.session_id !== info.session_id)) terminalBus.reset(slot);
      }
      this.slots[i] = info;
      if (info && info.state === 'needs-input' && prev?.state !== 'needs-input') {
        if (!this.muted) chime.play();
      }
      if (info && prev?.state !== info.state) {
        this.announce(`Slot ${slot} ${info.task}: ${STATE_LABEL[info.state].toLowerCase()}`);
      } else if (!info && prev) {
        this.announce(`Slot ${slot} is free`);
      }
      if (!info && this.maximized === slot) this.maximized = null;
      this.syncTitle();
    },

    markOutput(slot: number) {
      const i = slot - 1;
      if (i >= 0 && i < MAX_SESSIONS && !this.hasOutput[i]) this.hasOutput[i] = true;
    },

    setMuted(on: boolean) {
      this.muted = on;
      writePref('ccctl.muted', on);
      if (!on) chime.unlock();
    },

    setSrMode(on: boolean) {
      this.srMode = on;
      writePref('ccctl.sr', on);
    },

    toggleMaximize(slot: number) {
      this.maximized = this.maximized === slot ? null : slot;
    },

    openLaunch(slot?: number | null) {
      if (!this.canLaunch) return;
      this.launchSlot = slot ?? this.firstFreeSlot;
    },

    closeLaunch() {
      this.launchSlot = null;
    },

    connect() {
      if (this.conn) return;
      const conn = new Connection(wsURL(), {
        onHello: (h) => this.applyHello(h),
        onSlot: (slot, info) => this.applySlot(slot, info),
        onOutput: (slot, data, replay) => {
          if (replay) terminalBus.reset(slot);
          terminalBus.write(slot, data);
          if (data.length) this.markOutput(slot);
        },
        onStatus: (connected, retryIn) => {
          if (connected && !this.connected) this.announce(`Connected to ${this.host}`);
          this.connected = connected;
          this.lost = !connected;
          if (connected) this.everConnected = true;
          this.retryIn = retryIn;
        },
      });
      this.conn = markRaw(conn);
      conn.connect();
    },

    sendInput(slot: number, data: Uint8Array) {
      this.conn?.sendInput(slot, data);
    },

    resize(slot: number, rows: number, cols: number) {
      this.conn?.resize(slot, rows, cols);
    },
  },
});
