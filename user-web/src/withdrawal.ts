export function isValidWholeAmount(value: string, maximum: bigint): boolean {
  if (!/^\d+$/.test(value.trim())) return false
  const amount = BigInt(value.trim())
  return amount > 0n && amount <= maximum
}
