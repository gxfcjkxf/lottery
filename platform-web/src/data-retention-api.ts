import {PlatformApiError,readPlatformEnvelope} from './platform-api'

export interface RetentionInput {version:number;enabled:boolean;execution_day:number;execution_time:string}
export interface RetentionPolicy extends RetentionInput {timezone:string;next_run:{execute_at:string;cutoff_at:string}|null;updated_at:string}
export interface RetentionCounts {deleted:string;receipts:string;deferred:string}
export interface RetentionTask {id:string;brand_id:string;brand_name:string;policy_version:number;timezone:string;execute_at:string;cutoff_at:string;state:'pending'|'completed'|'deferred'|'failed';counts:Record<string,RetentionCounts>;error_code:string;created_at:string;finished_at:string|null}
export interface RetentionPage {items:RetentionTask[];total_count:string;limit:number;offset:number}
export type RetentionIntent = {actor:string;key:string;conflict:boolean} & ({kind:'policy';body:Readonly<RetentionInput>}|{kind:'retry';id:string})
const intents=new Map<string,RetentionIntent>()
export const getRetentionIntent=(actor:string)=>intents.get(actor)??null
export const setRetentionIntent=(intent:RetentionIntent)=>intents.set(intent.actor,intent)
export const clearRetentionIntents=()=>intents.clear()
export function discardRetentionIntent(intent:RetentionIntent){if(intents.get(intent.actor)===intent)intents.delete(intent.actor)}
const obj=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)
const date=(v:unknown):v is string=>typeof v==='string'&&Number.isFinite(Date.parse(v))
const count=(v:unknown):v is string=>typeof v==='string'&&/^(0|[1-9]\d*)$/.test(v)
const id=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v)
export const retentionFamilies=['configured_orders','cash_orders','recharge_orders','withdrawal_orders','cash_entries','point_ledger_entries','audit_logs','notifications','notification_records','operational_records','temporary_records','request_receipts'] as const
export function validRetentionInput(v:unknown):v is RetentionInput{return obj(v)&&Number.isSafeInteger(v.version)&&Number(v.version)>0&&Number(v.version)<Number.MAX_SAFE_INTEGER&&typeof v.enabled==='boolean'&&Number.isInteger(v.execution_day)&&Number(v.execution_day)>=1&&Number(v.execution_day)<=28&&typeof v.execution_time==='string'&&/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(v.execution_time)}
function policy(v:unknown):v is RetentionPolicy{
 if(!validRetentionInput(v)||!obj(v)||typeof v.timezone!=='string'||!date(v.updated_at)||Object.keys(v).sort().join(',')!=='enabled,execution_day,execution_time,next_run,timezone,updated_at,version')return false
 try{new Intl.DateTimeFormat('en',{timeZone:v.timezone})}catch{return false}
 return v.enabled?obj(v.next_run)&&date(v.next_run.execute_at)&&date(v.next_run.cutoff_at)&&Object.keys(v.next_run).length===2:v.next_run===null
}
function task(v:unknown):v is RetentionTask{
 if(!obj(v)||!id(v.id)||(v.brand_id!==''&&!id(v.brand_id))||typeof v.brand_name!=='string'||!Number.isSafeInteger(v.policy_version)||typeof v.timezone!=='string'||!date(v.execute_at)||!date(v.cutoff_at)||!date(v.created_at)||v.finished_at!==null&&!date(v.finished_at)||typeof v.error_code!=='string'||!['pending','completed','deferred','failed'].includes(String(v.state))||!obj(v.counts))return false
 return Object.entries(v.counts).every(([k,c])=>retentionFamilies.includes(k as typeof retentionFamilies[number])&&obj(c)&&Object.keys(c).sort().join(',')==='deferred,deleted,receipts'&&count(c.deleted)&&count(c.receipts)&&count(c.deferred))
}
export function createDataRetentionApi(fetcher:typeof fetch=fetch){
 async function request(path:string,method:'GET'|'PUT'|'POST'='GET',body?:unknown,intent?:RetentionIntent):Promise<unknown>{
  const headers=new Headers({Accept:'application/json'});if(body!==undefined)headers.set('Content-Type','application/json')
  if(intent){headers.set('Idempotency-Key',intent.key);headers.set('X-Data-Retention-Actor-ID',intent.actor)}
  let response:Response
  try{response=await fetcher('/api/v1/platform'+path,{method,credentials:'same-origin',headers,...(body!==undefined?{body:JSON.stringify(body)}:{})})}catch{throw new PlatformApiError('Network request failed',0,intent?'UNKNOWN_WRITE_STATUS':'NETWORK_ERROR')}
  if(intent&&response.status>=500)throw new PlatformApiError('Write result is unknown',response.status,'UNKNOWN_WRITE_STATUS')
  const result=await readPlatformEnvelope<unknown>(response)
  if(response.status!==200)throw new PlatformApiError('Invalid retention response',502,intent?'UNKNOWN_WRITE_STATUS':'INVALID_RESPONSE')
  return result.data
 }
 return {
  async policy(){const v=await request('/data-retention-policy');if(!policy(v))throw new PlatformApiError('Invalid retention policy',502,'INVALID_RESPONSE');return v},
  async write(intent:RetentionIntent){
   if(intent.kind==='policy'){
    if(!validRetentionInput(intent.body))throw new PlatformApiError('Invalid retention policy',400,'INVALID_INPUT')
    const v=await request('/data-retention-policy','PUT',intent.body,intent)
    if(!policy(v)||v.version!==intent.body.version+1||v.enabled!==intent.body.enabled||v.execution_day!==intent.body.execution_day||v.execution_time!==intent.body.execution_time)throw new PlatformApiError('Invalid retention receipt',502,'UNKNOWN_WRITE_STATUS')
    return v
   }
   const v=await request(`/data-retention-tasks/${encodeURIComponent(intent.id)}/retry`,'POST',{},intent)
   if(!obj(v)||v.id!==intent.id||Object.keys(v).length!==1)throw new PlatformApiError('Invalid retry receipt',502,'UNKNOWN_WRITE_STATUS')
   return null
  },
  async tasks(brand:string,limit:number,offset:number){
   const query=new URLSearchParams({limit:String(limit),offset:String(offset)});if(brand)query.set('brand_id',brand)
   const v=await request('/data-retention-tasks?'+query)
   if(!obj(v)||Object.keys(v).sort().join(',')!=='items,limit,offset,total_count'||!Array.isArray(v.items)||!v.items.every(task)||new Set(v.items.map(t=>t.id)).size!==v.items.length||!count(v.total_count)||v.limit!==limit||v.offset!==offset||v.items.length>limit||brand&&v.items.some(t=>t.brand_id!==brand))throw new PlatformApiError('Invalid retention tasks',502,'INVALID_RESPONSE')
   return v as unknown as RetentionPage
  },
 }
}
