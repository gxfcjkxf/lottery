import {expect,it} from 'vitest'
import {readPaginationTotal} from './pagination'
it('preserves exact totals, including empty and larger-than-safe-integer counts',()=>{
 for(const total_count of ['0','51','9007199254740993'])expect(readPaginationTotal({items:[],total_count})).toBe(total_count)
})
it('rejects missing, estimated or malformed totals instead of counting the loaded rows',()=>{
 for(const page of [{items:[1]}, {total_count:null}, {total_count:51}, {total_count:'01'}, {total_count:'-1'}, {total_count:'unknown'}])expect(()=>readPaginationTotal(page)).toThrow('Invalid pagination total_count')
})
