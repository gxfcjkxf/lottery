import { platformDisplaySettings, filterDateBoundary } from '@lottery/shared'
import { AdminApiError } from './admin-api'

export interface CommissionListQuery { from?: string; to?: string; state?: string; cycle_id?: string; agent_id?: string }
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
function validTimestamp(value:unknown):value is string {
  if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value)||!Number.isFinite(Date.parse(value)))return false
  try {filterDateBoundary(value.slice(0,10),false,'utc');return true}catch{return false}
}
export function commissionListParams(limit: number, offset: number, filter: Readonly<CommissionListQuery>, kind: 'cycle' | 'payment' | 'target' | 'correction') {
  const invalid = (): never => { throw new AdminApiError('Invalid commission query', 0, 'INVALID_INPUT') }
  const states = kind === 'cycle' ? ['', 'enumerating','waiting','calculating','summarizing','ready','failed','stale'] : kind === 'payment' ? ['', 'awaiting_approval','paying','paid','failed','blocked','stale','needs_attention'] : kind === 'correction' ? ['', 'pending','processing','history'] : ['', 'pending','paid']
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100 || !Number.isSafeInteger(offset) || offset < 0 || offset > 1000000 || !states.includes(filter.state ?? '')) invalid()
  if (Object.keys(filter).some(key => !['from','to','state','cycle_id','agent_id'].includes(key))) invalid()
  if ([filter.cycle_id, filter.agent_id].some(id => id !== undefined && !uuid.test(id))) invalid()
  if ((filter.from === undefined) !== (filter.to === undefined)) invalid()
  if (filter.from !== undefined && (!validTimestamp(filter.from) || !validTimestamp(filter.to) || Date.parse(filter.to!) <= Date.parse(filter.from))) invalid()
  if (kind === 'target' && (filter.from !== undefined || filter.cycle_id !== undefined)) invalid()
  const query = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  for (const [key,value] of Object.entries(filter)) if (value) query.set(key,value)
  return query
}

// Interpret day controls in the platform's configured timezone, independent of the device.
export function commissionDateRange(from: string, to: string): Pick<CommissionListQuery,'from'|'to'> {
  if (!from && !to) return {}
  if (!from || !to || !platformDisplaySettings.value) throw new TypeError('Invalid commission date range')
  const zone = platformDisplaySettings.value.timezone
  const formatter = new Intl.DateTimeFormat('en-US',{timeZone:zone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'})
  const boundary = (value: string, end: boolean) => {
    const wall = filterDateBoundary(value,end,'utc').getTime()
    let instant = wall
    for (let i=0;i<3;i++) {
      const parts = Object.fromEntries(formatter.formatToParts(new Date(instant)).map(p=>[p.type,p.value]))
      const actual = Date.parse(`${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}:${parts.second}Z`)
      if (actual === wall) return new Date(instant).toISOString()
      instant += wall-actual
    }
    throw new TypeError('Invalid commission date boundary')
  }
  const range = {from:boundary(from,false),to:boundary(to,true)}
  if (range.from >= range.to) throw new TypeError('Invalid commission date range')
  return range
}
