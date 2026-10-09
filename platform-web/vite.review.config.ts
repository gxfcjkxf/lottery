import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '127.0.0.1',
    port: 5185,
    strictPort: true,
    proxy: { '/api': { target: 'http://localhost:8085', changeOrigin: false } },
  },
})
