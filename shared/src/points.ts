/** Points are integer minor units represented as decimal strings across JSON boundaries. */
export type Points = string & { readonly __points: unique symbol }

export function points(value: string | bigint | number): Points {
  if (typeof value === 'number' && (!Number.isSafeInteger(value))) throw new TypeError('Number points values must be safe integers; use a decimal string for large values')
  const raw = typeof value === 'bigint' ? value.toString() : String(value)
  if (!/^-?\d+$/.test(raw)) throw new TypeError(`Invalid points value: ${raw}`)
  return BigInt(raw).toString() as Points
}

export function addPoints(...values: Array<Points | string | bigint>): Points {
  return values.reduce<bigint>((sum, value) => sum + BigInt(value), 0n).toString() as Points
}

export function multiplyPoints(value: Points | string | bigint, multiplier: number): Points {
  if (!Number.isSafeInteger(multiplier) || multiplier < 0) throw new RangeError('Multiplier must be a non-negative safe integer')
  return (BigInt(value) * BigInt(multiplier)).toString() as Points
}

export function formatPoints(value: Points | string | bigint, locale = 'en'): string {
  return new Intl.NumberFormat(locale).format(BigInt(value))
}
