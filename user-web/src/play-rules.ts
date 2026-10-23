import { createCatalogI18n, configuredNumbers, configuredOptionPrices, type ConfiguredPlay, type ConfiguredSnapshot } from '@lottery/shared'
import type { RuleCondition, RuleDefinition } from '../../shared/src/rules'
import { playRulesCatalog } from './play-rules-catalog'

type Node = Record<string, any>
export interface PlayerRule { text: string; detail?: string }
export interface PlayerRules { instructions: string[]; prizes: PlayerRule[]; options: PlayerRule[] }
export function describePlayRules(locale: 'en' | 'zh-CN', input: { snapshot: ConfiguredSnapshot; play: ConfiguredPlay; plate: string } | { definition: RuleDefinition }): PlayerRules {
  const { t } = createCatalogI18n(playRulesCatalog, locale)
  const text = (key: string, params: Record<string, string | number> = {}) => {
    if (!(key in playRulesCatalog.en)) throw new Error('Unsupported player rule description')
    return t(key, params)
  }
  const options = 'snapshot' in input ? input.snapshot.catalog.domains[input.play.selection.domain]!.options : []
  const scope = (s: Node) => {
    const label = text(s.target)
    return s.positions?.length ? text('scopePositions', { scope: label, positions: s.positions.map((p: number) => p + 1).join(', ') }) : label
  }
  const expression = (n: Node): string => {
    switch (n.op) {
      case 'constant': return String(n.value)
      case 'all': case 'any': return n.children.map((child: Node) => `(${expression(child)})`).join(text(n.op === 'all' ? 'and' : 'or'))
      case 'not': return text('not', { value: expression(n.children[0]) })
      case 'equals': case 'at_least': case 'at_most': case 'greater_than': {
        if (n.op === 'at_least' && n.left.op === 'match_count' && n.right.op === 'constant' && n.right.value === 1) return text('hitAny', { scope: scope(n.left.scope) })
        if (n.op === 'equals' && ['selected_contains','selected_tag','selected_is_unique_max'].includes(n.left.op) && n.right.op === 'constant' && [0,1].includes(n.right.value)) return n.right.value === 1 ? expression(n.left) : text('not', { value: expression(n.left) })
        return text(n.op, { left: expression(n.left), right: expression(n.right) })
      }
      case 'match_count': return text('matched', { scope: scope(n.scope) })
      case 'covered_option_count': return text('covered', { scope: scope(n.scope) })
      case 'numbers': case 'raw_value': return scope(n.scope)
      case 'selected_value': return text('selectedValue')
      case 'selected_count': return text('selectedCount')
      case 'selected_contains': {
        const option = options.find(o => o.id === n.option_id)
        if (!option) throw new Error('Unknown option')
        return text('contains', { option: option.label[locale] })
      }
      case 'selected_tag': return text('tag', { options: options.filter(o => o.tags?.includes(n.tag)).map(o => o.label[locale]).join(', ') })
      case 'sum': case 'distinct_count': case 'digit_sum': case 'max_tie_count': case 'selected_is_unique_max': return text(({ sum: 'sum', distinct_count: 'distinct', digit_sum: 'digitSum', max_tie_count: 'ties', selected_is_unique_max: 'uniqueMax' } as Record<string, string>)[n.op]!, { value: expression(n.input) })
      case 'modulo': case 'quotient': return text(n.op, { value: expression(n.input), number: n.parameter })
      case 'attribute_values': return text('attribute', { scope: scope(n.scope), attribute: text(n.attribute) })
      case 'weighted_group_sum': return text('weighted', { attribute: text(n.attribute), regular: n.weights.regular, special: n.weights.special })
      case 'between': return text('between', { left: expression(n.left), min: n.min, max: n.max })
      case 'in': return text('in', { left: expression(n.left), values: n.values.join(', ') })
      default: throw new Error('Unsupported player rule condition')
    }
  }
  const standardCondition = (c: RuleCondition): string => {
    if (['all', 'any'].includes(c.op)) return c.children!.map(child => `(${standardCondition(child)})`).join(text(c.op === 'all' ? 'and' : 'or'))
    if (c.op === 'not') return text('not', { value: standardCondition(c.children![0]!) })
    const left = text(c.field!, { scope: text(c.target || 'all'), position: (c.position ?? 0) + 1 })
    if (c.op === 'selected') return text('selected', { left })
    if (c.op === 'in') return text('in', { left, values: c.values!.join(', ') })
    if (c.op === 'between') return text('between', { left, min: c.min!, max: c.max! })
    if (c.op === 'equals') return text('equals', { left, right: c.value! })
    throw new Error('Unsupported player condition')
  }
  if ('definition' in input) {
    const d = input.definition
    const mode = d.selection.mode
    const how = mode === 'numbers' ? d.model.model === 'DIGITS_0_9' ? text('standardDigits') : text('standardNumbers', { regular: d.selection.regular_count, special: d.selection.special_count }) : mode === 'exclude' ? text('exclude', { count: d.selection.exclude_count }) : text(mode)
    return { instructions: [how, text('unit', { points: d.unit_points })], prizes: [
      ...(d.prize_tiers ?? []).map((tier, i) => ({ text: text('tier', { number: i + 1, condition: standardCondition(tier.condition) }), detail: `× ${tier.odds}` })),
      { text: text('return') }, { text: text('round') },
      ...(d.cap_points ? [{ text: text('cap', { points: d.cap_points }) }] : []),
    ], options: [] }
  }
  const { snapshot, play, plate } = input
  const instructions = [text('range', { min: play.selection.min, max: play.selection.max }), text(play.selection.mode, { count: play.selection.per_line }), play.stake_mode === 'unit_multiplier' ? text('unit', { points: play.unit_points! }) : text('perLine')]
  const prizes: PlayerRule[] = []
  if (play.decisions) {
    const d = play.decisions
    const describeIndex = (index: number[], heading: string) => {
      const buckets = d.outcomes.map(() => ({ sums: [] as number[], numbers: [] as number[] }))
      const sums = new Map<number, number[]>()
      index.forEach((_, value) => { const sum = [...String(value).padStart(snapshot.catalog.model.length, '0')].reduce((total, digit) => total + Number(digit), 0); const values = sums.get(sum) ?? []; values.push(value); sums.set(sum, values) })
      for (const [sum, numbers] of sums) {
        if (numbers.every(number => index[number] === index[numbers[0]!])) buckets[index[numbers[0]!]!]!.sums.push(sum)
        else numbers.forEach(number => buckets[index[number]!]!.numbers.push(number))
      }
      const ranges = (values: number[]) => {
        const chunks: string[] = []
        const label = (n: number) => String(n).padStart(snapshot.catalog.model.length, '0')
        for (let i = 0; i < values.length; i++) { const start = values[i]!; let end = start; while (i + 1 < values.length && values[i + 1] === end + 1) end = values[++i]!; chunks.push(start === end ? label(start) : `${label(start)}–${label(end)}`) }
        return chunks.join(', ')
      }
      for (const [i, bucket] of buckets.entries()) {
        if (!bucket.sums.length && !bucket.numbers.length) continue
        const outcome = d.outcomes[i]!
        const condition = [bucket.sums.length ? `${text('sums')}: ${bucket.sums.join(', ')}` : '', bucket.numbers.length ? `${text('numbers')}: ${ranges(bucket.numbers.sort((a,b) => a-b))}` : ''].filter(Boolean).join('; ')
        const amount = [outcome.outcome === 'won' ? text('decisionPrize', { odds: snapshot.prices.plates[plate]![outcome.price_key!]!, factor: outcome.prize_factor! }) : '', text('loss', { factor: outcome.loss_factor })].filter(Boolean).join('\n')
        prizes.push({ text: `${heading} · ${text(outcome.outcome === 'push' ? 'pushOutcome' : outcome.outcome)}`, detail: `${condition}\n${amount}` })
      }
    }
    describeIndex(d.index, text('base'))
    if (d.alternate) {
      const names = snapshot.catalog.aggregates![d.alternate.aggregate]!.map(id => snapshot.catalog.plays.find(p => p.id === id)!.label[locale])
      describeIndex(d.alternate.index, text('alternate', { plays: names.join(', '), points: d.alternate.at_least }))
    }
    instructions.push(text('patternHelp'))
    if (snapshot.catalog.funding === 'reserve') instructions.push(text('reserve', { factor: play.risk_multiplier! }))
  } else {
    if (play.push_when) prizes.push({ text: text('push', { condition: expression(play.push_when as Node) }) })
    for (const [i, raw] of (play.tiers ?? []).entries()) {
      const tier = raw as Node
      let price = ''
      if (tier.price.kind === 'fixed') price = `× ${snapshot.prices.plates[plate]![tier.price.key]}`
      else if (tier.price.kind === 'cases') price = tier.price.cases.map((entry: Node) => `${expression(entry.when)}: × ${snapshot.prices.plates[plate]![entry.key]}`).join('；')
      else if (tier.price.kind !== 'option') throw new Error('Unsupported player price')
      prizes.push({ text: text('tier', { number: i + 1, condition: expression(tier.when) }), detail: [price, text(tier.payout.kind === 'fixed_return' ? 'return' : 'net'), ...(tier.cap_points ? [text('cap', {points: tier.cap_points})] : [])].filter(Boolean).join('\n') })
    }
    if (play.mixed_tier_policy) prizes.push({ text: text(play.mixed_tier_policy) })
    prizes.push({ text: text('round') })
  }
  if (play.cap_points) prizes.push({ text: text('cap', { points: play.cap_points }) })
  const presentedOptions = options.map(option => ({ text: option.label[locale], detail: [configuredNumbers(snapshot, option).join(', '), ...configuredOptionPrices(snapshot, play, option, plate).map(price => `× ${price}`)].filter(Boolean).join(' · ') }))
  return { instructions, prizes, options: presentedOptions }
}
