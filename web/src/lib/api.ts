import type { Inspection, LaunchRequest, SlotInfo, Transcript } from './types';
import type { ReposResponse } from './repoList';

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    data = null;
  }
  if (!res.ok) {
    const msg = (data as { error?: string } | null)?.error ?? `Request failed (HTTP ${res.status}).`;
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

/** Sends one file to the host; resolves to its absolute path there. */
async function upload(slot: number, file: File): Promise<string> {
  const res = await fetch(`/api/sessions/${slot}/upload?name=${encodeURIComponent(file.name || 'file')}`, {
    method: 'POST',
    body: file,
  });
  let data: { path?: string; error?: string } | null = null;
  try {
    data = await res.json();
  } catch {
    data = null;
  }
  if (!res.ok || !data?.path) throw new ApiError(data?.error ?? `Upload failed (HTTP ${res.status}).`, res.status);
  return data.path;
}

export const api = {
  upload,
  repos: () => call<ReposResponse>('GET', '/api/repos').then((r) => ({ ...r, repos: r.repos ?? [] })),
  /** The one folder New session scans for repos; empty when unset. */
  repoPrefix: () => call<{ prefix: string }>('GET', '/api/settings/repo-prefix').then((r) => r.prefix),
  repoPrefixSet: (prefix: string) => call<{ prefix: string }>('PUT', '/api/settings/repo-prefix', { prefix }).then((r) => r.prefix),
  repoPrefixClear: () => call<{ prefix: string }>('DELETE', '/api/settings/repo-prefix').then((r) => r.prefix),
  inspect: (path: string) => call<Inspection>('GET', `/api/inspect?path=${encodeURIComponent(path)}`),
  transcript: (id: string) => call<Transcript>('GET', `/api/transcript?id=${encodeURIComponent(id)}`),
  launch: (req: LaunchRequest) => call<{ slot: number; info: SlotInfo }>('POST', '/api/sessions', req),
  stop: (slot: number) => call<object>('POST', `/api/sessions/${slot}/stop`),
  fresh: (slot: number) => call<{ slot: number; info: SlotInfo }>('POST', `/api/sessions/${slot}/fresh`),
  close: (slot: number) => call<object>('POST', `/api/sessions/${slot}/close`),
  /** Session environment ([env] in config.toml). Names only; values are write-only. */
  envNames: () => call<{ names: string[] }>('GET', '/api/settings/env').then((r) => r.names ?? []),
  envSet: (name: string, value: string) =>
    call<{ names: string[] }>('PUT', `/api/settings/env/${encodeURIComponent(name)}`, { value }).then((r) => r.names ?? []),
  envRemove: (name: string) =>
    call<{ names: string[] }>('DELETE', `/api/settings/env/${encodeURIComponent(name)}`).then((r) => r.names ?? []),
};
