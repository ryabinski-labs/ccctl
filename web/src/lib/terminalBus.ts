/**
 * Non-reactive routing between the WebSocket and xterm instances. Output that
 * arrives before a pane's terminal mounts is queued and flushed on register.
 */
export interface TerminalSink {
  write(data: Uint8Array): void;
  reset(): void;
  focus(): void;
  text(): string;
  /** Uploads files to the host and pastes their paths into the session. */
  attach(files: File[]): void;
}

const sinks = new Map<number, TerminalSink>();
const pending = new Map<number, Uint8Array[]>();

export const terminalBus = {
  register(slot: number, sink: TerminalSink) {
    sinks.set(slot, sink);
    for (const chunk of pending.get(slot) ?? []) sink.write(chunk);
    pending.delete(slot);
  },
  unregister(slot: number, sink: TerminalSink) {
    if (sinks.get(slot) === sink) sinks.delete(slot);
  },
  write(slot: number, data: Uint8Array) {
    const s = sinks.get(slot);
    if (s) s.write(data);
    else {
      const q = pending.get(slot) ?? [];
      q.push(data.slice());
      pending.set(slot, q);
    }
  },
  reset(slot: number) {
    pending.delete(slot);
    sinks.get(slot)?.reset();
  },
  focus(slot: number): boolean {
    const s = sinks.get(slot);
    s?.focus();
    return !!s;
  },
  text(slot: number): string {
    return sinks.get(slot)?.text() ?? '';
  },
  attach(slot: number, files: File[]) {
    sinks.get(slot)?.attach(files);
  },
};

declare global {
  interface Window {
    __ccctl?: { text(slot: number): string };
  }
}

if (typeof window !== 'undefined') {
  // Read-only hook used by the headless acceptance tests to read a pane's buffer.
  window.__ccctl = { text: (slot: number) => terminalBus.text(slot) };
}
