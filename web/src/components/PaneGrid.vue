<script setup lang="ts">
import { computed } from 'vue';
import { gridShape, visibleSlots } from '../lib/layout';
import { MAX_SESSIONS } from '../lib/types';
import { useSessions } from '../stores/sessions';
import SessionPane from './SessionPane.vue';

const store = useSessions();
const shape = computed(() => gridShape(store.slots.filter(Boolean).length));
const shown = computed(() => visibleSlots(store.slots, store.maximized, store.canLaunch));
const columns = computed(() => (store.maximized ? '1fr' : `repeat(${shape.value.cols}, 1fr)`));
const rows = computed(() => (store.maximized ? '1fr' : `repeat(${shape.value.rows}, 1fr)`));
</script>

<template>
  <main class="grid" data-testid="grid" :style="{ gridTemplateColumns: columns, gridTemplateRows: rows }">
    <SessionPane v-for="n in MAX_SESSIONS" v-show="shown.includes(n)" :key="n" :slot="n" />
  </main>
</template>

<style scoped>
.grid {
  display: grid;
  gap: 8px;
  flex: 1 1 auto;
  min-height: 0;
  padding: 12px;
}
</style>
