/**
 * Clipboard support for OSC 52 ("set clipboard") sequences. Claude Code copies
 * its own mouse selections this way. The UI is served over plain HTTP on a
 * Tailscale IP, which is not a secure context, so `navigator.clipboard` is
 * missing and we fall back to `execCommand('copy')`.
 */

/** Decode the payload of `OSC 52 ; <targets> ; <base64>`. Returns null for queries and bad data. */
export function parseOsc52(data: string): string | null {
  const sep = data.indexOf(';');
  if (sep < 0) return null;
  const b64 = data.slice(sep + 1);
  if (b64 === '' || b64 === '?') return null;
  try {
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return new TextDecoder().decode(bytes);
  } catch {
    return null;
  }
}

function execCopy(text: string): boolean {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
  const prev = document.activeElement as HTMLElement | null;
  document.body.appendChild(ta);
  ta.select();
  let ok = false;
  try {
    ok = document.execCommand('copy');
  } catch {
    ok = false;
  }
  ta.remove();
  prev?.focus?.();
  return ok;
}

/** Copy text to the system clipboard. Resolves false if the browser refused. */
export async function copyText(text: string): Promise<boolean> {
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      /* fall through to execCommand */
    }
  }
  return execCopy(text);
}
