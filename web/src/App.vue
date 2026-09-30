<script setup lang="ts">
import { onBeforeUnmount, onMounted, watch } from 'vue';
import { bannerText } from './lib/backoff';
import { chime } from './lib/chime';
import { handleShortcut } from './lib/shortcuts';
import { useSessions } from './stores/sessions';
import AppHeader from './components/AppHeader.vue';
import LaunchDialog from './components/LaunchDialog.vue';
import SettingsDialog from './components/SettingsDialog.vue';
import PaneGrid from './components/PaneGrid.vue';
import ReconnectBanner from './components/ReconnectBanner.vue';

const store = useSessions();

function onKey(e: KeyboardEvent) {
  if (handleShortcut(e)) e.preventDefault();
}
function onPointer() {
  if (!store.muted) chime.unlock();
}

// The live region announces the drop once; the countdown updates only visually.
watch(
  () => store.lost,
  (lost, was) => {
    if (lost && !was) store.announce(bannerText(store.host, store.retryIn));
  },
);

onMounted(() => {
  store.syncTitle();
  store.connect();
  window.addEventListener('keydown', onKey);
  window.addEventListener('pointerdown', onPointer, { once: true });
});
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>

<template>
  <div class="shell">
    <AppHeader />
    <ReconnectBanner />
    <PaneGrid />
    <LaunchDialog v-if="store.launchSlot !== null" :slot="store.launchSlot" @close="store.closeLaunch()" />
    <SettingsDialog v-if="store.settingsOpen" @close="store.settingsOpen = false" />
    <div class="sr-only" aria-live="polite" aria-atomic="true">{{ store.liveMessage }}</div>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  min-height: 600px;
}
</style>
