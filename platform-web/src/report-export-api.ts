import { readReportDownload } from "@lottery/shared"
import { readApiRejection } from '@lottery/shared'

import { PlatformApiError } from './platform-api'
import { normalizeReportQuery, type ReportKind, type ReportQuery } from './reports-api'

const BASE = '/api/v1/platform/reports'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export type ReportExportQuery = Omit<ReportQuery, 'limit' | 'offset'>
export interface PlatformReportExport {
  filename: string
  bytes: Uint8Array
  groupCount: string
  snapshotAt: string
  auditLogId: string
}

function fail(): never { throw new PlatformApiError('Invalid report export response', 502, 'INVALID_RESPONSE') }
function badInput(message: string): never { throw new PlatformApiError(message, 0, 'INVALID_INPUT') }
function route(kind: ReportKind): string {
  if (kind === 'betting' || kind === 'ledger') return `${BASE}/${kind}/export`
  if (kind === 'withdrawal') return `${BASE}/withdrawal/export`
  if (kind === 'commission') return `${BASE}/commission.csv`
  return `${BASE}/${kind === 'reward_orders' ? 'reward-orders' : 'rewards'}.csv`
}

export function createPlatformReportExportApi(fetcher: typeof fetch = fetch) {
  return {
    async export(brandId: string, kind: ReportKind, query: ReportExportQuery): Promise<PlatformReportExport> {
      if (!UUID.test(brandId)) badInput('A valid brand UUID is required')
      if (!['betting', 'ledger', 'withdrawal', 'commission', 'rewards', 'reward_orders'].includes(kind)) badInput('Unsupported report kind')
      if (!query || Object.hasOwn(query, 'limit') || Object.hasOwn(query, 'offset')) badInput('Report exports do not accept pagination')
      const brand = brandId.toLowerCase()
      const q = normalizeReportQuery(kind, { ...query, limit: 100, offset: 0 })
      const params = new URLSearchParams({ from: q.from, to: q.to, group_by: q.group_by })
      for (const [key, value] of Object.entries(q.filters)) if (value !== null) params.set(key, value)
      let response: Response
      try {
        response = await fetcher(`${route(kind)}?${params}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'text/csv', 'X-Brand-ID': brand } })
      } catch (cause) {
        throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
      }
      if (!response.ok) throw await readApiRejection(response,(message,status,code)=>new PlatformApiError(message,status,code))
      return readReportDownload(response, brand, kind, fail)
    },
  }
}
