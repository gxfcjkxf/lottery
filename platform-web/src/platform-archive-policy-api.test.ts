import {describe,it,expect,vi} from 'vitest';
import {createPlatformArchivePolicyApi} from './platform-archive-policy-api';
const input={version:1,daily_enabled:true,monthly_enabled:true,daily_execution_time:'07:30',daily_period_offset:2,monthly_execution_day:5,monthly_execution_time:'06:00',monthly_period_offset:2};
const policy={...input,version:2,timezone:'Asia/Singapore',next_runs:{daily:null,monthly:{execute_at:'2026-11-04T22:00:00Z',period_key:'2026-09'}},updated_at:'2026-10-27T00:00:00Z'};
const response=(data:unknown)=>new Response(JSON.stringify({success:true,data}),{status:200});
describe('platform global archive policy API',()=>{
 it('reads and writes one configuration without a brand header',async()=>{
  const f=vi.fn<typeof fetch>().mockImplementation(async()=>response(policy));const api=createPlatformArchivePolicyApi(f);
  await expect(api.read()).resolves.toEqual(policy);await expect(api.update(input,'global-archive-test-key','11111111-1111-4111-8111-111111111111')).resolves.toEqual(policy);
  expect(f.mock.calls.every(([,init])=>!new Headers(init?.headers).has('X-Brand-ID'))).toBe(true);expect(JSON.parse(String(f.mock.calls[1]![1]?.body))).toEqual(input);
 });
 it('keeps the exact original body across an in-flight edit and rejects mismatched acknowledgements',async()=>{
  let finish!:(r:Response)=>void;const f=vi.fn<typeof fetch>().mockImplementation(()=>new Promise(resolve=>finish=resolve));const mutable={...input};const pending=createPlatformArchivePolicyApi(f).update(mutable,'global-archive-test-key','11111111-1111-4111-8111-111111111111');mutable.monthly_period_offset=1;finish(response(policy));await expect(pending).resolves.toEqual(policy);
  await expect(createPlatformArchivePolicyApi(async()=>response({...policy,monthly_period_offset:1})).update(input,'global-archive-test-key','11111111-1111-4111-8111-111111111111')).rejects.toMatchObject({code:'UNKNOWN_WRITE_STATUS'});
 });
});
