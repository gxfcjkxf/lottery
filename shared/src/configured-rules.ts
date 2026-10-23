import { readApiEnvelope } from './api-response'
import type { RuleModel } from './rules'

export type RuleNode = null | boolean | number | string | RuleNode[] | { [key: string]: RuleNode }
export interface ConfiguredLabel { en: string; 'zh-CN': string }
export interface ConfiguredMapping { kind: string; numbers?: number[]; attribute?: string; labels?: string[]; children?: ConfiguredMapping[]; transform?: string; parameter?: number; value?: number }
export interface ConfiguredOption { id: string; label: ConfiguredLabel; mapping?: ConfiguredMapping; value?: number; reference?: { play_id: string; option_id: string }; slot?: string; tags?: string[]; price_key?: string; source_code?: string }
export interface ConfiguredPlay {
  id: string; group: string; label: ConfiguredLabel; source_code?: string
  selection: { domain: string; mode: string; per_line: number; min: number; max: number; distinct_slot: boolean }
  stake_mode: string; unit_points?: string; max_multiplier: string; settlement: string
  risk_multiplier?: number
  decisions?: { index:number[]; outcomes: Array<{ outcome:string; price_key?: string; effective:boolean; prize_factor?:string; loss_factor:number }>; alternate?: { aggregate:string; at_least:string; index:number[] } }
  push_when?: RuleNode; tiers?: RuleNode[]; mixed_tier_policy: string; rounding: string; rounding_scope: string; cap_points?: string
}
export interface ConfiguredCatalog {
  funding?: 'reserve'; period_prize_cap?: string; aggregates?: Record<string,string[]>
  schema_version: number; semantics_version: number; model: RuleModel; disjoint_draw_groups: boolean
  max_lines: number; price_min: string; price_max: string; plates: string[]
  attributes: Array<{ id: string; labels: string[]; partition: boolean; fixed?: Record<string, string> }>
  domains: Record<string, { options: ConfiguredOption[] }>; plays: ConfiguredPlay[]
}
export interface ConfiguredSnapshot {
  catalog: ConfiguredCatalog
  bindings: { version: string; attributes: Record<string, Record<string, number[]>> }
  prices: { version: string; plates: Record<string, Record<string, string>> }
}
export interface ConfiguredRequest { play_id: string; options: string[]; plate: string; points: string; multiplier: string }
export interface ConfiguredQuotedLine { play_id:string;options:string[];plate:string;stake:string;prices:Record<string,string>;legs?:ConfiguredQuotedLine[] }
export interface ConfiguredPreviewResult {
  definition_hash: string; snapshot_hash: string; price_keys: string[]
  quote?: { reserved_points?:string; bet_points: string; lines: ConfiguredQuotedLine[] }
  result?: { consumed_points?:string; bet_points: string; prize_points: string; refund_points: string; effective_points: string; lines: Array<{ play_id: string; options: string[]; outcome: string; prize_points: string; refund_points: string; tiers?: RuleNode[]; legs?: RuleNode[] }>; rounding: RuleNode[] }
}

export function createConfiguredPreviewClient(surface: 'platform' | 'admin', brandId: string, fetcher: typeof fetch = fetch) {
  async function request<T>(path: string, body?: unknown): Promise<T> {
    const headers = new Headers({ Accept: 'application/json' })
    if (surface === 'admin') headers.set('X-Brand-ID', brandId)
    if (body !== undefined) headers.set('Content-Type', 'application/json')
    let response: Response
    try { response = await fetcher(`/api/v1/${surface}/platform-games/rule-preview${path}`, { method: body === undefined ? 'GET' : 'POST', headers, credentials: 'same-origin', ...(body === undefined ? {} : { body: JSON.stringify(body) }) }) }
    catch { throw Object.assign(new Error('Rule preview network failure'), { code: 'NETWORK_ERROR', status: 0 }) }
    const value = await readApiEnvelope<T>(response,(message,status,code)=>Object.assign(new Error(message),{status,code}))
    return value.data as T
  }
  return {
    template: (name: string) => request<ConfiguredCatalog>(`/template?template=${encodeURIComponent(name)}`),
    preview: (snapshot: ConfiguredSnapshot, requests: ConfiguredRequest[] = [], draw?: { regular: number[]; special: number[]; digits: number[] }) => request<ConfiguredPreviewResult>('', { snapshot, requests, ...(draw ? { draw } : {}) }),
  }
}

/** Explicit synthetic inputs for the sandbox; never used by sales or as missing-price defaults. */
export function configuredDemo(catalog: ConfiguredCatalog): ConfiguredSnapshot {
  const attributes: ConfiguredSnapshot['bindings']['attributes'] = {}
  for (const a of catalog.attributes) {
    const labels: Record<string, number[]> = Object.fromEntries(a.labels.map(label => [label, []]))
    const pool = catalog.model.regular_pool
    const values = pool.values ?? Array.from({ length: pool.max! - pool.min! + 1 }, (_, i) => i + pool.min!)
    for (const [i, n] of values.entries()) labels[a.fixed?.[String(n)] ?? a.labels[i % a.labels.length]!]!.push(n)
    attributes[a.id] = labels
  }
  const keys = new Set<string>()
  const collect = (node: unknown) => {
    if (Array.isArray(node)) { node.forEach(collect); return }
    if (!node || typeof node !== 'object') return
    for (const [key, value] of Object.entries(node)) {
      if ((key === 'price_key' || key === 'key') && typeof value === 'string') keys.add(value)
      else collect(value)
    }
  }
  for (const play of catalog.plays) { collect(play.tiers); collect(play.decisions); collect(catalog.domains[play.selection.domain]) }
  return { catalog: JSON.parse(JSON.stringify(catalog)) as ConfiguredCatalog, bindings: { version: 'synthetic-attributes', attributes }, prices: { version: 'synthetic-prices', plates: Object.fromEntries(catalog.plates.map(plate => [plate, Object.fromEntries([...keys].map(key => [key, '2']))])) } }
}

export function configuredNumbers(snapshot: ConfiguredSnapshot, option: ConfiguredOption): number[] {
  if (option.reference) {
    const play = snapshot.catalog.plays.find(p => p.id === option.reference!.play_id)
    const leg = play && snapshot.catalog.domains[play.selection.domain]?.options.find(o => o.id === option.reference!.option_id)
    if (!leg || leg.reference) throw new Error('Invalid leg reference')
    return configuredNumbers(snapshot, leg)
  }
  if (!option.mapping) return []
  const matches = (m: ConfiguredMapping, n: number): boolean => {
    switch (m.kind) {
      case 'numbers': return m.numbers!.includes(n)
      case 'attribute': return m.labels!.some(label => snapshot.bindings.attributes[m.attribute!]![label]!.includes(n))
      case 'all': return m.children!.every(child => matches(child, n))
      case 'transform': {
        const value = m.transform === 'modulo' ? n % m.parameter! : m.transform === 'quotient' ? Math.trunc(n / m.parameter!) : m.transform === 'digit_sum' ? [...String(n)].reduce((sum, digit) => sum + Number(digit), 0) : NaN
        if (Number.isNaN(value)) throw new Error('Unsupported transform')
        return value === m.value
      }
      default: throw new Error('Unsupported mapping')
    }
  }
  const pools = [snapshot.catalog.model.regular_pool, snapshot.catalog.model.special_pool]
  const numbers = new Set(pools.flatMap(pool => pool.values ?? Array.from({ length: pool.max! - pool.min! + 1 }, (_, i) => i + pool.min!)))
  return [...numbers].sort((a, b) => a - b).filter(n => matches(option.mapping!, n))
}

/** Local stake display follows the engine's entry expansion; submission still uses its quote. */
export function configuredBetPoints(play: ConfiguredPlay, request: ConfiguredRequest): string | null {
  const count = request.options.length
  if (count < play.selection.min) return '0'
  const amount = play.stake_mode === 'per_line' ? request.points : request.multiplier
  if (!/^[0-9]+$/.test(amount) || BigInt(amount) <= 0n) return null
  let stake: bigint
  if (play.stake_mode === 'per_line') stake = BigInt(amount)
  else if (play.stake_mode === 'unit_multiplier') {
    if (!play.unit_points) throw new Error('Missing configured unit points')
    stake = BigInt(play.unit_points) * BigInt(amount)
  } else throw new Error('Unsupported configured stake mode')
  let entries: bigint
  switch (play.selection.mode) {
    case 'singles': entries = BigInt(count); break
    case 'as_selected': entries = 1n; break
    case 'combinations': {
      entries = 1n
      const size = Math.min(play.selection.per_line, count - play.selection.per_line)
      for (let index = 1; index <= size; index++) entries = entries * BigInt(count - size + index) / BigInt(index)
      break
    }
    default: throw new Error('Unsupported configured selection mode')
  }
  return (entries * stake).toString()
}

export type ConfiguredWave = 'red' | 'blue' | 'green'
const configuredWaves: ConfiguredWave[] = ['red', 'blue', 'green']

/** Wave bindings belong to the published rule, independently of the brand theme. */
export function configuredNumberWave(snapshot: ConfiguredSnapshot, number: number): ConfiguredWave | undefined {
  const waves = snapshot.bindings.attributes.wave
  if (!waves) return undefined
  const matches = configuredWaves.filter(wave => waves[wave]?.includes(number))
  if (matches.length !== 1) throw new Error('Invalid configured wave binding')
  return matches[0]
}

export function configuredOptionWave(snapshot: ConfiguredSnapshot, option: ConfiguredOption): ConfiguredWave | undefined {
  if (!snapshot.bindings.attributes.wave) return undefined
  const waves = new Set(configuredNumbers(snapshot, option).map(number => configuredNumberWave(snapshot, number)))
  return waves.size === 1 ? [...waves][0] : undefined
}

/** Display potential tier prices only; the engine determines the actual price of each expanded entry. */
export function configuredOptionPrices(snapshot: ConfiguredSnapshot, play: ConfiguredPlay, option: ConfiguredOption, plate: string): string[] {
  if (option.reference) {
    const target = snapshot.catalog.plays.find(p => p.id === option.reference!.play_id)
    const leg = target && snapshot.catalog.domains[target.selection.domain]?.options.find(o => o.id === option.reference!.option_id)
    if (!target || !leg || leg.reference) throw new Error('Invalid leg reference')
    return configuredOptionPrices(snapshot, target, leg, plate)
  }
  const values = new Set<string>()
  if (play.settlement === 'decision_table') {
    for (const key of new Set((play.decisions?.outcomes ?? []).flatMap(outcome => outcome.price_key ? [outcome.price_key] : []))) {
      const price = snapshot.prices.plates[plate]?.[key]
      if (!price) throw new Error('Missing price')
      values.add(price)
    }
    return [...values]
  }
  if (play.settlement !== 'tiers' || !play.tiers?.length) throw new Error('Missing tier configuration')
  for (const raw of play.tiers) {
    const tier = raw as { price: { kind: string; key?: string; cases?: Array<{ key: string }> } }
    const p = tier.price
    const keys = p.kind === 'fixed' ? [p.key!] : p.kind === 'option' ? [option.price_key!] : p.kind === 'cases' ? p.cases!.map(c => c.key) : []
    if (!keys.length) throw new Error('Unsupported price')
    for (const key of keys) { const price = snapshot.prices.plates[plate]?.[key]; if (!price) throw new Error('Missing price'); values.add(price) }
  }
  return [...values]
}

/** Create an independent copy, including its option domain; editing it cannot alter the source play. */
export function cloneConfiguredPlay(snapshot: ConfiguredSnapshot, play: ConfiguredPlay, id: string): ConfiguredPlay {
  if (!/^[a-z][a-z0-9_]{0,63}$/.test(id) || snapshot.catalog.plays.some(p => p.id === id) || snapshot.catalog.domains[id]) throw new Error('Invalid or duplicate play ID')
  const copy = JSON.parse(JSON.stringify(play)) as ConfiguredPlay
  copy.id = id
  delete copy.source_code
  copy.selection.domain = id
  const domain = JSON.parse(JSON.stringify(snapshot.catalog.domains[play.selection.domain]!)) as { options: ConfiguredOption[] }
  domain.options.forEach(option => { delete option.source_code })
  snapshot.catalog.domains[id] = domain
  snapshot.catalog.plays.push(copy)
  return copy
}

export interface NumberGroupDraft {
  id: string; label: ConfiguredLabel; numbers: number[]; target: 'all' | 'regular' | 'special'
  position?: number; pushNumber?: number; prices: Record<string, string>
}

/** A visual shortcut producing ordinary schema-v2 components, with no special runtime handling. */
export function addConfiguredNumberGroup(snapshot: ConfiguredSnapshot, input: NumberGroupDraft): ConfiguredPlay {
  const { id } = input
  const key = `${id}_price`
  if (!/^[a-z][a-z0-9_]{0,63}$/.test(id) || snapshot.catalog.plays.some(p => p.id === id) || snapshot.catalog.domains[id] || !input.label.en.trim() || !input.label['zh-CN'].trim() || !input.numbers.length) throw new Error('Invalid number group')
  const count = input.target === 'regular' ? snapshot.catalog.model.regular_count : input.target === 'special' ? snapshot.catalog.model.special_count : snapshot.catalog.model.regular_count + snapshot.catalog.model.special_count
  if (input.position !== undefined && (!Number.isInteger(input.position) || input.position < 0 || input.position >= count)) throw new Error('Invalid position')
  if (input.pushNumber !== undefined && input.position === undefined && count !== 1) throw new Error('Push requires one draw position')
  for (const plate of snapshot.catalog.plates) if (!input.prices[plate]?.trim() || key in snapshot.prices.plates[plate]!) throw new Error('Missing or duplicate price')
  const scope = { target: input.target, ...(input.position === undefined ? {} : { positions: [input.position] }) }
  const play: ConfiguredPlay = { id, group: id, label: input.label, selection: { domain: id, mode: 'singles', per_line: 1, min: 1, max: 1, distinct_slot: false }, stake_mode: 'per_line', max_multiplier: '1', settlement: 'tiers', tiers: [{ id: 'hit', when: { op: 'at_least', left: { op: 'match_count', scope, counting: 'occurrences' }, right: { op: 'constant', value: 1 } }, price: { kind: 'option' }, payout: { kind: 'fixed_return' }, exclusive: true }], mixed_tier_policy: '', rounding: 'half_up', rounding_scope: 'line' }
  if (input.pushNumber !== undefined) play.push_when = { op: 'equals', left: { op: 'raw_value', scope }, right: { op: 'constant', value: input.pushNumber } }
  snapshot.catalog.domains[id] = { options: [{ id: 'choice', label: input.label, mapping: { kind: 'numbers', numbers: input.numbers }, price_key: key }] }
  snapshot.catalog.plays.push(play)
  for (const plate of snapshot.catalog.plates) snapshot.prices.plates[plate]![key] = input.prices[plate]!
  return play
}

export function parseConfiguredSnapshot(raw: string): ConfiguredSnapshot {
  const value = JSON.parse(raw)
  if (value?.catalog?.schema_version !== 2 || value?.catalog?.semantics_version !== 1 || !Array.isArray(value.catalog.plays) || !value.catalog.domains || !value.bindings?.attributes || !value.prices?.plates) throw new Error('Invalid snapshot')
  return value as ConfiguredSnapshot // Full semantic validation remains authoritative on the server.
}
