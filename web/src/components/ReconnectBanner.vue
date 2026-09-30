<script setup lang="ts">
import { computed } from 'vue';
import { bannerText } from '../lib/backoff';
import { useSessions } from '../stores/sessions';
import AppIcon from './AppIcon.vue';

const store = useSessions();
const reload = () => window.location.reload();
const text = computed(() => bannerText(store.host, store.retryIn));
</script>

<template>
  <div v-if="store.lost" class="banner" data-testid="reconnect-banner">
    <AppIcon name="alert" :size="16" />
    <span>{{ text }}</span>
  </div>
  <div v-else-if="store.updated" class="banner banner--info" role="status" data-testid="updated-banner">
    <AppIcon name="alert" :size="16" />
    <span>ccctl on {{ store.host }} was updated. Reload to use the new version; your sessions keep running.</span>
    <button type="button" class="btn btn--sm" @click="reload">Reload</button>
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
.banner--info {
  border-bottom-color: var(--rule);
  background: #eef2fd;
  color: #1a2f6b;
}
.banner--info .btn {
  margin-left: auto;
}
</style>
