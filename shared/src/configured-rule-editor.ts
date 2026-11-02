import type { ConfiguredCatalog, ConfiguredSnapshot } from './configured-rules'
import { configuredOptionPriceKeys } from './configured-rules'

export function bindingText(snapshot: ConfiguredSnapshot): Record<string, Record<string, string>> {
  return Object.fromEntries(Object.entries(snapshot.bindings.attributes).map(([id, groups]) => [id,
    Object.fromEntries(Object.entries(groups).map(([label, numbers]) => [label, numbers.join(', ')])),
  ]))
}

export function parseBindingNumbers(text: string): number[] | null {
  if (!text.trim()) return []
  if (!/^\d+(?:[\s,]+\d+)*$/.test(text.trim())) return null
  return text.trim().split(/[\s,]+/).map(Number)
}

/** Match the engine's pool, partition and fixed-assignment constraints before applying. */
export function bindingErrors(catalog: ConfiguredCatalog, texts: Record<string, Record<string, string>>) {
  const model = catalog.model
  const pool = new Set(model.model === 'DIGITS_0_9' ? Array.from({ length: 10 }, (_, i) => i) :
    [model.regular_pool, model.special_pool].flatMap(p => p.values ?? Array.from({ length: (p.max ?? 0) - (p.min ?? 0) + 1 }, (_, i) => i + (p.min ?? 0))))
  const errors: Record<string, Record<string, string>> = {}
  for (const attribute of catalog.attributes) {
    const rows: Record<string, string> = {}
    errors[attribute.id] = rows
    const owners = new Map<number, string[]>()
    for (const label of attribute.labels) {
      const numbers = parseBindingNumbers(texts[attribute.id]![label]!)
      if (!numbers) { rows[label] = 'bindingFormat'; continue }
      if (new Set(numbers).size !== numbers.length) rows[label] = 'bindingDuplicate'
      if (numbers.some(n => !pool.has(n))) rows[label] = 'bindingOutsidePool'
      for (const n of new Set(numbers)) owners.set(n, [...(owners.get(n) ?? []), label])
      if (Object.entries(attribute.fixed ?? {}).some(([n, group]) => group === label && !numbers.includes(Number(n)))) rows[label] = 'bindingFixed'
    }
    if (attribute.partition) for (const labels of owners.values()) if (labels.length > 1) {
      for (const label of labels) rows[label] ??= 'bindingOverlap'
    }
    if ([...pool].some(n => !owners.has(n))) rows[''] = 'bindingIncomplete'
  }
  return errors
}

/** Keep shared keys in a single row, retaining every stored price item. */
export function configuredPriceRows(snapshot: ConfiguredSnapshot, locale: 'en' | 'zh-CN') {
  const references = new Map<string, Set<string>>()
  for (const play of snapshot.catalog.plays) {
    const options = snapshot.catalog.domains[play.selection.domain]!.options
    const names = new Map<string, string[]>()
    for (const option of options) for (const key of configuredOptionPriceKeys(snapshot, play, option)) {
      names.set(key, [...(names.get(key) ?? []), option.label[locale]])
    }
    for (const [key, labels] of names) {
      const label = labels.length === options.length ? play.label[locale] : `${play.label[locale]} · ${[...new Set(labels)].join(' / ')}`
      if (!references.has(key)) references.set(key, new Set())
      references.get(key)!.add(label)
    }
  }
  return [...new Set(Object.values(snapshot.prices.plates).flatMap(plate => Object.keys(plate)))].map(id => ({
    id, label: references.has(id) ? [...references.get(id)!].join(' / ') : id,
  }))
}
