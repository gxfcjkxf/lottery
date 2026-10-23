import { expect, it } from 'vitest'
import { filterDateBoundary } from './filter-dates'
it('includes the selected UTC day through next-day midnight',()=>{
 expect(filterDateBoundary('2026-10-20',false,'utc').toISOString()).toBe('2026-10-20T00:00:00.000Z')
 expect(filterDateBoundary('2026-12-31',true,'utc').toISOString()).toBe('2027-01-01T00:00:00.000Z')
 expect(()=>filterDateBoundary('2026-02-30',false,'utc')).toThrow()
})
it('preserves local calendar boundaries instead of adding 24 hours',()=>{
 const end=filterDateBoundary('2026-10-20',true,'local')
 expect([end.getFullYear(),end.getMonth(),end.getDate(),end.getHours()]).toEqual([2026,9,21,0])
})
