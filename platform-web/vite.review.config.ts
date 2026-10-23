import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '127.0.0.1',
    port: 5185,
    strictPort: true,
    allowedHosts: [],
    proxy: { '/api': { target: 'http://127.0.0.1:8085', changeOrigin: false } },
  },
})
