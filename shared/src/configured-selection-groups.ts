import type {ConfiguredLabel,ConfiguredPlay,ConfiguredSnapshot,ConfiguredOption,ConfiguredRequest} from './configured-rules'
import {configuredRuleCatalog} from './configured-rule-catalog'

export interface ConfiguredSelectionGroup {id:string;label:ConfiguredLabel;plays:ConfiguredPlay[]}

// Presentation groups retain the original play and option identifiers for quotes.
export function configuredSelectionGroups(snapshot:ConfiguredSnapshot):ConfiguredSelectionGroup[]{
 const groupKey=(play:ConfiguredPlay)=>play.group==='tails'?'tailNumbers':play.group==='extremes'?'sides':play.group
 const members=new Map<string,ConfiguredPlay[]>()
 for(const play of snapshot.catalog.plays){const group=members.get(groupKey(play))??[];group.push(play);members.set(groupKey(play),group)}
 const result:ConfiguredSelectionGroup[]=[],seen=new Set<string>()
 for(const play of snapshot.catalog.plays){
  const group=members.get(groupKey(play))!,key=`group.${groupKey(play)}`
  const merge=group.length>1&&key in configuredRuleCatalog.en&&group.every(p=>configuredIndependentPlay(snapshot,p))
  if(!merge){result.push({id:`play:${play.id}`,label:play.label,plays:[play]});continue}
  if(seen.has(groupKey(play)))continue
  seen.add(groupKey(play))
  const en=configuredRuleCatalog.en as Record<string,string>,zh=configuredRuleCatalog['zh-CN'] as Record<string,string>
  const ordered=groupKey(play)==='tailNumbers'?[...group.filter(p=>p.group==='tailNumbers'),...group.filter(p=>p.group!=='tailNumbers')]:group
  result.push({id:`group:${groupKey(play)}`,label:{en:en[key]!,'zh-CN':zh[key]!},plays:ordered})
 }
 return result
}

export function configuredSelectionOptionLabel(play:ConfiguredPlay,option:ConfiguredOption):ConfiguredLabel{
 if(play.group==='tails'){
  const label=(locale:'en'|'zh-CN')=>{
   const text=option.label[locale],prefix=configuredRuleCatalog[locale].tailOptionPrefix
   return text.startsWith(prefix)?text.slice(prefix.length):text
  }
  return {en:label('en'),'zh-CN':label('zh-CN')}
 }
 const sum=play.group==='sums'&&option.source_code?.match(/^sum_([0-9]+)$/)
 return sum?{en:sum[1]!,'zh-CN':sum[1]!}:option.label
}

export function configuredIndependentPlay(snapshot:ConfiguredSnapshot,play:ConfiguredPlay):boolean{
 return play.stake_mode==='per_line'&&play.selection.mode==='singles'&&play.selection.min===1&&play.selection.max===1&&snapshot.catalog.domains[play.selection.domain]!.options.length===1
}

export function configuredSelectionRequests(snapshot:ConfiguredSnapshot,request:ConfiguredRequest,selectedPlays:readonly string[]):ConfiguredRequest[]{
 const requests=selectedPlays.map(id=>{
  const play=snapshot.catalog.plays.find(p=>p.id===id)
  if(!play||!configuredIndependentPlay(snapshot,play))throw new Error('Invalid independent selection')
  return {...request,play_id:id,options:[snapshot.catalog.domains[play.selection.domain]!.options[0]!.id],multiplier:'1'}
 })
 if(request.options.length)requests.push({...request,options:[...request.options]})
 return requests
}
