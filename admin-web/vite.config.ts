import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5174,
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: false } },
  },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
