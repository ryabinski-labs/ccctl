<script setup lang="ts">
import { TEXT } from '../lib/types';
import ModalShell from './ModalShell.vue';

const props = defineProps<{ task: string; slot: number; busy?: boolean }>();
const emit = defineEmits<{ cancel: []; confirm: [] }>();
const id = `stop-title-${props.slot}`;
</script>

<template>
  <ModalShell :labelledby="id" initial-focus="[data-cancel]" :width="420" @close="emit('cancel')">
    <div class="body" data-testid="stop-dialog">
      <h2 :id="id" class="title">{{ TEXT.stopConfirm(task) }}</h2>
      <p class="sub">Slot {{ slot }}. Claude gets a hang-up signal; if it is still running after 5 seconds it is killed.</p>
      <div class="actions">
        <button type="button" class="btn" data-cancel @click="emit('cancel')">Cancel</button>
        <button type="button" class="btn btn--danger" data-confirm :disabled="busy" @click="emit('confirm')">
          Stop session
        </button>
      </div>
    </div>
  </ModalShell>
</template>

<style scoped>
.body {
  padding: 20px 20px 16px;
}
.title {
  margin: 0 0 6px;
  font-size: 16px;
  font-weight: 600;
  line-height: 1.35;
}
.sub {
  margin: 0 0 18px;
  color: var(--ink-2);
  font-size: 13px;
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
