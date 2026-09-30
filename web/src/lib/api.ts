import type { Inspection, LaunchRequest, Repo, SlotInfo } from './types';

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

export const api = {
  repos: () => call<{ repos: Repo[] }>('GET', '/api/repos').then((r) => r.repos ?? []),
  inspect: (path: string) => call<Inspection>('GET', `/api/inspect?path=${encodeURIComponent(path)}`),
  launch: (req: LaunchRequest) => call<{ slot: number; info: SlotInfo }>('POST', '/api/sessions', req),
  stop: (slot: number) => call<object>('POST', `/api/sessions/${slot}/stop`),
  fresh: (slot: number) => call<{ slot: number; info: SlotInfo }>('POST', `/api/sessions/${slot}/fresh`),
  close: (slot: number) => call<object>('POST', `/api/sessions/${slot}/close`),
};
