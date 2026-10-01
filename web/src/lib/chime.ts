/** Soft two-tone chime synthesized with WebAudio (no asset). Replaceable in tests. */
let ctx: AudioContext | null = null;

function context(): AudioContext | null {
  if (typeof window === 'undefined' || typeof window.AudioContext === 'undefined') return null;
  ctx ??= new window.AudioContext();
  return ctx;
}

function tone(ac: AudioContext, freq: number, start: number, dur: number) {
  const osc = ac.createOscillator();
  const gain = ac.createGain();
  osc.type = 'sine';
  osc.frequency.value = freq;
  gain.gain.setValueAtTime(0, start);
  gain.gain.linearRampToValueAtTime(0.18, start + 0.02);
  gain.gain.exponentialRampToValueAtTime(0.0001, start + dur);
  osc.connect(gain).connect(ac.destination);
  osc.start(start);
  osc.stop(start + dur + 0.05);
}

export const chime = {
  play(): void {
    const ac = context();
    if (!ac) return;
    void ac.resume?.();
    const t = ac.currentTime + 0.01;
    tone(ac, 880, t, 0.35);
    tone(ac, 1318.5, t + 0.14, 0.5);
  },
  /** Browsers need a user gesture before audio can start; call from any click. */
  unlock(): void {
    const ac = context();
    void ac?.resume?.();
  },
};
