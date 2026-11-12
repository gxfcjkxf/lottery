import {describe,expect,it} from 'vitest'
import {createQuickAmountsApi} from './betting-preferences'
describe('account betting preferences',()=>{
 it('saves exact quick amounts on the server and reads them from a separate client',async()=>{
  let values:string[]=[];const paths:string[]=[]
  const fetcher:typeof fetch=async(url,init)=>{
   paths.push(String(url));if(init?.method==='PUT')values=JSON.parse(String(init.body)).quick_amounts
   return new Response(JSON.stringify({success:true,data:{brand_id:'brand',member_id:'member',quick_amounts:values}}),{status:200})
  }
  const amounts=['5','50','999999']
  await createQuickAmountsApi({brandCode:'trial',fetcher}).save('brand','member',amounts)
  expect(await createQuickAmountsApi({brandCode:'trial',fetcher}).read('brand','member')).toEqual(amounts)
  expect(paths).toEqual(['/api/v1/b/trial/me/betting-preferences','/api/v1/b/trial/me/betting-preferences'])
  await expect(createQuickAmountsApi({fetcher}).read('other','member')).rejects.toThrow()
  await expect(createQuickAmountsApi({fetcher}).save('brand','member',['0'])).rejects.toThrow()
  await expect(createQuickAmountsApi({fetcher}).save('brand','member',['1000000'])).rejects.toThrow()
  await expect(createQuickAmountsApi({fetcher}).save('brand','member',['1','2','3','4','5','6'])).rejects.toThrow()
 })
 it('reports a server failure instead of accepting a browser copy',async()=>{
  const fetcher:typeof fetch=async()=>new Response(JSON.stringify({success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}),{status:503})
  await expect(createQuickAmountsApi({fetcher}).read('brand','member')).rejects.toThrow()
 })
})

it('accepts decimal per-line points and preserves integral quick-chip settings', async () => {
  const {validBetAmount,validQuickAmount}=await import('./betting-preferences')
  expect(validBetAmount('0.25')).toBe(true)
  expect(validBetAmount('9007199254740993.25')).toBe(true)
  expect(validQuickAmount('0.25')).toBe(false)
  for(const value of ['0','1.001','01.25','-0.25','92233720368547758.08']) expect(validBetAmount(value)).toBe(false)
})
