import {describe,it,expect,vi} from 'vitest'
import {createDataRetentionApi,validRetentionInput,type RetentionIntent} from './data-retention-api'
const policy={version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:'Asia/Singapore',next_run:null,updated_at:'2026-11-12T00:00:00Z'}
const response=(data:unknown)=>new Response(JSON.stringify({success:true,data,request_id:'test'}),{status:200})
describe('monthly data retention API',()=>{
 it('accepts only the monthly schedule and day 1–28',()=>{
  expect(validRetentionInput(policy)).toBe(true)
  for(const execution_day of [0,29,31,1.5])expect(validRetentionInput({...policy,execution_day})).toBe(false)
  expect(validRetentionInput({...policy,execution_time:'2:00'})).toBe(false)
 })
 it('preserves the original actor, body and key when an outcome is unknown',async()=>{
  const intent:RetentionIntent={kind:'policy',actor:'11111111-1111-4111-8111-111111111111',key:'retention-original-key',conflict:false,body:Object.freeze({version:1,enabled:true,execution_day:10,execution_time:'02:00'})}
  const result={...policy,...intent.body,version:2,next_run:{execute_at:'2026-12-09T18:00:00Z',cutoff_at:'2026-10-31T16:00:00Z'}}
  const fetcher=vi.fn<typeof fetch>().mockRejectedValueOnce(new Error('lost response')).mockResolvedValueOnce(response(result))
  const api=createDataRetentionApi(fetcher)
  await expect(api.write(intent)).rejects.toMatchObject({code:'UNKNOWN_WRITE_STATUS'})
  await expect(api.write(intent)).resolves.toEqual(result)
  expect(fetcher).toHaveBeenCalledTimes(2)
  expect(fetcher.mock.calls[0][1]?.body).toBe(fetcher.mock.calls[1][1]?.body)
  const headers=new Headers(fetcher.mock.calls[1][1]?.headers)
  expect(headers.get('X-Data-Retention-Actor-ID')).toBe(intent.actor);expect(headers.get('Idempotency-Key')).toBe(intent.key)
  expect(headers.has('X-Brand-ID')).toBe(false)
 })
 it('rejects a mismatched success receipt as an unknown write result',async()=>{
  const intent:RetentionIntent={kind:'policy',actor:'11111111-1111-4111-8111-111111111111',key:'retention-original-key',conflict:false,body:{version:1,enabled:false,execution_day:10,execution_time:'02:00'}}
  await expect(createDataRetentionApi(async()=>response({...policy,version:2,execution_day:20})).write(intent)).rejects.toMatchObject({code:'UNKNOWN_WRITE_STATUS'})
 })
 it('rejects cross-brand or imprecise task counts',async()=>{
  const task={id:'11111111-1111-4111-8111-111111111111',brand_id:'22222222-2222-4222-8222-222222222222',brand_name:'Test',policy_version:1,timezone:'UTC',execute_at:'2026-11-10T02:00:00Z',cutoff_at:'2026-10-01T00:00:00Z',state:'completed',counts:{audit_logs:{deleted:'999999999999999999',receipts:'0',deferred:'0'}},error_code:'',created_at:'2026-11-10T02:00:00Z',finished_at:'2026-11-10T02:10:00Z'}
  const page={items:[task],total_count:'1',limit:50,offset:0}
  await expect(createDataRetentionApi(async()=>response(page)).tasks(task.brand_id,50,0)).resolves.toEqual(page)
  await expect(createDataRetentionApi(async()=>response(page)).tasks('33333333-3333-4333-8333-333333333333',50,0)).rejects.toMatchObject({code:'INVALID_RESPONSE'})
  task.counts.audit_logs.deleted='1.5'
  await expect(createDataRetentionApi(async()=>response(page)).tasks(task.brand_id,50,0)).rejects.toMatchObject({code:'INVALID_RESPONSE'})
 })
})
