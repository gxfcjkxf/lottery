import {configuredSelectionGroups} from '../../shared/src/configured-selection-groups'
import { readFileSync, readdirSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { configuredDemo } from '../../shared/src/configured-rules'
import { describePlayRules,describePlayGroup } from './play-rules'
const folder = new URL('../../backend/internal/rules/config/', import.meta.url)
describe('player play instructions', () => {
  for (const file of readdirSync(folder).filter(file => file.endsWith('.json'))) {
    it(`describes every published play in ${file} in both languages`, () => {
      const snapshot = configuredDemo(JSON.parse(readFileSync(new URL(file, folder), 'utf8')))
      for (const play of snapshot.catalog.plays) for (const locale of ['en','zh-CN'] as const) {
        const description = describePlayRules(locale, {snapshot,play,plate:snapshot.catalog.plates[0]!})
        expect(description.instructions.length, play.id).toBeGreaterThan(1)
        expect(description.prizes.length, play.id).toBeGreaterThan(0)
        expect(JSON.stringify(description)).not.toMatch(/schema_version|decision_table|price_key|definition_hash|undefined/)
      }
    })
  }
  it('explains special-number matching and stake-return exceptions from the current rule', () => {
    const snapshot=configuredDemo(JSON.parse(readFileSync(new URL('mark_six_v2.json', folder),'utf8')))
    const play=snapshot.catalog.plays.find(play=>play.id==='special_sides')!
    const rules=describePlayRules('zh-CN',{snapshot,play,plate:'A'})
    expect(rules.prizes[0]!.text).toContain('特别号码为 49')
    expect(rules.prizes[1]!.text).toContain('特别号码中至少有一个号码落在所选号码或分类范围内')
    const changed=structuredClone(play);(changed.tiers![0] as any).when.right.value=2
    expect(describePlayRules('zh-CN',{snapshot,play:changed,plate:'A'}).prizes[1]!.text).toContain('不少于 2')
  })
  it('explains standard digit betting without backend field names', () => {
    const definition = {schema_version:1,model:{model:'DIGITS_0_9',length:3},selection:{mode:'numbers'},unit_points:'2',prize_tiers:[{code:'internal_exact',odds:'90',condition:{op:'equals',field:'position_match',value:3}}],rounding:'half_up',rounding_scope:'order',limits:{}} as any
    const zh=describePlayRules('zh-CN',{definition})
    expect(zh.instructions[0]).toContain('每一位选择 0–9')
    expect(zh.prizes[0]!.text).toContain('与开奖数字按位置相符的个数为 3')
    expect(zh.prizes[0]!.detail).toBe('× 90')
    expect(JSON.stringify(zh)).not.toContain('internal_exact')
  })
  it('rejects an unsupported condition without exposing its raw definition', () => {
    const snapshot=configuredDemo(JSON.parse(readFileSync(new URL('mark_six_v2.json',folder),'utf8')))
    const play=structuredClone(snapshot.catalog.plays[0]!);(play.tiers![0] as any).when={op:'unknown'}
    expect(()=>describePlayRules('en',{snapshot,play,plate:'A'})).toThrow('Unsupported player rule condition')
  })
})

describe('grouped player instructions',()=>{
 it('covers every member of the four presentation groups with the displayed labels and current odds',()=>{
  const snapshot=configuredDemo(JSON.parse(readFileSync(new URL('canada_28.json',folder),'utf8')))
  const groups=configuredSelectionGroups(snapshot),before=JSON.stringify(snapshot)
  for(const group of groups)for(const locale of ['en','zh-CN'] as const){
   const description=describePlayGroup(locale,snapshot,group,'A')
   expect(description.entries.map(entry=>entry.id)).toEqual(group.plays.map(play=>play.id))
   expect(description.entries.every(entry=>entry.rules.prizes.length>0&&entry.prices.length>0)).toBe(true)
  }
  const sums=describePlayGroup('zh-CN',snapshot,groups.find(group=>group.id==='group:sums')!,'A')
  expect(sums.entries.map(entry=>entry.label)).toEqual(Array.from({length:28},(_,i)=>String(i)))
  const tails=describePlayGroup('zh-CN',snapshot,groups.find(group=>group.id==='group:tailNumbers')!,'A')
  expect(tails.entries.map(entry=>entry.label)).toEqual(['1尾','2尾','3尾','4尾','5尾','6尾','7尾','8尾','大','小','单','双','大单','大双','小单','小双'])
  expect(tails.notes).toEqual(['总和尾数为 0, 9 时，本组所有选项均不中奖。'])
  expect(JSON.stringify(snapshot)).toBe(before)
 })
 it('derives the shared tail exception from the actual rule instead of fixing it in the wording',()=>{
  const snapshot=configuredDemo(JSON.parse(readFileSync(new URL('canada_28.json',folder),'utf8')))
  const tail=snapshot.catalog.plays.find(play=>play.id==='tail_1')!
  tail.decisions!.index[0]=tail.decisions!.index[1]!
  const group=configuredSelectionGroups(snapshot).find(group=>group.id==='group:tailNumbers')!
  expect(describePlayGroup('en',snapshot,group,'A').notes).toEqual(['When the sum ends in 9, none of the options in this group wins.'])
 })
})
