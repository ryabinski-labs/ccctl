import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig({
  plugins: [vue()],
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 900 },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:7681',
      '/ws': { target: 'ws://127.0.0.1:7681', ws: true },
    },
  },
});
