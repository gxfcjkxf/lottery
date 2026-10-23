import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({plugins:[vue()],server:{host:'127.0.0.1',port:5186,strictPort:true,proxy:{'/api':{target:process.env.AGENT_API_URL || 'http://127.0.0.1:8085',changeOrigin:false}}}})
