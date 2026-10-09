import { describe, expect, it, vi } from 'vitest'
import { createPlatformArchiveTasksApi } from './archive-tasks-api'

const brand = '4b1bc4f5-24ed-456b-8c43-680bcb710c51'
const taskId = 'aaec733e-1135-4c55-9f69-0b93e817e18f'
const archiveId = 'ff794189-608c-4dd1-b454-572540a79910'
const auditId = '5b698c19-f51e-48cc-8d63-bd9d87e45cf7'
const stamp = '2026-10-09T01:02:03.123456789+08:00'

function reply(data: unknown, status = 200) {
  return new Response(JSON.stringify({ success: true, data }), { status, headers: { 'Content-Type': 'application/json' } })
}

const policy = {
  brand_id: brand, version: 1, daily_enabled: false, monthly_enabled: false,
  daily_start_period: null, monthly_start_period: null, timezone: 'Asia/Singapore', audit_log_id: null, updated_at: stamp,
}

function task(state: 'pending' | 'completed' | 'skipped' | 'failed' = 'pending') {
  return {
    id: taskId, brand_id: brand, policy_version: 1,
    window: { kind: 'daily', period_key: '2026-10-08', timezone: 'Asia/Singapore', from: '2026-10-08T00:00:00+08:00', to: '2026-10-09T00:00:00+08:00' },
    state, version: state === 'pending' ? 1 : 2, attempt_count: state === 'pending' ? 0 : 1,
    archive_id: state === 'completed' || state === 'skipped' ? archiveId : null,
    last_error_code: state === 'failed' ? 'ARCHIVE_FAILED' : null,
    creation_audit_log_id: auditId, last_audit_log_id: auditId, created_at: stamp, updated_at: stamp,
  }
}

describe('platform report archive tasks API', () => {
  it('reads disabled initial policy and later enabled policy while allowing retained starts after disable', async () => {
    const fetcher = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(reply(policy))
      .mockResolvedValueOnce(reply({ ...policy, version: 2, daily_enabled: true, daily_start_period: '2026-10-01', audit_log_id: auditId }))
      .mockResolvedValueOnce(reply({ ...policy, version: 3, daily_start_period: '2026-10-01', audit_log_id: auditId }))
    const api = createPlatformArchiveTasksApi(fetcher)
    await expect(api.policy(brand)).resolves.toEqual(policy)
    await expect(api.policy(brand)).resolves.toMatchObject({ daily_enabled: true, daily_start_period: '2026-10-01' })
    await expect(api.policy(brand)).resolves.toMatchObject({ daily_enabled: false, daily_start_period: '2026-10-01' })
    expect(fetcher.mock.calls.map(call => call[0])).toEqual(Array(3).fill('/api/v1/platform/report-archive-policy'))
  })

  it('lists pages, reads an exact task, scopes every request to the brand, and uses GET only', async () => {
    const item = task('pending')
    const page = { brand_id: brand, items: [item], total_count: '1', limit: 50, offset: 0 }
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(reply(page)).mockResolvedValueOnce(reply(item))
    const api = createPlatformArchiveTasksApi(fetcher)
    await expect(api.list(brand)).resolves.toEqual(page)
    await expect(api.read(brand, taskId)).resolves.toEqual(item)
    expect(fetcher.mock.calls.map(([url, init]) => [url, init?.method, new Headers(init?.headers).get('X-Brand-ID')])).toEqual([
      ['/api/v1/platform/report-archive-tasks?limit=50&offset=0', 'GET', brand],
      [`/api/v1/platform/report-archive-tasks/${taskId}`, 'GET', brand],
    ])
  })

  it('accepts all four current task states with their archive and error relationships', async () => {
    for (const state of ['pending', 'completed', 'skipped', 'failed'] as const) {
      await expect(createPlatformArchiveTasksApi(async () => reply(task(state))).read(brand, taskId)).resolves.toMatchObject({ state })
    }
  })

  it('rejects wrong brand or task ID and invalid current DTOs', async () => {
    await expect(createPlatformArchiveTasksApi(async () => reply({ ...policy, brand_id: taskId })).policy(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply(task())).read(brand, archiveId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply({ ...task(), brand_id: taskId })).read(brand, taskId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply({ ...task('pending'), archive_id: archiveId })).read(brand, taskId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply({ ...policy, daily_enabled: true })).policy(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply(policy, 201)).policy(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('validates canonical paging, textual totals, and task row scope', async () => {
    const api = createPlatformArchiveTasksApi(async () => reply({ brand_id: brand, items: [task()], total_count: '01', limit: 50, offset: 0 }))
    await expect(api.list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply({ brand_id: brand, items: [task()], total_count: '1', limit: 50, offset: 0 })).list(brand)).resolves.toMatchObject({ total_count: '1' })
    await expect(createPlatformArchiveTasksApi().list(brand, 0, 0)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(createPlatformArchiveTasksApi().list(brand, 50, 1_000_001)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(createPlatformArchiveTasksApi(async () => reply({ brand_id: brand, items: [{ ...task(), brand_id: taskId }], total_count: '1', limit: 50, offset: 0 })).list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    await expect(createPlatformArchiveTasksApi(async () => reply({ brand_id: brand, items: [task()], total_count: '51', limit: 50, offset: 0 })).list(brand)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })

  it('requires a brand and canonical task ID, and preserves HTTP errors', async () => {
    await expect(createPlatformArchiveTasksApi().policy('')).rejects.toMatchObject({ code: 'BRAND_REQUIRED' })
    await expect(createPlatformArchiveTasksApi().read(brand, 'wrong-id')).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    const fetcher = async () => new Response(JSON.stringify({ success: false, error: { code: 'DENIED', message: 'Not allowed' } }), { status: 403 })
    await expect(createPlatformArchiveTasksApi(fetcher).policy(brand)).rejects.toMatchObject({ status: 403, code: 'DENIED', message: 'Not allowed' })
  })
})
