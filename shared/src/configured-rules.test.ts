import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { shallowReactive } from 'vue'
import { addConfiguredNumberGroup, cloneConfiguredPlay, configuredDemo, configuredBetPoints, configuredNumberWave, configuredOptionWave, configuredNumbers, configuredOptionPrices, createConfiguredPreviewClient, parseConfiguredSnapshot, type ConfiguredCatalog } from './configured-rules'
import { configuredRuleCatalog } from './configured-rule-catalog'
import { createCatalogI18n } from './localization'

const catalog = (): ConfiguredCatalog => JSON.parse(readFileSync(new URL('../../backend/internal/rules/config/mark_six_v2.json', import.meta.url), 'utf8'))

describe('configured stake display', () => {
  const request = {play_id:'test', options:['1','2','3'], plate:'A', points:'15', multiplier:'1'}
  it('totals independent entries, expanded combinations and a single combined entry', () => {
    const single = catalog().plays[0]!
    expect(configuredBetPoints(single, request)).toBe('45')
    expect(configuredBetPoints(single, {...request, options:[]})).toBe('0')
    const combination = {...single, selection:{...single.selection,mode:'combinations',per_line:3,min:3}}
    const five = {...request, options:['1','2','3','4','5'], points:'7'}
    expect(configuredBetPoints(combination, five)).toBe('70')
    expect(configuredBetPoints(combination, {...five,options:['1','2']})).toBe('0')
    const combined = {...single, selection:{...single.selection,mode:'as_selected',per_line:0,min:1}}
    expect(configuredBetPoints(combined, five)).toBe('7')
    expect(configuredBetPoints({...combination,stake_mode:'unit_multiplier',unit_points:'4'}, {...five,points:'0',multiplier:'3'})).toBe('120')
  })
  it('keeps integer point precision and leaves unfinished inputs unpriced', () => {
    const single = catalog().plays[0]!
    expect(configuredBetPoints(single, {...request,points:'9007199254740993'})).toBe('27021597764222979')
    for (const points of ['', '0', '-1', '1.5']) expect(configuredBetPoints(single, {...request,points})).toBeNull()
  })
})

describe('configured wave presentation', () => {
  it('uses rule bindings for numbered options and single-wave groups', () => {
    const snapshot = configuredDemo(catalog())
    const play = snapshot.catalog.plays[0]!
    const number = snapshot.catalog.domains[play.selection.domain]!.options[0]!
    expect(configuredNumberWave(snapshot, 1)).toBe('red')
    expect(configuredOptionWave(snapshot, number)).toBe('red')
    for (const wave of ['red', 'blue', 'green']) {
      const option = snapshot.catalog.domains.special_waves!.options.find(option => option.id === wave)!
      expect(configuredOptionWave(snapshot, option)).toBe(wave)
    }
    expect(configuredOptionWave(snapshot, {id:'mixed', label:{en:'Mixed','zh-CN':'混合'}, mapping:{kind:'numbers',numbers:[1,2,3]}})).toBeUndefined()
    snapshot.bindings.attributes.wave!.red = snapshot.bindings.attributes.wave!.red!.filter(number => number !== 1)
    expect(() => configuredNumberWave(snapshot, 1)).toThrow('Invalid configured wave binding')
  })
  it('leaves games without wave attributes uncolored', () => {
    const snapshot = configuredDemo(catalog())
    delete snapshot.bindings.attributes.wave
    expect(configuredNumberWave(snapshot, 1)).toBeUndefined()
    expect(configuredOptionWave(snapshot, snapshot.catalog.domains.special_waves!.options[0]!)).toBeUndefined()
  })
})

describe('configured draft preview', () => {
  it('creates a grouped option with position, push and separate A/B prices using the visual shortcut', () => {
    const snapshot = configuredDemo(catalog())
    const input = { id:'new_group', label:{en:'Lucky group','zh-CN':'幸运组'}, numbers:[2,7,19,31], target:'regular' as const, position:2, pushNumber:49, prices:{A:'2.5',B:'3.5'} }
    const before = JSON.stringify(snapshot)
    expect(() => addConfiguredNumberGroup(snapshot,{...input,prices:{A:'2.5'}})).toThrow()
    expect(JSON.stringify(snapshot)).toBe(before)
    const play = addConfiguredNumberGroup(snapshot,input)
    const option = snapshot.catalog.domains.new_group!.options[0]!
    expect(configuredNumbers(snapshot,option)).toEqual([2,7,19,31])
    expect(play.selection).toMatchObject({mode:'singles',per_line:1,max:1})
    expect(play.push_when).toMatchObject({left:{scope:{target:'regular',positions:[2]}},right:{value:49}})
    expect(configuredOptionPrices(snapshot,play,option,'A')).toEqual(['2.5'])
    expect(configuredOptionPrices(snapshot,play,option,'B')).toEqual(['3.5'])
    expect(() => addConfiguredNumberGroup(snapshot,{...input,id:'another',position:undefined})).toThrow()
  })
  it('shows prices for decision-table plays used by configured Canada games', () => {
    const snapshot = configuredDemo(catalog())
    const play = snapshot.catalog.plays[0]!
    const option = snapshot.catalog.domains[play.selection.domain]!.options[0]!
    play.settlement = 'decision_table'
    play.decisions = { index:[0], outcomes: [{outcome:'lost',effective:true,loss_factor:1}, {outcome:'won',effective:true,loss_factor:0,price_key:'decision_price'}] }
    snapshot.prices.plates.A!.decision_price = '2.25'
    expect(configuredOptionPrices(snapshot, play, option, 'A')).toEqual(['2.25'])
    play.decisions = { index:[0], outcomes: [{outcome:'lost',effective:true,loss_factor:1}] }
    expect(configuredOptionPrices(snapshot, play, option, 'A')).toEqual([])
  })
  it('builds an explicit complete synthetic snapshot and displays every configured option', () => {
    const snapshot = configuredDemo(catalog())
    expect(snapshot.catalog.plays).toHaveLength(87)
    expect(Object.keys(snapshot.prices.plates.A!)).toHaveLength(669)
    expect(Object.keys(snapshot.prices.plates.B!)).toHaveLength(669)
    expect(new Set(Object.values(snapshot.prices.plates.A!))).toEqual(new Set(['2']))
    expect(snapshot.bindings.attributes.wave!.green).toContain(49)
    for (const a of snapshot.catalog.attributes) {
      expect(Object.values(snapshot.bindings.attributes[a.id]!).flat().sort((a,b) => a-b)).toEqual(Array.from({ length: 49 }, (_,i) => i+1))
    }
    for (const play of snapshot.catalog.plays) for (const option of snapshot.catalog.domains[play.selection.domain]!.options) {
      expect(option.label.en.length).toBeGreaterThan(0)
      expect(option.label['zh-CN'].length).toBeGreaterThan(0)
      const numbers = configuredNumbers(snapshot, option)
      expect(numbers.every(n => n >= 1 && n <= 49)).toBe(true)
      if (option.mapping || option.reference) expect(numbers.length).toBeGreaterThan(0)
    }
  })

  it('adds a previously unknown group by configuration, with an independent domain and portable file', () => {
    const snapshot = configuredDemo(catalog())
    const source = shallowReactive(snapshot.catalog.plays[0]!)
    const copied = cloneConfiguredPlay(snapshot, source, 'operator_lucky_group')
    copied.label = { en: 'Lucky group', 'zh-CN': '幸运组' }
    const option = snapshot.catalog.domains[copied.selection.domain]!.options[0]!
    option.label = copied.label
    option.mapping = { kind: 'numbers', numbers: [2,7,19,31] }
    expect(configuredNumbers(snapshot, option)).toEqual([2,7,19,31])
    expect(configuredNumbers(snapshot, snapshot.catalog.domains[source.selection.domain]!.options[0]!)).toEqual([1])
    expect(copied.source_code).toBeUndefined()
    expect(snapshot.catalog.domains[copied.selection.domain]!.options.every(o => !o.source_code)).toBe(true)
    expect(parseConfiguredSnapshot(JSON.stringify(snapshot))).toEqual(JSON.parse(JSON.stringify(snapshot)))
    expect(() => cloneConfiguredPlay(snapshot, source, copied.id)).toThrow()
    expect(() => parseConfiguredSnapshot('{"catalog":{"schema_version":1}}')).toThrow()
  })

  it('projects intersections, attribute sets, transforms and leg references from the same configuration', () => {
    const snapshot = configuredDemo(catalog())
    const option = { id:'custom', label:{en:'Custom','zh-CN':'自定义'}, mapping:{kind:'all',children:[{kind:'numbers',numbers:[2,7,19,31]},{kind:'transform',transform:'modulo',parameter:2,value:1}] } }
    expect(configuredNumbers(snapshot, option)).toEqual([7,19,31])
    option.mapping = {kind:'all',children:[{kind:'transform',transform:'quotient',parameter:10,value:4}]} as typeof option.mapping
    expect(configuredNumbers(snapshot, option)).toEqual([40,41,42,43,44,45,46,47,48,49])
    const parlay = snapshot.catalog.plays.find(p => p.settlement === 'product_of_legs')!
    const leg = snapshot.catalog.domains[parlay.selection.domain]!.options[0]!
    const target = snapshot.catalog.plays.find(p => p.id === leg.reference!.play_id)!
    expect(configuredNumbers(snapshot, leg)).toEqual(configuredNumbers(snapshot, snapshot.catalog.domains[target.selection.domain]!.options.find(o => o.id === leg.reference!.option_id)!))
  })

  it('sends only the explicit draft and exact amount strings to the authenticated preview API', async () => {
    const snapshot = configuredDemo(catalog())
    const calls: Array<{url:string;init:RequestInit}> = []
    const fetcher = (async (url: string | URL | Request, init?: RequestInit) => {
      calls.push({url:String(url),init:init!})
      return new Response(JSON.stringify({success:true,data:{definition_hash:'definition',snapshot_hash:'snapshot',price_keys:[]}}),{status:200})
    }) as typeof fetch
    const api = createConfiguredPreviewClient('admin', 'brand-one', fetcher)
    await api.preview(snapshot,[{play_id:'custom',options:['choice'],plate:'A',points:'9007199254740993',multiplier:'1'}])
    expect(calls[0]!.url).toBe('/api/v1/admin/platform-games/rule-preview')
    expect(new Headers(calls[0]!.init.headers).get('X-Brand-ID')).toBe('brand-one')
    expect(new Headers(calls[0]!.init.headers).has('Idempotency-Key')).toBe(false)
    const body = JSON.parse(calls[0]!.init.body as string)
    expect(body.snapshot).toEqual(snapshot)
    expect(body.requests[0].points).toBe('9007199254740993')
    expect(body.draw).toBeUndefined()
    const denied = createConfiguredPreviewClient('platform','', (async()=>new Response(JSON.stringify({success:false,error:{code:'PERMISSION_DENIED',message:'Permission denied'}}),{status:403})) as typeof fetch)
    await expect(denied.template('mark_six')).rejects.toMatchObject({status:403,code:'PERMISSION_DENIED'})
  })

  it('provides the same complete editor catalog for both locales', () => {
    const i18n = createCatalogI18n(configuredRuleCatalog, 'en')
    expect(i18n.t('editorTitle')).toBe('Configurable rules and publication')
    i18n.setLocale('zh-CN')
    expect(i18n.t('editorTitle')).toBe('配置式规则与发布')
  })
})
