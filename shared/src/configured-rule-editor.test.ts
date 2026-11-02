import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { bindingErrors, bindingText, configuredPriceRows } from './configured-rule-editor'
import { configuredDemo, type ConfiguredCatalog } from './configured-rules'

const sample = () => configuredDemo(JSON.parse(readFileSync(new URL('../../backend/internal/rules/config/mark_six_v2.json', import.meta.url), 'utf8')) as ConfiguredCatalog)

describe('flat rule editing', () => {
  it('validates complete partitions, syntax, duplicates and pool membership', () => {
    const snapshot = sample(), texts = bindingText(snapshot)
    expect(bindingErrors(snapshot.catalog, texts)).toEqual({ zodiac: {}, wave: {}, element: {} })
    texts.zodiac!.rat = '1,1,50'
    expect(bindingErrors(snapshot.catalog, texts).zodiac).toMatchObject({ rat: 'bindingOutsidePool', '': 'bindingIncomplete' })
    texts.zodiac!.rat = '1,1'
    expect(bindingErrors(snapshot.catalog, texts).zodiac!.rat).toBe('bindingDuplicate')
    texts.zodiac!.rat = '1, nope'
    expect(bindingErrors(snapshot.catalog, texts).zodiac!.rat).toBe('bindingFormat')
    texts.zodiac!.rat = texts.zodiac!.ox!
    expect(bindingErrors(snapshot.catalog, texts).zodiac).toMatchObject({ rat: 'bindingOverlap', ox: 'bindingOverlap' })
  })
  it('allows editable wave assignments while retaining the individual fixed assignment', () => {
    const snapshot = sample(), texts = bindingText(snapshot)
    const red = snapshot.bindings.attributes.wave!.red!, blue = snapshot.bindings.attributes.wave!.blue!
    texts.wave!.red = [...red.slice(1), blue[0]!].join(', ')
    texts.wave!.blue = [...blue.slice(1), red[0]!].join(', ')
    expect(bindingErrors(snapshot.catalog, texts).wave).toEqual({})
    texts.wave!.green = snapshot.bindings.attributes.wave!.green!.filter(n => n !== 49).join(', ')
    expect(bindingErrors(snapshot.catalog, texts).wave!.green).toBe('bindingFixed')
  })
  it('deduplicates shared prices without losing stored keys or changing their values', () => {
    const snapshot = sample(), before = JSON.stringify(snapshot)
    snapshot.catalog.plays.push({ ...snapshot.catalog.plays[0]!, id: 'copied_play' })
    snapshot.prices.plates.B!.extra_price = '3.5'
    const rows = configuredPriceRows(snapshot, 'zh-CN')
    expect(new Set(rows.map(row => row.id)).size).toBe(rows.length)
    expect(rows.map(row => row.id).sort()).toEqual([...new Set(Object.values(snapshot.prices.plates).flatMap(Object.keys))].sort())
    expect(rows.find(row => row.id === 'extra_price')!.label).toBe('extra_price')
    expect(JSON.parse(before).prices.plates.A).toEqual(snapshot.prices.plates.A)
  })
})
