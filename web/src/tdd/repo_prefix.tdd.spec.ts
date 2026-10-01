// TDD scenarios from tdd/repo-prefix.tdd.yaml — scenario IDs stay in the test names so QA maps
// results back by ID.

import { describe, expect, it } from 'vitest';
import { filterRepos, repoListState } from '../lib/repoList';
import type { Repo } from '../lib/types';

const TIMEOUT_MSG = 'Scanning took too long. On a Mac, allow ccctl to access this folder in the prompt on the host, then retry.';

describe('REQ-pfx-noscan — With no prefix set, the list says no folder is set', () => {
  it('SC-pfx-noscan-d The list state is unset when the server reports an empty prefix', () => {
    expect(repoListState({ loading: false, error: null, resp: { repos: [], prefix: '' } })).toEqual({
      kind: 'unset',
      message: 'No repository folder is set.',
    });
  });
});

describe('REQ-pfx-timeout — A slow scan ends with an explanation', () => {
  it('SC-pfx-timeout-c A 504 maps to the timeout state and loading maps to Scanning', () => {
    expect(repoListState({ loading: false, error: { status: 504 }, resp: null })).toEqual({ kind: 'timeout', message: TIMEOUT_MSG });
    expect(repoListState({ loading: true, error: null, resp: null })).toEqual({ kind: 'loading', message: 'Scanning folders…' });
  });
});

describe('REQ-pfx-empty — An empty or missing prefix folder says which', () => {
  it('SC-pfx-empty-b The list state words the empty and missing cases', () => {
    const prefix = '~/Documents/projects';
    expect(repoListState({ loading: false, error: null, resp: { repos: [], prefix } })).toEqual({
      kind: 'empty',
      message: 'No repositories found under ~/Documents/projects. Use a custom path or change the folder in Settings.',
    });
    expect(repoListState({ loading: false, error: null, resp: { repos: [], prefix, missing: true } })).toEqual({
      kind: 'missing',
      message: 'The repository folder ~/Documents/projects was not found. Change it in Settings.',
    });
  });
});

describe('REQ-pfx-search — The search box narrows the list', () => {
  it('SC-pfx-search-a Filtering a 12-repo list', () => {
    const names = ['folio', 'folder-x', 'alpha', 'beta', 'gamma', 'delta', 'eps', 'zeta', 'eta', 'theta', 'iota', 'kappa'];
    const repos: Repo[] = names.map((n) => ({
      name: n,
      path: `/h/${n}`,
      display: n === 'kappa' ? '~/fol/kappa' : `~/${n}`,
    }));
    expect(filterRepos(repos, 'fol')).toHaveLength(3);
    expect(filterRepos(repos, 'FOL')).toHaveLength(3);
    expect(filterRepos(repos, '  ')).toHaveLength(12);
  });
});
