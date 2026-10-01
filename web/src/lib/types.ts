export type SessionState = 'starting' | 'running' | 'needs-input' | 'exited' | 'resume-failed';

export interface SlotInfo {
  slot: number;
  task: string;
  cwd: string;
  repo: string;
  worktree: string;
  branch: string;
  session_id: string;
  state: SessionState;
  launched_at: string;
  resumed: boolean;
  exit_code: number | null;
  message: string;
}

export interface Repo {
  name: string;
  path: string;
  display: string;
}

export interface Inspection {
  path: string;
  git: boolean;
  worktree_allowed: boolean;
  note: string;
}

export interface LaunchRequest {
  task: string;
  path: string;
  worktree: boolean;
  prompt: string;
  slot?: number;
}

export const MAX_SESSIONS = 6;
export const ACTIVE_STATES: SessionState[] = ['starting', 'running', 'needs-input'];

export const TEXT = {
  allSlotsInUse: `All ${MAX_SESSIONS} slots are in use. Stop or close a session first.`,
  notGit: 'Not a git repository: session runs in the folder itself.',
  taskRule: 'Task name must be 1 to 40 characters of a-z, 0-9 or -, and unique among open sessions.',
  stopConfirm: (task: string) => `Stop ${task}? The worktree and branch are kept.`,
  startingIn: (path: string) => `Starting claude in ${path}`,
  free: (slot: number) => `Slot ${slot} is free`,
} as const;

export const STATE_LABEL: Record<SessionState, string> = {
  starting: 'Starting',
  running: 'Running',
  'needs-input': 'Needs input',
  exited: 'Exited',
  'resume-failed': 'Resume failed',
};
