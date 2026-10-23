import { describe, expect, it, vi } from 'vitest'
import { createConfiguredBusinessClient } from './configured-business'
import { createApiClient } from './api'
import { readApiEnvelope } from './api-response'

const request = { period_id:'period',rule_id:'rule',sales_version:1,snapshot_hash:'snapshot',actor_context:'actor',requests:[] }
describe('current API response protocol', () => {
  it.each([
    {success:false,error:'old rejection'},
    {success:false,error:{code:'CONFLICT'}},
    {success:false,error:{message:'Missing code'}},
    {success:false,error:{code:'bad code',message:'Invalid code'}},
    {success:true,data:{id:'unconfirmed'}},
    null,
  ])('does not turn an invalid 4xx response into a confirmed write rejection: %j', async body => {
    const fetcher=vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify(body),{status:409}))
    await expect(createConfiguredBusinessClient('user','',undefined,fetcher).place(request,'original-write-key'))
      .rejects.toMatchObject({status:502,code:'INVALID_RESPONSE'})
    expect(fetcher).toHaveBeenCalledOnce()
    const [,init]=fetcher.mock.calls[0]!
    expect(new Headers(init?.headers).get('Idempotency-Key')).toBe('original-write-key')
    expect(JSON.parse(init!.body as string)).toEqual(request)
  })
  it('preserves a current structured rejection and rejects bare successful data', async () => {
    const error=()=>new Response(JSON.stringify({success:false,error:{code:'VERSION_CONFLICT',message:'Version changed'}}),{status:409})
    await expect(createApiClient({fetcher:vi.fn<typeof fetch>().mockResolvedValue(error())}).request('/operation'))
      .rejects.toMatchObject({status:409,code:'VERSION_CONFLICT',message:'Version changed'})
    await expect(createApiClient({fetcher:vi.fn<typeof fetch>().mockResolvedValue(new Response(JSON.stringify({id:'bare'})))}).request('/operation'))
      .rejects.toMatchObject({status:502,code:'INVALID_RESPONSE'})
  })
  it('keeps association metadata on the original data and rejects unreadable responses', async () => {
    const factory=(message:string,status:number,code:string)=>Object.assign(new Error(message),{status,code})
    const response=new Response(JSON.stringify({success:true,data:{id:'original'},associations:{'user:original':{name:'Member'}}}))
    await expect(readApiEnvelope(response,factory)).resolves.toMatchObject({data:{id:'original'}})
    await expect(readApiEnvelope(new Response('gateway failure',{status:503}),factory)).rejects.toMatchObject({status:502,code:'INVALID_RESPONSE'})
  })
})
