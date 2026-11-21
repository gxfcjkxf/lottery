import {afterEach,describe,expect,it,vi} from 'vitest'
import {publicApiErrorKey} from '@lottery/shared'
import {request,AgentError} from './api'
afterEach(()=>vi.unstubAllGlobals())
describe('agent transport failures',()=>{
 it('keeps a keyed write unconfirmed and reuses the supplied body and key',async()=>{
  const fetcher=vi.fn().mockRejectedValueOnce(new TypeError('Failed to fetch')).mockResolvedValueOnce(new Response(JSON.stringify({success:true,data:{id:'member-id'}}),{status:201}))
  vi.stubGlobal('fetch',fetcher)
  const body={username:'isolated_member',password:'test-only-password'},key='unchanged-request-key'
  const error=await request('/members',body,key).catch(error=>error)
  expect(error).toBeInstanceOf(AgentError)
  expect(publicApiErrorKey(error)).toBe('unknown')
  expect(error).toMatchObject({status:0,code:'UNKNOWN_WRITE_STATUS'})
  await expect(request('/members',body,key)).resolves.toEqual({id:'member-id'})
  expect(fetcher.mock.calls[1]).toEqual(fetcher.mock.calls[0])
 })
 it('classifies failed reads as connection errors',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
  const error=await request('/members').catch(error=>error)
  expect(publicApiErrorKey(error)).toBe('network')
  expect(error).toMatchObject({code:'NETWORK_ERROR'})
 })
})
