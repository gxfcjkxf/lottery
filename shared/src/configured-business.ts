import { readApiEnvelope } from './api-response'

import type { ConfiguredSnapshot, ConfiguredRequest, ConfiguredPreviewResult, ConfiguredQuotedLine } from './configured-rules'
export interface ConfiguredCase { context?: { aggregates?: Record<string,string>; prize_remaining?: string }; requests: ConfiguredRequest[]; draw: {regular:number[];special:number[];digits:number[]}; bet_points:string;prize_points:string;refund_points:string }
export interface ConfiguredVersion { id:string;game_id:string;revision:number;snapshot:ConfiguredSnapshot;snapshot_hash:string;cases:ConfiguredCase[];status:string;validation:unknown }
export interface ConfiguredSales {version:number;rule_id:string;enabled:boolean;prices:ConfiguredSnapshot['prices'];min_points:string;max_points:string;max_period_points:string;max_user_period_points:string}
export interface ConfiguredLiveInput {period_id:string;rule_id:string;sales_version:number;snapshot_hash:string;actor_context:string;requests:ConfiguredRequest[]}
export interface ConfiguredLiveQuote extends NonNullable<ConfiguredPreviewResult['quote']>{game_id:string;period_id:string;rule_id:string;sales_version:number;definition_hash:string;snapshot_hash:string;actor_context:string}
export interface ConfiguredLiveOrder {game_name:string;id:string;member_name:string;game_id:string;period_id:string;period_no:string;rule_id:string;sales_version:number;snapshot:ConfiguredSnapshot;quote:NonNullable<ConfiguredPreviewResult['quote']>;result:ConfiguredPreviewResult['result']|null;status:string;placed_at:string;settled_at:string|null}
export interface ConfiguredLiveCatalog {game:{id:string;name:string;status:string;brand_enabled:boolean};snapshot:ConfiguredSnapshot;rule_id:string;sales:ConfiguredSales;snapshot_hash:string;period:{id:string;period_no:string;bet_start_at:string;bet_end_at:string;draw_at:string}|null}
export function createConfiguredBusinessClient(surface:'platform'|'admin'|'user',brandId:string,brandCode?:string,fetcher:typeof fetch=fetch){
 const base=surface==='user'?`/api/v1${brandCode?`/b/${encodeURIComponent(brandCode)}`:''}/configured`:`/api/v1/${surface}/platform-games`
 async function request<T>(path:string,body?:unknown,key?:string):Promise<T>{
  const headers=new Headers({Accept:'application/json'});if(surface==='admin')headers.set('X-Brand-ID',brandId);if(body!==undefined)headers.set('Content-Type','application/json');if(key)headers.set('Idempotency-Key',key)
  let response:Response;try{response=await fetcher(base+path,{credentials:'same-origin',method:body===undefined?'GET':'POST',headers,...(body===undefined?{}:{body:JSON.stringify(body)})})}catch{throw Object.assign(new Error('Network failure'),{code:'NETWORK_ERROR',status:0})}
  const value = await readApiEnvelope<T>(response,(message,status,code)=>Object.assign(new Error(message),{status,code}));return value.data

 }
 return {request,
  games:()=>request<{games:Array<{id:string;name:string}>}>('/games'),catalog:(id:string)=>request<ConfiguredLiveCatalog>(`/games/${encodeURIComponent(id)}`),
  allOrders:(offset=0)=>request<{items:ConfiguredLiveOrder[]}>(`/orders?offset=${offset}`),
  orders:(id:string,offset=0)=>request<{items:ConfiguredLiveOrder[]}>(`/games/${encodeURIComponent(id)}/orders?offset=${offset}`),
  preview:(input:ConfiguredLiveInput)=>request<ConfiguredLiveQuote>('/preview',input),place:(input:ConfiguredLiveInput,key:string)=>request<ConfiguredLiveOrder>('/orders',input,key),
  versions:(id:string)=>request<{items:ConfiguredVersion[]}>(`/${encodeURIComponent(id)}/configured/versions`),
  sales:(id:string)=>request<ConfiguredSales>(`/${encodeURIComponent(id)}/configured/sales`),
  adminOrders:(id:string,offset=0)=>request<{items:ConfiguredLiveOrder[]}>(`/${encodeURIComponent(id)}/configured/orders?offset=${offset}`),
 }
}
export function configuredTicketLabel(snapshot:ConfiguredSnapshot,line:{play_id:string;options:string[]},locale:'en'|'zh-CN'){
 const play=snapshot.catalog.plays.find(p=>p.id===line.play_id)!
 return `${play.label[locale]} · ${line.options.map(id=>snapshot.catalog.domains[play.selection.domain]!.options.find(o=>o.id===id)!.label[locale]).join(' / ')}`
}
export function configuredTicketPrices(snapshot:ConfiguredSnapshot,line:ConfiguredQuotedLine,locale:'en'|'zh-CN'):string[]{
 return [...Object.values(line.prices),...(line.legs??[]).flatMap(leg=>configuredTicketPrices(snapshot,leg,locale).map(price=>`${configuredTicketLabel(snapshot,leg,locale)}: ${price}`))]
}
