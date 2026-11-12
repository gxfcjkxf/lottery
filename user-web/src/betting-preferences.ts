import { isPoints, pointsMinor } from '@lottery/shared'
import {createApiClient,type ApiClientOptions} from '../../shared/src/api'
export function validBetAmount(value: string): boolean {
  return isPoints(value) && pointsMinor(value) > 0n
}
export function validQuickAmount(value:string):boolean{return /^[1-9]\d{0,5}$/.test(value)}
export function createQuickAmountsApi(options:ApiClientOptions={}) {
 const client=createApiClient(options)
 const path=(options.brandCode?`/api/v1/b/${encodeURIComponent(options.brandCode)}`:'/api/v1')+'/me/betting-preferences'
 async function request(brandId:string,memberId:string,init?:RequestInit):Promise<string[]> {
  const value=await client.request<{brand_id:string;member_id:string;quick_amounts:string[]}>(path,init)
  if(!value||value.brand_id!==brandId||value.member_id!==memberId||!Array.isArray(value.quick_amounts)||value.quick_amounts.length>5||!value.quick_amounts.every(amount=>typeof amount==='string'&&validQuickAmount(amount)))throw new Error('Invalid quick stake settings')
  return value.quick_amounts
 }
 return {
  read:(brandId:string,memberId:string)=>request(brandId,memberId),
  save:(brandId:string,memberId:string,amounts:string[])=>{
   if(amounts.length>5||!amounts.every(validQuickAmount))return Promise.reject(new Error('Invalid quick stake settings'))
   return request(brandId,memberId,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({quick_amounts:amounts})})
  }
 }
}
