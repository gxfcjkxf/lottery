import { readApiEnvelope } from './api-response'
import { notifySessionReplaced } from './session-replacement'

export interface FilterOption { id: string; name: string }
export interface FilterOptionsPage { brand_id: string; items: FilterOption[]; limit: number; offset: number }
export class FilterOptionsError extends Error {
  constructor(message: string, public status: number, public code: string) { super(message) }
}
interface FilterOptionsScope {
  surface: 'admin' | 'platform'; brandId: string; resource: string; kind: string; search?: string; action?: 'view' | 'grant'; signal?: AbortSignal
}
/** The server selects the directory by the target resource, under that resource's
 * permission. Callers never fall back to a different game source on failure. */
export async function readFilterOptions(scope: FilterOptionsScope, offset = 0, fetcher: typeof fetch = fetch): Promise<FilterOptionsPage> {
  const params = new URLSearchParams({ resource: scope.resource, kind: scope.kind, limit: '100', offset: String(offset) })
  if (scope.action !== undefined) params.set('action', scope.action)
  if (scope.search !== undefined) params.set('search', scope.search)
  let response: Response
  try {
    response = await fetcher(`/api/v1/${scope.surface}/filter-options?${params}`, { signal: scope.signal, credentials: 'same-origin', cache: 'no-store', headers: { Accept: 'application/json', 'X-Brand-ID': scope.brandId } })
  } catch (cause) {
    if (scope.signal?.aborted) throw cause
    throw new FilterOptionsError('Unable to load options', 0, 'NETWORK_ERROR')
  }
  let data: FilterOptionsPage
  try { data = (await readApiEnvelope<FilterOptionsPage>(response, (message, status, code) => new FilterOptionsError(message, status, code))).data }
  catch (cause) { if (cause instanceof FilterOptionsError) notifySessionReplaced(cause.code); throw cause }
  if (!data || data.brand_id !== scope.brandId || data.limit !== 100 || data.offset !== offset || !Array.isArray(data.items) || data.items.length > 100 || data.items.some(v => !v || typeof v.id !== 'string' || !v.id || typeof v.name !== 'string' || !v.name)) throw new FilterOptionsError('Invalid options response', 502, 'INVALID_RESPONSE')
  return data
}
export async function readAllFilterOptions(scope: FilterOptionsScope, fetcher: typeof fetch = fetch): Promise<FilterOption[]> {
  const items: FilterOption[] = []
  for (let offset = 0; ; offset += 100) {
    const page = await readFilterOptions(scope, offset, fetcher)
    items.push(...page.items)
    if (page.items.length < 100) return items
  }
}
