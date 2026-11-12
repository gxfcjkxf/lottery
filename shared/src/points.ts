/** Exact points are decimal strings with at most two fractional digits. */
export type Points = string & { readonly __points: unique symbol }

export function pointsMinor(value: string | bigint): bigint {
  const raw = String(value)
  if (/^-0(?:\.0{1,2})?$/.test(raw)) throw new TypeError(`Invalid points value: ${raw}`)
  if (!/^-?(0|[1-9]\d*)(?:\.\d{1,2})?$/.test(raw)) throw new TypeError(`Invalid points value: ${raw}`)
  const [whole, fraction = ''] = raw.replace(/^-/, '').split('.')
  const minor = BigInt(whole!) * 100n + BigInt(fraction.padEnd(2, '0'))
  return raw.startsWith('-') ? -minor : minor
}

export function pointsFromMinor(value: bigint): Points {
  const negative = value < 0n
  const amount = negative ? -value : value
  const fraction = (amount % 100n).toString().padStart(2, '0').replace(/0+$/, '')
  return `${negative ? '-' : ''}${amount / 100n}${fraction ? `.${fraction}` : ''}` as Points
}

export function isPointTotal(value: unknown, signed = false): value is string {
  if (typeof value !== 'string' || value.length > 128) return false
  try { const minor = pointsMinor(value); return signed || minor >= 0n } catch { return false }
}

export function isPoints(value: unknown, signed = false): value is string {
  if (typeof value !== 'string') return false
  try {
    const minor = pointsMinor(value)
    return minor >= (signed ? -9223372036854775808n : 0n) && minor <= 9223372036854775807n
  } catch { return false }
}

export function points(value: string | bigint | number): Points {
  if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new TypeError('Number points values must be safe integers; use a decimal string')
  return pointsFromMinor(pointsMinor(String(value)))
}

export function formatPoints(value: Points | string | bigint, locale = 'en'): string {
  const raw = pointsFromMinor(pointsMinor(value))
  const [whole, fraction] = raw.replace(/^-/, '').split('.')
  const parts = new Intl.NumberFormat(locale).formatToParts(BigInt(whole!))
  const grouped = parts.map(part => part.value).join('')
  const decimal = new Intl.NumberFormat(locale).formatToParts(1.1).find(part => part.type === 'decimal')!.value
  return `${raw.startsWith('-') ? '-' : ''}${grouped}${fraction ? decimal + fraction.padEnd(2, '0') : ''}`
}
