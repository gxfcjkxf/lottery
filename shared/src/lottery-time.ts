/** Convert a canonical local date/time using an explicit IANA zone, not the device zone. */
export function lotteryLocalTimeToISO(value: string, timezone: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value)
  if (!match) throw new Error('Invalid lottery local time')
  const target = [...match.slice(1, 6).map(Number), Number(match[6] ?? '0')]
  const formatter = new Intl.DateTimeFormat('en', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' })
  const numbers = (instant: number) => {
    const parts = Object.fromEntries(formatter.formatToParts(new Date(instant)).map(part => [part.type, part.value]))
    return ['year', 'month', 'day', 'hour', 'minute', 'second'].map(key => Number(parts[key]))
  }
  const utc = (parts: number[]) => Date.UTC(parts[0], parts[1] - 1, parts[2], parts[3], parts[4], parts[5])
  const wanted = utc(target)
  let instant = wanted
  for (let step = 0; step < 2; step++) instant += wanted - utc(numbers(instant))
  if (!numbers(instant).every((part, index) => part === target[index])) throw new Error('Invalid lottery local time')
  return new Date(instant).toISOString()
}
