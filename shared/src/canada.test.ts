import {it,expect} from 'vitest'
import {canadaCatalog} from './canada'
it('keeps literal UI messages translated on every Canada surface',async()=>{const {readFileSync}=await import('node:fs');for(const file of ['CanadaOrders.vue','CanadaLine.vue']){const text=readFileSync(new URL(file,import.meta.url),'utf8');for(const [,key]of text.matchAll(/\bt\('([^']+)'/g)){if(key==='auth-expired'||key==='user')continue;expect(canadaCatalog.en).toHaveProperty(key!);expect(canadaCatalog['zh-CN']).toHaveProperty(key!)}}})
