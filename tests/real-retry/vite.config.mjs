import {defineConfig} from '../../user-web/node_modules/vite/dist/node/index.js'
import vue from '../../user-web/node_modules/@vitejs/plugin-vue/dist/index.mjs'
if(!process.env.RETRY_REAL_API_URL)throw Error('Owned API fixture required')
export default defineConfig({plugins:[vue()],server:{strictPort:true,proxy:{'/api':{target:process.env.RETRY_REAL_API_URL,changeOrigin:false}}}})
