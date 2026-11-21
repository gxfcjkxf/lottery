import {configuredSelectionGroups,configuredSelectionGroupKey,configuredSelectionOptionLabel} from './configured-selection-groups'
import {configuredRuleCatalog} from './configured-rule-catalog'
import { readApiEnvelope } from './api-response'

import type { ConfiguredSnapshot, ConfiguredRequest, ConfiguredPreviewResult, ConfiguredQuotedLine, ConfiguredPlay, ConfiguredOption } from './configured-rules'
export interface ConfiguredCase { context?: { aggregates?: Record<string,string>; prize_remaining?: string }; requests: ConfiguredRequest[]; draw: {regular:number[];special:number[];digits:number[]}; bet_points:string;prize_points:string;refund_points:string }
export interface ConfiguredVersion { id:string;game_id:string;revision:number;snapshot:ConfiguredSnapshot;snapshot_hash:string;cases:ConfiguredCase[];status:string;validation:unknown }
export interface ConfiguredSales {version:number;rule_id:string;enabled:boolean;prices:ConfiguredSnapshot['prices'];min_points:string;max_points:string;max_period_points:string;max_user_period_points:string}
export type ConfiguredOptionStates = Record<string, Record<string, boolean>>
export type ConfiguredDisabledOptions = Record<string, readonly string[]>
export interface ConfiguredAvailabilityScope { version: number; items: ConfiguredOptionStates }
export interface ConfiguredAvailability { rule_id: string; platform: ConfiguredAvailabilityScope; brand: ConfiguredAvailabilityScope | null }
export function configuredItemState(items: ConfiguredOptionStates | undefined, play: string, option: string): boolean | undefined {
 if (!items || !Object.hasOwn(items, play)) return undefined
 return Object.hasOwn(items[play]!, option) ? items[play]![option] : undefined
}
export function configuredAvailabilityDisabled(availability: ConfiguredAvailability, play: string, option: string): boolean {
 return configuredItemState(availability.platform.items, play, option) === false ||
   configuredItemState(availability.brand?.items, play, option) === false
}
export function configuredOptionsForScope(snapshot: ConfiguredSnapshot, play: ConfiguredPlay, availability: ConfiguredAvailability) {
 const options = snapshot.catalog.domains[play.selection.domain]!.options
 return availability.brand ? options.filter(option => configuredItemState(availability.platform.items, play.id, option.id) !== false) : options
}
export function configuredGroupsForScope(snapshot: ConfiguredSnapshot, availability: ConfiguredAvailability) {
 return configuredSelectionGroups(snapshot).filter(group =>
   group.plays.some(play => configuredOptionsForScope(snapshot, play, availability).length > 0))
}
export function configuredOptionDisabled(disabled: ConfiguredDisabledOptions | undefined, play: string, option: string): boolean {
 return disabled !== undefined && Object.hasOwn(disabled, play) && disabled[play]!.includes(option)
}
export function configuredPlayAvailable(snapshot: ConfiguredSnapshot, play: ConfiguredPlay, disabled: ConfiguredDisabledOptions | undefined): boolean {
 return snapshot.catalog.domains[play.selection.domain]!.options.some(option => !configuredOptionDisabled(disabled, play.id, option.id))
}
export function configuredAvailableSelectionGroups(snapshot: ConfiguredSnapshot, disabled: ConfiguredDisabledOptions | undefined) {
 return configuredSelectionGroups(snapshot).filter(group => group.plays.some(play => configuredPlayAvailable(snapshot, play, disabled)))
}
export interface ConfiguredLiveInput {period_id:string;rule_id:string;sales_version:number;snapshot_hash:string;actor_context:string;requests:ConfiguredRequest[]}
export interface ConfiguredLiveQuote extends NonNullable<ConfiguredPreviewResult['quote']>{game_id:string;period_id:string;rule_id:string;sales_version:number;definition_hash:string;snapshot_hash:string;actor_context:string}
export interface ConfiguredOrderDisplaySnapshot {
 catalog: {
  funding?: 'reserve' | null
  plays: Array<Pick<ConfiguredPlay,'id'|'group'|'label'|'selection'|'stake_mode'>>
  domains: Record<string, { options: Array<Pick<ConfiguredOption,'id'|'label'|'source_code'>> }>
 }
}
export type ConfiguredOrderSummary = Omit<ConfiguredLiveOrder, 'snapshot'> & { snapshot: ConfiguredOrderDisplaySnapshot }
export interface ConfiguredLiveOrder {game_name:string;id:string;member_name:string;game_id:string;period_id:string;period_no:string;rule_id:string;sales_version:number;snapshot:ConfiguredSnapshot;quote:NonNullable<ConfiguredPreviewResult['quote']>;result:ConfiguredPreviewResult['result']|null;status:string;placed_at:string;settled_at:string|null}
export interface ConfiguredLiveCatalog {game:{id:string;name:string;status:string;brand_enabled:boolean};snapshot:ConfiguredSnapshot;rule_id:string;sales:ConfiguredSales;snapshot_hash:string;disabled_options?:ConfiguredDisabledOptions;previous_draw:{period_no:string;result:{regular:number[];special:number[];digits:number[]};regular_order:number[]|null;attributes:Record<string,{wave:string;zodiac:string;element:string}>}|null;period:{id:string;period_no:string;bet_start_at:string;bet_end_at:string;draw_at:string}|null}
export function createConfiguredBusinessClient(surface:'platform'|'admin'|'user',brandId:string,brandCode?:string,fetcher:typeof fetch=fetch){
 const base=surface==='user'?`/api/v1${brandCode?`/b/${encodeURIComponent(brandCode)}`:''}/configured`:`/api/v1/${surface}/platform-games`
 async function request<T>(path:string,body?:unknown,key?:string):Promise<T>{
  const headers=new Headers({Accept:'application/json'});if(surface==='admin')headers.set('X-Brand-ID',brandId);if(body!==undefined)headers.set('Content-Type','application/json');if(key)headers.set('Idempotency-Key',key)
  let response:Response;try{response=await fetcher(base+path,{credentials:'same-origin',method:body===undefined?'GET':'POST',headers,...(body===undefined?{}:{body:JSON.stringify(body)})})}catch{throw Object.assign(new Error('Network failure'),{code:'NETWORK_ERROR',status:0})}
  const value = await readApiEnvelope<T>(response,(message,status,code)=>Object.assign(new Error(message),{status,code}));return value.data

 }
 return {request,
  games:()=>request<{games:Array<{id:string;name:string}>}>('/games'),catalog:(id:string)=>request<ConfiguredLiveCatalog>(`/games/${encodeURIComponent(id)}`),
  allOrders:(offset=0)=>request<{items:ConfiguredOrderSummary[]}>(`/orders?offset=${offset}`),
  orders:(id:string,offset=0)=>request<{items:ConfiguredOrderSummary[]}>(`/games/${encodeURIComponent(id)}/orders?offset=${offset}`),
  preview:(input:ConfiguredLiveInput)=>request<ConfiguredLiveQuote>('/preview',input),place:(input:ConfiguredLiveInput,key:string)=>request<{items:ConfiguredLiveOrder[]}>('/orders',input,key),
  rules:(id:string)=>request<ConfiguredVersion|null>(`/${encodeURIComponent(id)}/configured/rules`),
  availability:(id:string)=>request<ConfiguredAvailability>(`/${encodeURIComponent(id)}/configured/availability`),
  sales:(id:string)=>request<ConfiguredSales>(`/${encodeURIComponent(id)}/configured/sales`),
  adminOrders:(id:string,offset=0)=>request<{items:ConfiguredOrderSummary[]}>(`/${encodeURIComponent(id)}/configured/orders?offset=${offset}`),
 }
}
export function configuredTicketLabel(snapshot:ConfiguredOrderDisplaySnapshot,line:{play_id:string;options:string[]},locale:'en'|'zh-CN'){
 const play=snapshot.catalog.plays.find(p=>p.id===line.play_id)!
 const domain=snapshot.catalog.domains[play.selection.domain]!
 const key=`group.${configuredSelectionGroupKey(play.group)}`
 const grouped=play.stake_mode==='per_line'&&play.selection.mode==='singles'&&play.selection.min===1&&play.selection.max===1&&domain.options.length===1&&key in configuredRuleCatalog.en
 const title=grouped?(configuredRuleCatalog[locale] as Record<string,string>)[key]!:play.label[locale]
 return `${title} · ${line.options.map(id=>configuredSelectionOptionLabel(play,domain.options.find(o=>o.id===id)!)[locale]).join(' / ')}`
}
export function configuredTicketPrices(snapshot:ConfiguredOrderDisplaySnapshot,line:ConfiguredQuotedLine,locale:'en'|'zh-CN'):string[]{
 return [...Object.values(line.prices),...(line.legs??[]).flatMap(leg=>configuredTicketPrices(snapshot,leg,locale).map(price=>`${configuredTicketLabel(snapshot,leg,locale)}: ${price}`))]
}
