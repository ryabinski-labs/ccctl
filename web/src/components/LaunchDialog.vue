<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { api, ApiError } from '../lib/api';
import { filterRepos, repoListState, type ReposResponse } from '../lib/repoList';
import { TEXT, type Inspection, type Repo } from '../lib/types';
import { useSessions } from '../stores/sessions';
import AppIcon from './AppIcon.vue';
import ModalShell from './ModalShell.vue';

const props = defineProps<{ slot: number }>();
const emit = defineEmits<{ close: [] }>();
const store = useSessions();

const PROMPT_MAX = 10000;
const LAST_KEY = 'ccctl.lastFolder';

const task = ref('');
const taskTouched = ref(false);
const repos = ref<Repo[]>([]);
const reposLoading = ref(true);
const reposResp = ref<ReposResponse | null>(null);
const scanError = ref<{ status: number } | null>(null);
const query = ref('');
const selected = ref<string>('');
const useCustom = ref(false);
const customPath = ref('');
const inspection = ref<Inspection | null>(null);
const inspecting = ref(false);
const worktree = ref(true);
const prompt = ref('');
const submitting = ref(false);
const serverError = ref('');
// Custom-path problem, shown under the Folder field (not in the footer error block).
const pathError = ref('');
const activeIndex = ref(0);
const errorBox = ref<HTMLElement | null>(null);

const folderPath = computed(() => (useCustom.value ? customPath.value.trim() : selected.value));
const worktreeAllowed = computed(() => inspection.value?.worktree_allowed !== false);

const listState = computed(() =>
  repoListState({ loading: reposLoading.value, error: scanError.value, resp: reposResp.value }),
);
const filtered = computed(() => filterRepos(repos.value, query.value));

const openTasks = computed(() =>
  store.slots.filter((s) => s && ['starting', 'running', 'needs-input'].includes(s.state)).map((s) => s!.task),
);

const normalizedTask = computed(() => task.value.trim().toLowerCase());
const taskValid = computed(
  () => /^[a-z0-9-]{1,40}$/.test(normalizedTask.value) && !openTasks.value.includes(normalizedTask.value),
);
const taskError = computed(() => (taskTouched.value && !taskValid.value ? TEXT.taskRule : ''));

const selectedDisplay = computed(() => {
  if (useCustom.value) return customPath.value.trim();
  return repos.value.find((r) => r.path === selected.value)?.display ?? selected.value;
});

const preview = computed(() => {
  if (!worktree.value || !worktreeAllowed.value || !selectedDisplay.value || !taskValid.value) return '';
  if (useCustom.value && (!inspection.value || pathError.value)) return ''; // never preview an unverified path
  const d = selectedDisplay.value.replace(/\/+$/, '');
  return `Creates ${d}-wt-${normalizedTask.value} on branch ctl/${normalizedTask.value}`;
});

const canSubmit = computed(
  () => !submitting.value && !!folderPath.value && !pathError.value && prompt.value.length <= PROMPT_MAX,
);

let inspectSeq = 0;
async function inspect(path: string) {
  const seq = ++inspectSeq;
  if (!path) {
    inspection.value = null;
    return;
  }
  inspecting.value = true;
  try {
    const r = await api.inspect(path);
    if (seq !== inspectSeq) return;
    inspection.value = r;
    serverError.value = '';
    pathError.value = '';
  } catch (e) {
    if (seq !== inspectSeq) return;
    inspection.value = null;
    if (useCustom.value && e instanceof ApiError) pathError.value = e.message;
  } finally {
    if (seq === inspectSeq) inspecting.value = false;
  }
}

watch(inspection, (r) => {
  worktree.value = r ? r.worktree_allowed : true;
});

function choose(r: Repo) {
  selected.value = r.path;
  useCustom.value = false;
  void inspect(r.path);
}

function onListKey(e: KeyboardEvent) {
  const n = filtered.value.length;
  if (!n) return;
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    activeIndex.value = (activeIndex.value + 1) % n;
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    activeIndex.value = (activeIndex.value - 1 + n) % n;
  } else if (e.key === 'Enter') {
    e.preventDefault();
    choose(filtered.value[activeIndex.value]);
  }
}

watch(query, () => (activeIndex.value = 0));

let customTimer: ReturnType<typeof setTimeout> | null = null;
watch(customPath, (p) => {
  if (!useCustom.value) return;
  pathError.value = '';
  inspection.value = null;
  if (customTimer) clearTimeout(customTimer);
  customTimer = setTimeout(() => void inspect(p.trim()), 300);
});

watch(useCustom, (on) => {
  inspection.value = null;
  pathError.value = '';
  if (on && customPath.value.trim()) void inspect(customPath.value.trim());
  if (!on && selected.value) void inspect(selected.value);
});

async function submit() {
  taskTouched.value = true;
  serverError.value = '';
  if (!taskValid.value) {
    serverError.value = TEXT.taskRule;
    errorBox.value?.focus();
    return;
  }
  submitting.value = true;
  try {
    const r = await api.launch({
      task: normalizedTask.value,
      path: folderPath.value,
      worktree: worktree.value && worktreeAllowed.value,
      prompt: prompt.value,
      slot: props.slot,
    });
    try {
      localStorage.setItem(LAST_KEY, folderPath.value);
    } catch {
      /* per-viewer convenience only */
    }
    if (r?.info) store.applySlot(r.slot, r.info);
    emit('close');
  } catch (e) {
    serverError.value = e instanceof ApiError ? e.message : 'The controller did not respond. Try again.';
    requestAnimationFrame(() => errorBox.value?.focus());
  } finally {
    submitting.value = false;
  }
}

async function loadRepos() {
  reposLoading.value = true;
  scanError.value = null;
  try {
    reposResp.value = await api.repos();
    repos.value = reposResp.value.repos;
  } catch (e) {
    reposResp.value = null;
    repos.value = [];
    scanError.value = { status: e instanceof ApiError ? e.status : 0 };
  } finally {
    reposLoading.value = false;
  }
}

function openSettings() {
  emit('close');
  store.settingsOpen = true;
}

onMounted(async () => {
  await loadRepos();
  let last = '';
  try {
    last = localStorage.getItem(LAST_KEY) ?? '';
  } catch {
    last = '';
  }
  const hit = repos.value.find((r) => r.path === last);
  if (hit) choose(hit);
});
</script>

<template>
  <ModalShell labelledby="launch-title" initial-focus="#launch-task" :width="600" @close="emit('close')">
    <form class="form" data-testid="launch-form" novalidate @submit.prevent="submit">
      <header class="fhead">
        <h2 id="launch-title">New session in slot {{ slot }}</h2>
        <button type="button" class="icon-btn" aria-label="Cancel" @click="emit('close')">
          <AppIcon name="close" :size="16" />
        </button>
      </header>

      <div class="field">
        <label for="launch-task">Task name</label>
        <input
          id="launch-task"
          v-model="task"
          class="input mono"
          name="task"
          autocomplete="off"
          spellcheck="false"
          maxlength="60"
          placeholder="fix-login"
          :aria-invalid="!!taskError"
          aria-describedby="launch-task-hint"
          @blur="taskTouched = true"
        />
        <p id="launch-task-hint" class="hint" :class="{ 'hint--err': taskError }">
          {{ taskError || 'a-z, 0-9 and -, up to 40 characters. Becomes the branch ctl/<task>.' }}
        </p>
      </div>

      <fieldset class="field">
        <legend>Folder</legend>
        <template v-if="!useCustom">
          <div class="search">
            <AppIcon name="search" :size="15" />
            <input
              v-model="query"
              class="input input--bare"
              type="search"
              placeholder="Search repositories"
              aria-label="Search repositories"
              aria-controls="repo-list"
              :aria-activedescendant="filtered.length ? `repo-opt-${activeIndex}` : undefined"
              @keydown="onListKey"
            />
          </div>
          <ul
            id="repo-list"
            class="repos"
            :role="listState.kind === 'list' ? 'listbox' : undefined"
            aria-label="Repositories"
            :aria-live="listState.kind === 'list' ? undefined : 'polite'"
          >
            <li v-if="listState.kind === 'loading'" class="repos-empty">{{ listState.message }}</li>
            <li v-else-if="listState.kind !== 'list'" class="repos-empty" data-testid="repo-state">
              <span>{{ listState.message }}</span>
              <button v-if="listState.kind === 'unset'" type="button" class="btn btn--sm" @click="openSettings">Open Settings</button>
              <button v-else-if="listState.kind === 'timeout' || listState.kind === 'error'" type="button" class="btn btn--sm" @click="loadRepos">
                Retry
              </button>
            </li>
            <li v-else-if="!filtered.length" class="repos-empty">No repositories match. Use a custom path instead.</li>
            <li
              v-for="(r, i) in filtered"
              :id="`repo-opt-${i}`"
              :key="r.path"
              role="option"
              class="repo"
              :class="{ 'repo--active': i === activeIndex, 'repo--sel': r.path === selected }"
              :aria-selected="r.path === selected"
              :data-path="r.path"
              @click="choose(r)"
              @mouseenter="activeIndex = i"
            >
              <AppIcon name="folder" :size="15" />
              <span class="repo-name">{{ r.name }}</span>
              <span class="repo-path mono">{{ r.display }}</span>
            </li>
          </ul>
          <button type="button" class="linkish" @click="useCustom = true">Use a custom path</button>
        </template>
        <template v-else>
          <input
            id="launch-path"
            v-model="customPath"
            class="input mono"
            name="path"
            aria-label="Custom folder path"
            :aria-invalid="!!pathError"
            :aria-describedby="pathError ? 'launch-path-error' : undefined"
            placeholder="/Users/me/code/project"
            autocomplete="off"
            spellcheck="false"
          />
          <p v-if="pathError" id="launch-path-error" class="hint hint--err" role="alert" data-testid="path-error">{{ pathError }}</p>
          <button type="button" class="linkish" @click="useCustom = false">Back to the repository list</button>
        </template>
      </fieldset>

      <div class="field field--row">
        <label class="switch" :class="{ 'switch--off': !worktreeAllowed }">
          <input
            v-model="worktree"
            type="checkbox"
            role="switch"
            data-testid="worktree-toggle"
            :disabled="!worktreeAllowed || !folderPath"
          />
          <span class="track" aria-hidden="true"><span class="thumb" /></span>
          <span class="switch-label">
            <span>Isolate in a git worktree</span>
            <span v-if="!worktreeAllowed" class="hint" data-testid="worktree-note">{{
              inspection?.note || TEXT.notGit
            }}</span>
            <span v-else-if="preview" class="hint mono">{{ preview }}</span>
            <span v-else class="hint">New sibling folder on its own branch, from the repo's current HEAD.</span>
          </span>
        </label>
      </div>

      <div class="field">
        <label for="launch-prompt">First prompt <span class="opt">optional</span></label>
        <textarea
          id="launch-prompt"
          v-model="prompt"
          class="input textarea"
          name="prompt"
          rows="4"
          :maxlength="PROMPT_MAX"
          aria-describedby="launch-prompt-count"
          placeholder="What should Claude work on?"
        />
        <p id="launch-prompt-count" class="hint count">
          {{ prompt.length.toLocaleString('en-US') }} / {{ PROMPT_MAX.toLocaleString('en-US') }}
        </p>
      </div>

      <div v-if="serverError" ref="errorBox" class="ferror" role="alert" tabindex="-1" data-testid="launch-error">
        <AppIcon name="alert" :size="16" /><span>{{ serverError }}</span>
      </div>

      <footer class="ffoot">
        <button type="button" class="btn" @click="emit('close')">Cancel</button>
        <button type="submit" class="btn btn--primary" :disabled="!canSubmit" data-testid="launch-submit">
          <span v-if="submitting" class="spinner" aria-hidden="true" />{{ submitting ? 'Starting…' : 'Start session' }}
        </button>
      </footer>
    </form>
  </ModalShell>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
}
.fhead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 14px 8px 20px;
}
.fhead h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
}
.field {
  margin: 0;
  padding: 8px 20px;
  border: 0;
  min-width: 0;
}
label,
legend {
  display: block;
  padding: 0;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 600;
}
.opt {
  font-weight: 400;
  color: var(--muted);
}
.input {
  display: block;
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--rule-2);
  border-radius: var(--radius);
  background: var(--panel);
  font-size: 14px;
}
.input:focus-visible {
  border-color: var(--accent);
}
.input[aria-invalid='true'] {
  border-color: var(--err);
}
.input--bare {
  border: 0;
  height: 34px;
  padding: 0;
  background: transparent;
}
.input--bare:focus-visible {
  box-shadow: none;
}
.textarea {
  height: auto;
  padding: 8px 10px;
  resize: vertical;
  line-height: 1.45;
}
.hint {
  display: block;
  margin: 5px 0 0;
  font-size: 12px;
  font-weight: 400;
  color: var(--ink-2);
}
.hint--err {
  color: var(--err);
}
.count {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  border: 1px solid var(--rule-2);
  border-radius: var(--radius) var(--radius) 0 0;
  color: var(--muted);
}
.search:focus-within {
  border-color: var(--accent);
  box-shadow: var(--focus);
}
.repos {
  list-style: none;
  margin: 0;
  padding: 4px;
  max-height: 188px;
  overflow: auto;
  border: 1px solid var(--rule-2);
  border-top: 0;
  border-radius: 0 0 var(--radius) var(--radius);
  background: var(--paper);
}
.repo {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  cursor: pointer;
  color: var(--ink-2);
}
.repo--active {
  background: var(--paper-2);
}
.repo--sel {
  background: var(--accent-soft);
  color: var(--accent);
  box-shadow: inset 2px 0 0 var(--accent);
}
.repo-name {
  font-weight: 600;
  color: var(--ink);
}
.repo-path {
  margin-left: auto;
  font-size: 12px;
  color: var(--muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.repos-empty {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px 8px;
  color: var(--muted);
  font-size: 13px;
}
.linkish {
  margin-top: 6px;
  padding: 2px 0;
  border: 0;
  background: none;
  color: var(--accent);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}
.field--row {
  padding-top: 10px;
}
.switch {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin: 0;
  font-weight: 500;
  cursor: pointer;
}
.switch input {
  position: absolute;
  opacity: 0;
  width: 1px;
  height: 1px;
}
.track {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  margin-top: 1px;
  border-radius: 999px;
  background: var(--rule-2);
  transition: background-color 140ms ease;
}
.thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
  transition: transform 140ms ease;
}
.switch input:checked + .track {
  background: var(--ok);
}
.switch input:checked + .track .thumb {
  transform: translateX(16px);
}
.switch input:focus-visible + .track {
  box-shadow: var(--focus);
}
.switch input:disabled + .track {
  opacity: 0.5;
}
.switch--off {
  cursor: not-allowed;
}
.switch-label {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.switch-label .hint {
  overflow-wrap: anywhere;
}
.ferror {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 8px 20px 0;
  padding: 10px 12px;
  border-radius: var(--radius);
  background: var(--err-soft);
  border: 1px solid #efb8b2;
  color: #7a1a12;
  font-size: 13px;
  font-weight: 500;
}
.ffoot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 12px;
  padding: 12px 20px 16px;
  border-top: 1px solid var(--rule);
  background: var(--paper);
  /* Keep Start session in view on short windows (1280x720) while the form scrolls. */
  position: sticky;
  bottom: 0;
  z-index: 1;
}
.spinner {
  width: 13px;
  height: 13px;
  border-radius: 50%;
  border: 2px solid rgba(245, 243, 238, 0.35);
  border-top-color: var(--paper);
  animation: spin 0.8s linear infinite;
}
</style>
