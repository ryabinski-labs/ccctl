import type { Repo } from './types';

export interface ReposResponse {
  repos: Repo[];
  prefix: string;
  missing?: boolean;
}

export type RepoListKind = 'loading' | 'unset' | 'timeout' | 'error' | 'missing' | 'empty' | 'list';

export interface RepoListState {
  kind: RepoListKind;
  message: string;
}

export interface RepoListInput {
  loading: boolean;
  error: { status: number } | null;
  resp: ReposResponse | null;
}

export const SCAN_TIMEOUT_MSG =
  'Scanning took too long. On a Mac, allow ccctl to access this folder in the prompt on the host, then retry.';

/** What the Folder list shows for a scan in flight, a failed scan, or a response. */
export function repoListState({ loading, error, resp }: RepoListInput): RepoListState {
  if (loading) return { kind: 'loading', message: 'Scanning folders…' };
  if (error) {
    if (error.status === 504) return { kind: 'timeout', message: SCAN_TIMEOUT_MSG };
    return { kind: 'error', message: 'The controller could not list repositories. Use a custom path or retry.' };
  }
  if (!resp || resp.prefix === '') return { kind: 'unset', message: 'No repository folder is set.' };
  if (resp.missing) return { kind: 'missing', message: `The repository folder ${resp.prefix} was not found. Change it in Settings.` };
  if (resp.repos.length === 0) {
    return { kind: 'empty', message: `No repositories found under ${resp.prefix}. Use a custom path or change the folder in Settings.` };
  }
  return { kind: 'list', message: '' };
}

/** Narrows repos by name or display path, ignoring case and outer spaces. */
export function filterRepos(repos: Repo[], query: string): Repo[] {
  const q = query.trim().toLowerCase();
  if (!q) return repos;
  return repos.filter((r) => r.display.toLowerCase().includes(q) || r.name.toLowerCase().includes(q));
}
