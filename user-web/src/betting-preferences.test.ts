import {describe,expect,it} from 'vitest'
import {createQuickAmountsApi,readBettingPreferences,writeBettingPreferences} from './betting-preferences'
function storage(){const values=new Map<string,string>();return {getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value)}}}
describe('account betting preferences',()=>{
 it('remembers only the last input locally, scoped by brand and member',()=>{
  const store=storage()
  expect(readBettingPreferences(store,'brand','member')).toEqual({lastPoints:''})
  writeBettingPreferences(store,'brand','member',{lastPoints:'75'})
  expect(readBettingPreferences(store,'brand','member')).toEqual({lastPoints:'75'})
  expect(readBettingPreferences(store,'other','member').lastPoints).toBe('')
  expect(readBettingPreferences(store,'brand','other').lastPoints).toBe('')
 })
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
