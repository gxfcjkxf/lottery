import { readFileSync, readdirSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { configuredDemo } from '../../shared/src/configured-rules'
import { describePlayRules } from './play-rules'
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
