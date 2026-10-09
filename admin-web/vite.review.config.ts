import { mergeConfig } from 'vite'
import config from './vite.config'

export default mergeConfig(config, {
  server: {
    host: '127.0.0.1',
    port: 5184,
    strictPort: true,
    proxy: { '/api': { target: 'http://127.0.0.1:8085', changeOrigin: false } },
  },
})
