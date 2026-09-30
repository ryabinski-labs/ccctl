<script setup lang="ts">
import { computed } from 'vue';
import { bannerText } from '../lib/backoff';
import { useSessions } from '../stores/sessions';
import AppIcon from './AppIcon.vue';

const store = useSessions();
const text = computed(() => bannerText(store.host, store.retryIn));
</script>

<template>
  <div v-if="store.lost" class="banner" data-testid="reconnect-banner">
    <AppIcon name="alert" :size="16" />
    <span>{{ text }}</span>
  </div>
</template>

<style scoped>
.banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 14px;
  border-bottom: 1px solid #efb8b2;
  background: var(--err-soft);
  color: #7a1a12;
  font-size: 13px;
  font-weight: 500;
}
</style>
