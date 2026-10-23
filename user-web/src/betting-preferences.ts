import {createApiClient,type ApiClientOptions} from '../../shared/src/api'
export interface BettingPreferences { lastPoints: string }
export const bettingPreferencesKey = (brandId: string, memberId: string) => `lottery:betting-preferences:${brandId}:${memberId}`

export function validBetAmount(value: string): boolean {
  return /^[1-9]\d*$/.test(value) && BigInt(value) <= 9223372036854775807n
}
export function validQuickAmount(value:string):boolean{return validBetAmount(value)&&value.length<=6}
function validate(value: unknown): asserts value is BettingPreferences {
  const prefs = value as BettingPreferences | null
  if (!prefs || typeof prefs.lastPoints !== 'string' || (prefs.lastPoints !== '' && !validBetAmount(prefs.lastPoints))) throw new Error('Invalid betting preferences')
}
export function readBettingPreferences(storage: Pick<Storage,'getItem'>, brandId: string, memberId: string): BettingPreferences {
  const raw = storage.getItem(bettingPreferencesKey(brandId, memberId))
  if (raw === null) return {lastPoints: ''}
  const value: unknown = JSON.parse(raw)
  validate(value)
  return value
}
export function writeBettingPreferences(storage: Pick<Storage,'setItem'>, brandId: string, memberId: string, value: BettingPreferences) {
  validate(value)
  storage.setItem(bettingPreferencesKey(brandId, memberId), JSON.stringify(value))
}

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
