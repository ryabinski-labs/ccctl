export const FRAME_OUTPUT = 0x01;
export const FRAME_REPLAY = 0x02;
export const FRAME_INPUT = 0x01;

export function encodeInput(slot: number, data: Uint8Array): Uint8Array {
  const out = new Uint8Array(data.length + 2);
  out[0] = FRAME_INPUT;
  out[1] = slot;
  out.set(data, 2);
  return out;
}

export function decodeFrame(buf: ArrayBuffer): { type: number; slot: number; data: Uint8Array } | null {
  const bytes = new Uint8Array(buf);
  if (bytes.length < 2) return null;
  return { type: bytes[0], slot: bytes[1], data: bytes.subarray(2) };
}
