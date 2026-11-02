import { archiveScheduleFields,archiveScheduleOf,sameArchiveSchedule,validArchiveSchedule,validArchiveNextRuns,type ArchiveSchedule,type ArchiveNextRuns } from '@lottery/shared';
import {PlatformApiError,readPlatformEnvelope} from './platform-api';
export interface PlatformArchivePolicy extends ArchiveSchedule {version:number;daily_enabled:boolean;monthly_enabled:boolean;timezone:string;next_runs:ArchiveNextRuns;updated_at:string}
export type PlatformArchiveInput=ArchiveSchedule & {version:number;daily_enabled:boolean;monthly_enabled:boolean};
export interface PendingPlatformArchive {actor:string;key:string;body:Readonly<PlatformArchiveInput>;conflict:boolean}
const intents=new Map<string,PendingPlatformArchive>();
export function getPlatformArchiveIntent(actor:string){return intents.get(actor)??null}
export function setPlatformArchiveIntent(intent:PendingPlatformArchive){intents.set(intent.actor,intent)}
export function clearPlatformArchiveIntents(){intents.clear()}
export function discardPlatformArchiveIntent(actor:string,intent:PendingPlatformArchive){if(intents.get(actor)===intent)intents.delete(actor)}
const path='/api/v1/platform/report-archive-policy';
const record=(value:unknown):value is Record<string,unknown>=>!!value && typeof value==='object' && !Array.isArray(value);
function valid(value:unknown):value is PlatformArchivePolicy {
 if(!record(value)||Object.keys(value).sort().join(',')!==[...archiveScheduleFields,'version','daily_enabled','monthly_enabled','timezone','next_runs','updated_at'].sort().join(','))return false;
 if(!validArchiveSchedule(value)||!validArchiveNextRuns(value.next_runs)||!Number.isSafeInteger(value.version)||Number(value.version)<1||typeof value.daily_enabled!=='boolean'||typeof value.monthly_enabled!=='boolean'||typeof value.timezone!=='string'||!value.timezone||typeof value.updated_at!=='string'||!Number.isFinite(Date.parse(value.updated_at)))return false;
 try{new Intl.DateTimeFormat('en',{timeZone:value.timezone})}catch{return false}return true;
}
export function createPlatformArchivePolicyApi(fetcher:typeof fetch=fetch){
 async function request(input?:PlatformArchiveInput,key?:string,actor?:string):Promise<PlatformArchivePolicy>{
  const headers=new Headers({Accept:'application/json'});let body:PlatformArchiveInput|undefined;
  if(input){
   if(!validArchiveSchedule(input)||!Number.isSafeInteger(input.version)||input.version<1||input.version>=Number.MAX_SAFE_INTEGER||typeof input.daily_enabled!=='boolean'||typeof input.monthly_enabled!=='boolean'||!key||!actor)throw new PlatformApiError('Invalid archive configuration',400,'INVALID_INPUT');
   body=Object.freeze({...archiveScheduleOf(input),version:input.version,daily_enabled:input.daily_enabled,monthly_enabled:input.monthly_enabled});headers.set('Content-Type','application/json');headers.set('Idempotency-Key',key);headers.set('X-Report-Archive-Actor-ID',actor);
  }
  let response:Response;
  try{response=await fetcher(path,{method:body?'PUT':'GET',credentials:'same-origin',headers,...(body?{body:JSON.stringify(body)}:{})});}
  catch{throw new PlatformApiError('Network request failed',0,body?'UNKNOWN_WRITE_STATUS':'NETWORK_ERROR')}
  if(body && response.status>=500)throw new PlatformApiError('Archive configuration result is unknown',response.status,'UNKNOWN_WRITE_STATUS');
  const envelope=await readPlatformEnvelope<unknown>(response);
  if(response.status!==200||!valid(envelope.data)||body && (envelope.data.version!==body.version+1||!sameArchiveSchedule(envelope.data,body)||envelope.data.daily_enabled!==body.daily_enabled||envelope.data.monthly_enabled!==body.monthly_enabled))throw new PlatformApiError('Invalid archive configuration response',502,body?'UNKNOWN_WRITE_STATUS':'INVALID_RESPONSE');
  return envelope.data;
 }
 return {read:()=>request(),update:(body:PlatformArchiveInput,key:string,actor:string)=>request(body,key,actor)};
}
