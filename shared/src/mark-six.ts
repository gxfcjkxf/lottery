import { readApiEnvelope } from './api-response'

import {markSixExpandedCatalog,markSixPositionAttributes} from './mark-six-expanded'

export const markSixWaves=['red','blue','green'] as const
export const markSixZodiac=['rat','ox','tiger','rabbit','dragon','snake','horse','goat','monkey','rooster','dog','pig'] as const
export const markSixElements=['metal','wood','water','fire','earth'] as const
const sequence=(n:number)=>Array.from({length:n},(_,i)=>i)
export const markSixSelectionKinds: Record<string,'number'|'zodiac'|'tail'>={
 ...Object.fromEntries(['linked_2','linked_3_2','linked_3','linked_4','linked_2_special','linked_special',...sequence(6).map(i=>`miss_${i+5}`)].map(code=>[code,'number' as const])),
 ...Object.fromEntries(['zodiac_pair',...sequence(5).map(i=>`zodiac_link_${i+1}`),...sequence(10).flatMap(i=>[`zodiac_hit_${i+2}`,`zodiac_miss_${i+2}`]),...sequence(5).map(i=>`zodiac_nohit_${i+1}`)].map(code=>[code,'zodiac' as const])),
 ...Object.fromEntries([...sequence(4).map(i=>`tail_link_${i+2}`),...sequence(5).map(i=>`tail_nohit_${i+1}`)].map(code=>[code,'tail' as const])),
}
export type MarkSixWave=typeof markSixWaves[number]
export interface MarkSixAttributes { id:string;year:number;version:number;effective_at:string;expires_at:string;waves:Record<string,MarkSixWave>;zodiac:Record<string,typeof markSixZodiac[number]>;elements:Record<string,typeof markSixElements[number]> }
export function markSixWave(number:number,waves:Record<string,MarkSixWave>):MarkSixWave {const wave=waves[String(number).padStart(2,'0')];if(!Number.isInteger(number)||number<1||number>49||!markSixWaves.includes(wave))throw new Error('Missing Mark Six number attribute');return wave}
export interface MarkSixItem { leg_odds?:string[];hit_count?:number; selection?:number[];odds_code?:string;odds_full?:string;winning_odds?:string; code:string; points:string; odds:string; outcome:''|'won'|'lost'|'push'; payout:string }
export interface MarkSixPurchase { plate:'A'|'B'; attributes:MarkSixAttributes; game_id:string; period_id:string; rule_version:number; sales_version:number; items:MarkSixItem[]; total_points:string }
export interface MarkSixOrder extends MarkSixPurchase { id:string; brand_id:string; member_id:string; period_no:string; prize_points:string; refund_points:string; effective_points:string; status:'placed'|'settled'; placed_at:string; settled_at:string|null }

export class MarkSixError extends Error { constructor(public code:string,public status:number){super(code)} }
export function createMarkSixClient(surface:'user'|'admin',brand:()=>string,brandCode?:string,fetcher:typeof fetch=fetch){
 const base=surface==='user'?`/api/v1${brandCode?`/b/${encodeURIComponent(brandCode)}`:''}`:`/api/v1/${surface}`
 async function request<T>(path:string):Promise<T>{
  const headers=new Headers({Accept:'application/json'});if(surface==='admin')headers.set('X-Brand-ID',brand())
  let response:Response;try{response=await fetcher(base+path,{credentials:'same-origin',method:'GET',headers})}catch{throw new MarkSixError('NETWORK_ERROR',0)}
  const value = await readApiEnvelope<T>(response, (_message,status,code)=>new MarkSixError(code,status))
  return value.data
 }
 return {
  allOrders:(offset=0)=>request<{items:MarkSixOrder[]}>(`/mark-six/orders?offset=${offset}&limit=50`),
  orders:(id:string,offset=0)=>request<{items:MarkSixOrder[]}>(surface==='user'?`/mark-six/games/${encodeURIComponent(id)}/orders?offset=${offset}&limit=50`:`/platform-games/${encodeURIComponent(id)}/mark-six/orders?offset=${offset}&limit=50`),
 }
}

export const markSixCatalog={
 en:{
 ...markSixExpandedCatalog.en,
 
 
 regularNumber:'Regular {number}',positionNumber:'Position {position} · {number}',headNumber:'Special head {number}',specialTail:'Special tail {number}',drawTail:'Draw tail {number}',totalZodiac:'{number} distinct zodiac signs',halfWave:'{wave} · {side}',sevenWave:'Seven-ball wave · {wave}',linked_2:'2 of 2 regular',linked_3_2:'2 of 3 regular',linked_3_2_full:'2 of 3 · all 3 hit',linked_3:'3 of 3 regular',zodiac_pair:'Special zodiac pair',domestic:'Domestic zodiac',wild:'Wild zodiac',fullOdds:'All-three-hit odds',winningOdds:'Applied winning odds',zodiac_link_1:'Any-ball zodiac',zodiac_link_2:'2-zodiac link',zodiac_link_3:'3-zodiac link',zodiac_link_4:'4-zodiac link',zodiac_link_5:'5-zodiac link',tail_link_2:'2-tail link',tail_link_2_zero:'2-tail link · includes zero',tail_link_3:'3-tail link',tail_link_3_zero:'3-tail link · includes zero',tail_link_4:'4-tail link',tail_link_4_zero:'4-tail link · includes zero',tail_link_5:'5-tail link',tail_link_5_zero:'5-tail link · includes zero',miss_5:'5 no-hit',miss_6:'6 no-hit',miss_7:'7 no-hit',miss_8:'8 no-hit',miss_9:'9 no-hit',miss_10:'10 no-hit',
 
  wave_red:'Red wave',wave_blue:'Blue wave',wave_green:'Green wave',zodiac_rat:'Rat',zodiac_ox:'Ox',zodiac_tiger:'Tiger',zodiac_rabbit:'Rabbit',zodiac_dragon:'Dragon',zodiac_snake:'Snake',zodiac_horse:'Horse',zodiac_goat:'Goat',zodiac_monkey:'Monkey',zodiac_rooster:'Rooster',zodiac_dog:'Dog',zodiac_pig:'Pig',element_metal:'Metal',element_wood:'Wood',element_water:'Water',element_fire:'Fire',element_earth:'Earth',
  wave:'Wave color',zodiac:'Zodiac',element:'Five elements',attributesMissing:'Number attributes are missing for this draw date. Publish them in the platform console before creating a period.',rat:'Rat',ox:'Ox',tiger:'Tiger',rabbit:'Rabbit',dragon:'Dragon',snake:'Snake',horse:'Horse',goat:'Goat',monkey:'Monkey',rooster:'Rooster',dog:'Dog',pig:'Pig',metal:'Metal',wood:'Wood',water:'Water',fire:'Fire',earth:'Earth',red:'Red wave',blue:'Blue wave',green:'Green wave',
  
  title:'Mark Six',   number:'Special number', sides:'Two-sided bets', big:'Big',small:'Small',odd:'Odd',even:'Even',big_odd:'Big odd',big_even:'Big even',small_odd:'Small odd',small_even:'Small even',
  odds:'Payout odds',stake:'Stake (points)', refresh:'Refresh',
  
  
  
  ticketDetails:'Ticket details',ticketMore:'+{count} more',ticketId:'Ticket ID',period:'Period',selected:'Selected items',total:'Total points',login:'Sign in to place bets',history:'Bet history',empty:'No tickets yet',placed:'Awaiting settlement',settled:'Settled',won:'Won',lost:'Lost',push:'Push',prize:'Winnings',refund:'Push refund',effective:'Effective stake',next:'Next',prev:'Previous',loading:'Loading…',invalid:'Check the stake, odds and limits.',changed:'The period or configuration changed. Refresh and review again.',failed:'The request failed. Please retry.',network:'Connection failed. Please retry.',denied:'You do not have permission for this operation.',insufficient:'Insufficient available points.',limit:'The bet exceeds the current limits.',
 },
 'zh-CN':{
 ...markSixExpandedCatalog['zh-CN'],
 
 
 regularNumber:'正码 {number}',positionNumber:'正{position}特 · {number}',headNumber:'特码{number}头',specialTail:'特码{number}尾',drawTail:'开奖{number}尾',totalZodiac:'总肖 · {number}肖',halfWave:'{wave} · {side}',sevenWave:'七色波 · {wave}',linked_2:'二全中',linked_3_2:'三中二',linked_3_2_full:'三中二 · 中三',linked_3:'三全中',zodiac_pair:'特码合肖',domestic:'家肖',wild:'野肖',fullOdds:'中三赔率',winningOdds:'中奖采用赔率',zodiac_link_1:'平特一肖',zodiac_link_2:'2连肖',zodiac_link_3:'3连肖',zodiac_link_4:'4连肖',zodiac_link_5:'5连肖',tail_link_2:'2连尾',tail_link_2_zero:'2连尾 · 含0尾',tail_link_3:'3连尾',tail_link_3_zero:'3连尾 · 含0尾',tail_link_4:'4连尾',tail_link_4_zero:'4连尾 · 含0尾',tail_link_5:'5连尾',tail_link_5_zero:'5连尾 · 含0尾',miss_5:'自选5不中',miss_6:'自选6不中',miss_7:'自选7不中',miss_8:'自选8不中',miss_9:'自选9不中',miss_10:'自选10不中',
 
  wave_red:'红波',wave_blue:'蓝波',wave_green:'绿波',zodiac_rat:'鼠',zodiac_ox:'牛',zodiac_tiger:'虎',zodiac_rabbit:'兔',zodiac_dragon:'龙',zodiac_snake:'蛇',zodiac_horse:'马',zodiac_goat:'羊',zodiac_monkey:'猴',zodiac_rooster:'鸡',zodiac_dog:'狗',zodiac_pig:'猪',element_metal:'金',element_wood:'木',element_water:'水',element_fire:'火',element_earth:'土',
  wave:'波色',zodiac:'生肖',element:'五行',attributesMissing:'开奖日期对应的号码属性尚未配置，请先在总后台发布，再创建期次。',rat:'鼠',ox:'牛',tiger:'虎',rabbit:'兔',dragon:'龙',snake:'蛇',horse:'马',goat:'羊',monkey:'猴',rooster:'鸡',dog:'狗',pig:'猪',metal:'金',wood:'木',water:'水',fire:'火',earth:'土',red:'红波',blue:'蓝波',green:'绿波',
  
  title:'六合彩',number:'特码',sides:'两面',big:'特大',small:'特小',odd:'特单',even:'特双',big_odd:'大单',big_even:'大双',small_odd:'小单',small_even:'小双',
  odds:'派彩赔率',stake:'投注积分',refresh:'刷新',
  
  
  
  ticketDetails:'注单详情',ticketMore:'另有 {count} 项投注',ticketId:'注单编号',period:'期号',selected:'已选项数',total:'投注总积分',login:'登录后投注',history:'投注记录',empty:'暂无注单',placed:'待结算',settled:'已结算',won:'中奖',lost:'未中奖',push:'和局',prize:'中奖派彩',refund:'和局退本',effective:'有效投注',next:'下一页',prev:'上一页',loading:'加载中…',invalid:'请检查投注积分、赔率和限额。',changed:'期次或配置已变化，请刷新后重新预览。',failed:'请求失败，请重试。',network:'连接失败，请重试。',denied:'无权执行此操作。',insufficient:'可用积分不足。',limit:'投注超过当前限额。',
 }
} as const
export function markSixErrorKey(error:unknown):'changed'|'network'|'denied'|'insufficient'|'limit'|'invalid'|'failed'|'attributesMissing'{
 if(!(error instanceof MarkSixError))return 'failed'
 if(error.code==='MARK_SIX_ATTRIBUTES_MISSING')return 'attributesMissing'
 if(error.status===0)return 'network'
 if(/INSUFFICIENT/.test(error.code))return 'insufficient'
 if(/LIMIT/.test(error.code))return 'limit'
 if(/VERSION|CLOSED|CONFLICT/.test(error.code))return 'changed'
 if(error.status===401||error.status===403)return 'denied'
 if(error.status===400)return 'invalid'
 return 'failed'
}

export function markSixLabel(code:string,t:(key:string,parameters?:Readonly<Record<string,string|number>>)=>string):string{
 if(code==='earth')return t('earthZodiac')
 if(code==='back')return t('backZodiac')
 if(code.startsWith('number_'))return code.slice(7)
 if(code.startsWith('regular_zodiac_'))return t('regularZodiacLabel',{zodiac:t(code.slice(15))})
 if(code.startsWith('position_attribute_')){const parts=code.slice(19).split('_');const p=parts.shift()!;return t('positionAttributeLabel',{position:p,attribute:markSixAttributeLabel(parts.join('_'),t)})}
 if(code.startsWith('quarter_')){const [,wave,...side]=code.split('_');return t('quarterWaveLabel',{wave:t(wave!),side:t(side.join('_'))})}
 if(code==='zodiac_pair')return t('zodiacCombinedLabel',{size:2,mode:t('hitMode')})
 if(/^zodiac_(hit|miss)_/.test(code)){const [,mode,size]=code.split('_');return t('zodiacCombinedLabel',{size:size!,mode:t(mode==='hit'?'hitMode':'missMode')})}
 if(code.startsWith('zodiac_nohit_'))return t('zodiacNoHitLabel',{size:code.slice(13)})
 if(code.startsWith('tail_nohit_'))return t('tailNoHitLabel',{size:code.slice(11)})
 if(code.startsWith('regular_'))return t('regularNumber',{number:code.slice(8)})
 if(code.startsWith('position_')){const [,position,number]=code.split('_');return t('positionNumber',{position:position!,number:number!})}
 if(code.startsWith('head_'))return t('headNumber',{number:code.slice(5)})
 if(code.startsWith('special_tail_'))return t('specialTail',{number:code.slice(13)})
 if(code.startsWith('draw_tail_'))return t('drawTail',{number:code.slice(10)})
 if(code.startsWith('zodiac_count_'))return t('totalZodiac',{number:code.slice(13)})
 if(code.startsWith('half_')){const [,wave,side]=code.split('_');return t('halfWave',{wave:t(wave!),side:t(side!)})}
 if(code.startsWith('seven_'))return t('sevenWave',{wave:t(code.slice(6)==='tie'?'push':code.slice(6))})
 return t(code)
}
export function markSixBallNumber(code:string):number|undefined{if(/^(number|regular)_\d{2}$/.test(code)||/^position_[1-6]_\d{2}$/.test(code))return Number(code.slice(-2));return undefined}
export function markSixSelectionLabel(code:string,selection:number[],t:(key:string,parameters?:Readonly<Record<string,string|number>>)=>string):string{
 if(code==='parlay')return markSixParlayCodes(selection).map(v=>markSixLabel(v,t)).join(' · ')
 const kind=markSixSelectionKinds[code];if(!kind)throw new Error('Unsupported Mark Six combination')
 return selection.map(n=>kind==='zodiac'?t(markSixZodiac[n-1]!):kind==='tail'?t('drawTail',{number:n}):String(n).padStart(2,'0')).join(' · ')
}

export function markSixParlayCodes(selection:number[]):string[]{
 if(selection.length<2||selection.length>6)throw new Error('Invalid Mark Six parlay')
 const positions=new Set<number>()
 return selection.map(value=>{const p=Math.floor(value/100),index=value%100;if(!Number.isInteger(value)||p<1||p>6||index<0||index>=markSixPositionAttributes.length||positions.has(p))throw new Error('Invalid Mark Six parlay');positions.add(p);return `position_attribute_${p}_${markSixPositionAttributes[index]}`})
}

export function markSixAttributeLabel(code:string,t:(key:string)=>string):string{return ['big','small','odd','even'].includes(code)?t(`attribute_${code}`):t(code)}
