import { describe, expect, it } from 'vitest'
import { reportDateRange } from './report-date-range'
import { normalizeReportQuery } from './reports-api'

describe('whole-day report filters', () => {
  it('includes the entire selected end date with an exclusive API boundary', () => {
    expect(reportDateRange('2026-10-20', '2026-10-20')).toEqual({ from: '2026-10-20T00:00:00Z', to: '2026-10-21T00:00:00Z' })
    expect(reportDateRange('2026-12-30', '2026-12-31').to).toBe('2027-01-01T00:00:00Z')
  })
  it('rejects invalid or reversed dates and retains the API range limit', () => {
    for (const [from, to] of [['2026-02-30', '2026-03-01'], ['2026-10-21', '2026-10-20']]) expect(() => reportDateRange(from, to)).toThrow()
    expect(() => normalizeReportQuery('betting', { ...reportDateRange('2026-01-01', '2026-04-04'), group_by: 'day' })).toThrow()
  })
})
