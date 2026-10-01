import { api } from './api';

/** Backslash-escapes a path the way a macOS terminal does when a file is dropped on it. */
export function termPath(p: string): string {
  return p.replace(/[^A-Za-z0-9_\-./~@%+=:,]/g, '\\$&');
}

/** Files carried by a paste or drop event; empty when it is plain text. */
export function filesOf(dt: DataTransfer | null): File[] {
  return dt ? Array.from(dt.files ?? []) : [];
}

/**
 * Uploads each file to the host and returns the text to paste into the
 * session: the escaped paths, each followed by a space.
 */
export async function uploadForPaste(slot: number, files: File[]): Promise<string> {
  let out = '';
  for (const f of files) out += termPath(await api.upload(slot, f)) + ' ';
  return out;
}
