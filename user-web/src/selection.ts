export interface SelectionModel {
  id: string
  kind: 'regular-special' | 'digits'
  regularPool: number
  regularPick: number
  specialPool?: number
  specialPick?: number
  digitCount?: number
  unitPoints: bigint
  minMultiplier: number
  maxMultiplier: number
}

export interface SelectionDraft {
  regular: number[]
  special: number[]
  digits: Array<number | null>
}

export function combinations(n: number, k: number): bigint {
  if (!Number.isInteger(n) || !Number.isInteger(k) || n < 0 || k < 0 || k > n) return 0n
  let result = 1n
  for (let i = 1; i <= k; i++) result = result * BigInt(n - k + i) / BigInt(i)
  return result
}

export function selectionCount(model: SelectionModel, draft: SelectionDraft): bigint {
  if (model.kind === 'digits') {
    const count = model.digitCount ?? model.regularPick
    return draft.digits.length === count && draft.digits.every(digit => digit !== null) ? 1n : 0n
  }
  const regularLines = combinations(draft.regular.length, model.regularPick)
  const specialLines = model.specialPick ? combinations(draft.special.length, model.specialPick) : 1n
  return regularLines * specialLines
}

export function totalPoints(model: SelectionModel, draft: SelectionDraft, multiplier: number): bigint {
  if (!Number.isSafeInteger(multiplier) || multiplier < model.minMultiplier || multiplier > model.maxMultiplier) {
    throw new RangeError(`Multiplier must be ${model.minMultiplier}–${model.maxMultiplier}`)
  }
  return selectionCount(model, draft) * model.unitPoints * BigInt(multiplier)
}

function hasOutOfRange(values: number[], min: number, max: number): boolean {
  return values.some(value => !Number.isInteger(value) || value < min || value > max)
}

export function validateSelection(model: SelectionModel, draft: SelectionDraft, multiplier: number): string | null {
  if (model.kind === 'digits') {
    const count = model.digitCount ?? model.regularPick
    if (draft.digits.length !== count || draft.digits.some(digit => digit === null)) return `Choose all ${count} digits.`
    if (draft.digits.some(digit => digit !== null && (!Number.isInteger(digit) || digit < 0 || digit >= model.regularPool))) return `Choose digits from 0 to ${model.regularPool - 1}.`
  } else {
    if (new Set(draft.regular).size !== draft.regular.length) return 'Remove repeated regular numbers.'
    if (hasOutOfRange(draft.regular, 1, model.regularPool)) return `Choose regular numbers from 1 to ${model.regularPool}.`
    if (draft.regular.length < model.regularPick) return `Choose at least ${model.regularPick} regular numbers.`
    if (model.specialPick) {
      if (new Set(draft.special).size !== draft.special.length) return 'Remove repeated special numbers.'
      if (hasOutOfRange(draft.special, 1, model.specialPool ?? 0)) return `Choose special numbers from 1 to ${model.specialPool}.`
      if (draft.special.length < model.specialPick) return `Choose at least ${model.specialPick} special number.`
    }
  }
  if (!Number.isSafeInteger(multiplier) || multiplier < model.minMultiplier || multiplier > model.maxMultiplier) return `Multiplier must be ${model.minMultiplier}–${model.maxMultiplier}.`
  return null
}
