<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue';
import { api } from '../lib/api';
import AppIcon from './AppIcon.vue';
import ModalShell from './ModalShell.vue';

const emit = defineEmits<{ close: [] }>();

// Variables are write-only: the page only ever learns names, never values.
const names = ref<string[]>([]);
const loading = ref(true);
const loadError = ref('');
const name = ref('');
const value = ref('');
const saving = ref(false);
const error = ref('');
const saved = ref('');
const confirmRemove = ref('');
const valueInput = ref<HTMLInputElement | null>(null);

// The one folder New session scans for repos. Empty means no scan.
const prefix = ref('');
const savedPrefix = ref('');
const prefixSaving = ref(false);
const prefixError = ref('');
const prefixSaved = ref('');
const prefixDirty = computed(() => prefix.value.trim() !== savedPrefix.value);

const SUGGESTED = ['CLAUDE_CODE_OAUTH_TOKEN', 'GEMINI_API_KEY', 'ANTHROPIC_API_KEY', 'OPENAI_API_KEY', 'GITHUB_TOKEN'];
const suggestions = computed(() => SUGGESTED.filter((n) => !names.value.includes(n)));
const nameOk = computed(() => /^[A-Za-z_][A-Za-z0-9_]*$/.test(name.value));
const replacing = computed(() => names.value.includes(name.value));
const canSave = computed(() => nameOk.value && value.value.length > 0 && !saving.value);

onMounted(async () => {
  api
    .repoPrefix()
    .then((p) => {
      savedPrefix.value = p;
      if (!prefix.value) prefix.value = p; // keep anything typed before the load finished
    })
    .catch(() => {
      /* the field stays empty; saving still works */
    });
  try {
    names.value = await api.envNames();
  } catch (e) {
    loadError.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
});

async function savePrefix() {
  if (prefixSaving.value) return;
  prefixSaving.value = true;
  prefixError.value = '';
  prefixSaved.value = '';
  const typed = prefix.value.trim();
  try {
    if (typed === '') {
      savedPrefix.value = await api.repoPrefixClear();
      prefixSaved.value = 'Repository folder cleared. New session will not scan until you set one.';
    } else {
      savedPrefix.value = await api.repoPrefixSet(typed);
      prefixSaved.value = 'Repository folder saved. It applies the next time you open New session.';
    }
    prefix.value = savedPrefix.value;
  } catch (e) {
    prefixError.value = (e as Error).message;
  } finally {
    prefixSaving.value = false;
  }
}

async function clearPrefix() {
  prefix.value = '';
  await savePrefix();
}

async function save() {
  if (!canSave.value) return;
  saving.value = true;
  error.value = '';
  saved.value = '';
  const n = name.value;
  try {
    names.value = await api.envSet(n, value.value);
    saved.value = `${n} saved. It applies to sessions you start from now on.`;
    name.value = '';
    value.value = '';
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}

async function replace(n: string) {
  name.value = n;
  value.value = '';
  saved.value = '';
  await nextTick();
  valueInput.value?.focus();
}

async function remove(n: string) {
  error.value = '';
  saved.value = '';
  try {
    names.value = await api.envRemove(n);
    saved.value = `${n} removed. Sessions you start from now on will not get it.`;
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    confirmRemove.value = '';
  }
}
</script>

<template>
  <ModalShell labelledby="settings-title" describedby="settings-desc" initial-focus="#repo-prefix" :width="560" @close="emit('close')">
    <div class="wrap" data-testid="settings-dialog">
      <header class="fhead">
        <h2 id="settings-title">Settings</h2>
        <button type="button" class="icon-btn" aria-label="Close" @click="emit('close')">
          <AppIcon name="close" :size="16" />
        </button>
      </header>
      <form class="prefix" novalidate autocomplete="off" @submit.prevent="savePrefix">
        <label for="repo-prefix">Repository folder</label>
        <div class="prefix-row">
          <input
            id="repo-prefix"
            v-model="prefix"
            class="input mono"
            spellcheck="false"
            placeholder="~/projects"
            :aria-invalid="!!prefixError"
            :aria-describedby="prefixError ? 'repo-prefix-hint repo-prefix-error' : 'repo-prefix-hint'"
          />
          <button type="submit" class="btn btn--primary" :disabled="prefixSaving || !prefixDirty" data-testid="prefix-save">
            {{ prefixSaving ? 'Saving…' : 'Save folder' }}
          </button>
          <button v-if="savedPrefix" type="button" class="btn" :disabled="prefixSaving" @click="clearPrefix">Clear</button>
        </div>
        <p id="repo-prefix-hint" class="hint">
          Only git repos under this folder are listed when you start a session. Leave empty to turn scanning off.
        </p>
        <p v-if="prefixError" id="repo-prefix-error" class="ferror" role="alert" data-testid="prefix-error">
          <AppIcon name="alert" :size="16" />{{ prefixError }}
        </p>
        <p v-if="prefixSaved" class="ok" role="status" data-testid="prefix-saved">{{ prefixSaved }}</p>
      </form>

      <h3 class="sub env-head">Session environment</h3>
      <p id="settings-desc" class="desc">
        Added to every Claude session you start, for example <code>CLAUDE_CODE_OAUTH_TOKEN</code> or
        <code>GEMINI_API_KEY</code>. Values are stored in <code>~/.ccctl/config.toml</code> on this host and are never
        shown again. Running sessions keep what they started with.
      </p>

      <section class="list" aria-labelledby="env-list-title">
        <h3 id="env-list-title" class="sub">Saved variables</h3>
        <p v-if="loading" class="muted">Loading…</p>
        <p v-else-if="loadError" class="ferror" role="alert"><AppIcon name="alert" :size="16" />{{ loadError }}</p>
        <p v-else-if="!names.length" class="muted" data-testid="env-empty">None yet. Sessions use the host's own Claude login.</p>
        <ul v-else class="rows">
          <li v-for="n in names" :key="n" class="row" data-testid="env-row">
            <AppIcon name="key" :size="15" />
            <span class="mono nm">{{ n }}</span>
            <span class="set">Set</span>
            <template v-if="confirmRemove === n">
              <span class="ask">Remove?</span>
              <button type="button" class="btn btn--sm btn--danger" @click="remove(n)">Remove</button>
              <button type="button" class="btn btn--sm" @click="confirmRemove = ''">Keep</button>
            </template>
            <template v-else>
              <button type="button" class="btn btn--sm btn--ghost" :aria-label="`Replace ${n}`" @click="replace(n)">Replace</button>
              <button type="button" class="btn btn--sm btn--ghost" :aria-label="`Remove ${n}`" @click="confirmRemove = n">Remove</button>
            </template>
          </li>
        </ul>
      </section>

      <form class="add" novalidate autocomplete="off" @submit.prevent="save">
        <h3 class="sub">{{ replacing ? `Replace ${name}` : 'Add a variable' }}</h3>
        <div class="grid">
          <div>
            <label for="env-name">Name</label>
            <input
              id="env-name"
              v-model.trim="name"
              class="input mono"
              list="env-suggestions"
              spellcheck="false"
              autocapitalize="characters"
              placeholder="CLAUDE_CODE_OAUTH_TOKEN"
              :aria-invalid="name.length > 0 && !nameOk"
              aria-describedby="env-name-hint"
            />
            <datalist id="env-suggestions">
              <option v-for="s in suggestions" :key="s" :value="s" />
            </datalist>
            <p id="env-name-hint" class="hint" :class="{ bad: name.length > 0 && !nameOk }">
              Letters, digits and _; not starting with a digit.
            </p>
          </div>
          <div>
            <label for="env-value">Value</label>
            <input
              id="env-value"
              ref="valueInput"
              v-model="value"
              class="input mono"
              type="password"
              autocomplete="new-password"
              data-1p-ignore
              data-lpignore="true"
              spellcheck="false"
              placeholder="Paste the value"
            />
            <p class="hint">Write-only: it is not displayed after saving.</p>
          </div>
        </div>
        <p v-if="error" class="ferror" role="alert" data-testid="env-error"><AppIcon name="alert" :size="16" />{{ error }}</p>
        <p v-if="saved" class="ok" role="status" data-testid="env-saved">{{ saved }}</p>
        <footer class="ffoot">
          <button type="button" class="btn" @click="emit('close')">Done</button>
          <button type="submit" class="btn btn--primary" :disabled="!canSave" data-testid="env-save">
            {{ saving ? 'Saving…' : replacing ? 'Replace value' : 'Save variable' }}
          </button>
        </footer>
      </form>
    </div>
  </ModalShell>
</template>

<style scoped>
.fhead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 14px 4px 20px;
}
.fhead h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 600;
}
.desc {
  margin: 0;
  padding: 0 20px 8px;
  color: var(--ink-2);
  font-size: 13px;
  line-height: 1.5;
}
code {
  font-family: var(--mono, 'IBM Plex Mono', monospace);
  font-size: 12px;
  padding: 0 3px;
  border-radius: 3px;
  background: var(--paper);
}
.sub {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
}
.prefix {
  padding: 8px 20px 4px;
}
.prefix .ferror,
.prefix .ok {
  margin: 8px 0 0;
}
.prefix-row {
  display: flex;
  gap: 8px;
}
.env-head {
  margin: 12px 0 6px;
  padding: 12px 20px 0;
  border-top: 1px solid var(--rule);
}
.list {
  padding: 8px 20px;
}
.muted {
  margin: 0;
  color: var(--muted);
  font-size: 13px;
}
.rows {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--rule);
  border-radius: var(--radius);
}
.row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  padding: 4px 6px 4px 10px;
  color: var(--ink-2);
}
.row + .row {
  border-top: 1px solid var(--rule);
}
.nm {
  color: var(--ink);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.set {
  margin-right: auto;
  padding: 1px 7px;
  border-radius: 999px;
  background: #e3f1e9;
  color: var(--ok);
  font-size: 11px;
  font-weight: 600;
}
.ask {
  font-size: 13px;
  color: var(--ink);
}
.add {
  margin-top: 4px;
  padding-top: 12px;
  border-top: 1px solid var(--rule);
}
.add .sub {
  padding: 0 20px;
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  padding: 0 20px;
}
label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 600;
}
.input {
  display: block;
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--rule-2);
  border-radius: var(--radius);
  background: var(--panel);
  font-size: 13px;
}
.input:focus-visible {
  border-color: var(--accent);
}
.input[aria-invalid='true'] {
  border-color: var(--err);
}
.hint {
  margin: 4px 0 0;
  color: var(--muted);
  font-size: 12px;
}
.hint.bad {
  color: var(--err);
}
.ferror {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin: 10px 20px 0;
  padding: 10px 12px;
  border-radius: var(--radius);
  background: var(--err-soft);
  border: 1px solid #efb8b2;
  color: #7a1a12;
  font-size: 13px;
}
.list .ferror {
  margin: 0;
}
.ok {
  margin: 10px 20px 0;
  padding: 8px 12px;
  border-radius: var(--radius);
  background: #e3f1e9;
  color: #14532f;
  font-size: 13px;
}
.ffoot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 14px;
  padding: 12px 20px 16px;
  border-top: 1px solid var(--rule);
  background: var(--paper);
  position: sticky;
  bottom: 0;
}
</style>
