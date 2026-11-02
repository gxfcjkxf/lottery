import config from '../../admin-web/vite.config'
import {readFileSync} from 'node:fs'
const fixture=JSON.parse(readFileSync(process.env.BRAND_SCOPE_FIXTURE_FILE!,'utf8'))
config.server={...config.server,host:'127.0.0.1',port:15684,strictPort:true,proxy:{'/api':{target:fixture.url,changeOrigin:false}}}
export default config
