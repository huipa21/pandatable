import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import VueI18n from '@intlify/unplugin-vue-i18n/vite'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [
    vue(),
    VueI18n({
      include: fileURLToPath(new URL('./src/i18n/*.json', import.meta.url)),
      runtimeOnly: true,
    }),
  ],
  build: {
    target: 'es2022',
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': {
        target: process.env.PT_DEV_API_TARGET ?? 'http://127.0.0.1:8080',
        ws: true,
      },
      '/healthz': {
        target: process.env.PT_DEV_API_TARGET ?? 'http://127.0.0.1:8080',
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    clearMocks: true,
    maxWorkers: 2,
  },
})
