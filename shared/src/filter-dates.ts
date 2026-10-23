/** Day filters include the selected end day; preserve each endpoint's existing time basis. */
export function filterDateBoundary(value: string, end: boolean, basis: 'utc' | 'local'): Date {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) throw new TypeError('Invalid filter date')
  const date = new Date(`${value}T00:00:00${basis === 'utc' ? 'Z' : ''}`)
  const canonical = basis === 'utc' ? date.toISOString().slice(0, 10) : `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`
  if (!Number.isFinite(date.getTime()) || canonical !== value) throw new TypeError('Invalid filter date')
  if (end) { if (basis === 'utc') date.setUTCDate(date.getUTCDate()+1); else date.setDate(date.getDate()+1) }
  return date
}
