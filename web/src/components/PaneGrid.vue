<script setup lang="ts">
import { computed } from 'vue';
import { useSessions } from '../stores/sessions';
import SessionPane from './SessionPane.vue';

const store = useSessions();
const columns = computed(() => (store.maximized ? '1fr' : 'repeat(2, 1fr)'));
const rows = computed(() => (store.maximized ? '1fr' : 'repeat(2, 1fr)'));
</script>

<template>
  <main class="grid" data-testid="grid" :style="{ gridTemplateColumns: columns, gridTemplateRows: rows }">
    <SessionPane
      v-for="n in 4"
      v-show="store.maximized === null || store.maximized === n"
      :key="n"
      :slot="n"
    />
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
