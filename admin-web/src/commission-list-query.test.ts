import {describe,it,expect} from 'vitest'
import {setPlatformDisplaySettings} from '@lottery/shared'
import {commissionDateRange,commissionListParams} from './commission-list-query'
describe('commission list queries',()=>{
 it('uses platform day boundaries including the full end day',()=>{
  setPlatformDisplaySettings({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
  expect(commissionDateRange('2026-10-01','2026-10-01')).toEqual({from:'2026-09-30T16:00:00.000Z',to:'2026-10-01T16:00:00.000Z'})
  setPlatformDisplaySettings({timezone:'America/New_York',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
  expect(commissionDateRange('2026-11-01','2026-11-01')).toEqual({from:'2026-11-01T04:00:00.000Z',to:'2026-11-02T05:00:00.000Z'})
 })
 it('rejects incomplete and invalid ranges and unsupported query fields',()=>{
  expect(()=>commissionDateRange('2026-02-30','2026-03-01')).toThrow()
  expect(()=>commissionDateRange('2026-10-01','')).toThrow()
  expect(()=>commissionListParams(20,0,{state:'needs_attention'},'cycle')).toThrow()
  expect(()=>commissionListParams(20,0,{cycle_id:'bad'},'payment')).toThrow()
 })
 it('sends filters with pagination to the server',()=>{
  const params=commissionListParams(20,40,{state:'needs_attention',agent_id:'11111111-1111-4111-8111-111111111111'},'payment')
  expect(params.get('offset')).toBe('40');expect(params.get('state')).toBe('needs_attention');expect(params.has('agent_id')).toBe(true)
 })
})
