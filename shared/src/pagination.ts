/** Database counts are decimal strings so totals remain exact above Number.MAX_SAFE_INTEGER. */
export function readPaginationTotal(page: unknown): string {
  const total = page && typeof page === 'object' && 'total_count' in page ? page.total_count : undefined
  if (typeof total !== 'string' || !/^(0|[1-9]\d*)$/.test(total)) throw new Error('Invalid pagination total_count')
  return total
}
