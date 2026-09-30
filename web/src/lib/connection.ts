import { retryDelay } from './backoff';
import { decodeFrame, encodeInput, FRAME_OUTPUT, FRAME_REPLAY } from './frames';
import type { SlotInfo } from './types';

export interface Hello {
  type: 'hello';
  host: string;
  login: string;
  slots: (SlotInfo | null)[];
}

export interface ConnectionHandlers {
  onHello(h: Hello): void;
  onSlot(slot: number, info: SlotInfo | null): void;
  onOutput(slot: number, data: Uint8Array, replay: boolean): void;
  /** connected=false with retryIn = seconds until the next attempt (counts down). */
  onStatus(connected: boolean, retryIn: number): void;
  /** Called once per scheduled retry with its delay in seconds. */
  onRetryScheduled?(delay: number): void;
}

type WSCtor = new (url: string) => WebSocket;

export class Connection {
  private ws: WebSocket | null = null;
  private attempt = 0;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private tickTimer: ReturnType<typeof setInterval> | null = null;
  private stopped = false;

  constructor(
    private readonly url: string,
    private readonly h: ConnectionHandlers,
    private readonly WS: WSCtor = WebSocket,
  ) {}

  connect(): void {
    this.stopped = false;
    this.clearTimers();
    let ws: WebSocket;
    try {
      ws = new this.WS(this.url);
    } catch {
      this.scheduleRetry();
      return;
    }
    ws.binaryType = 'arraybuffer';
    this.ws = ws;
    let opened = false;
    ws.onopen = () => {
      opened = true;
      this.attempt = 0;
      this.h.onStatus(true, 0);
    };
    ws.onmessage = (ev: MessageEvent) => this.handle(ev.data);
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      if (!this.stopped) this.scheduleRetry();
      void opened;
    };
    ws.onerror = () => {
      /* close follows */
    };
  }

  close(): void {
    this.stopped = true;
    this.clearTimers();
    this.ws?.close();
    this.ws = null;
  }

  sendInput(slot: number, data: Uint8Array): void {
    if (this.ws?.readyState === 1) this.ws.send(encodeInput(slot, data));
  }

  resize(slot: number, rows: number, cols: number): void {
    if (this.ws?.readyState === 1) this.ws.send(JSON.stringify({ type: 'resize', slot, rows, cols }));
  }

  private handle(data: unknown) {
    if (typeof data === 'string') {
      let msg: { type?: string; slot?: number; info?: SlotInfo | null };
      try {
        msg = JSON.parse(data);
      } catch {
        return;
      }
      if (msg.type === 'hello') this.h.onHello(msg as unknown as Hello);
      else if (msg.type === 'slot' && typeof msg.slot === 'number') this.h.onSlot(msg.slot, msg.info ?? null);
      return;
    }
    if (data instanceof ArrayBuffer) {
      const f = decodeFrame(data);
      if (!f) return;
      if (f.type === FRAME_OUTPUT) this.h.onOutput(f.slot, f.data, false);
      else if (f.type === FRAME_REPLAY) this.h.onOutput(f.slot, f.data, true);
    }
  }

  private scheduleRetry() {
    this.clearTimers();
    const delay = retryDelay(this.attempt);
    this.attempt += 1;
    this.h.onRetryScheduled?.(delay);
    let left = delay;
    this.h.onStatus(false, left);
    this.tickTimer = setInterval(() => {
      left = Math.max(left - 1, 0);
      this.h.onStatus(false, left);
    }, 1000);
    this.retryTimer = setTimeout(() => {
      this.clearTimers();
      this.connect();
    }, delay * 1000);
  }

  private clearTimers() {
    if (this.retryTimer) clearTimeout(this.retryTimer);
    if (this.tickTimer) clearInterval(this.tickTimer);
    this.retryTimer = null;
    this.tickTimer = null;
  }
}

export function wsURL(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${location.host}/ws`;
}
