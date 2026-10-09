import { describe, expect, it, vi } from 'vitest'
import { createPlatformRulesApi } from './rules-api'
import { PlatformApiError } from './platform-api'

const brandId = '11111111-1111-4111-8111-111111111111'
const gameId = '22222222-2222-4222-8222-222222222222'
const playId = '33333333-3333-4333-8333-333333333333'
const versionId = '44444444-4444-4444-8444-444444444444'
const definition = {
  schema_version: 1,
  model: { model: 'DIGITS_0_9', regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, regular_count: 0, special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true },
  selection: { mode: 'numbers', regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: null, feature_choices: null },
  number_attributes: null, unit_points: '1', prize_tiers: [{ code: 'EXACT', condition: { op: 'equals', field: 'position_match', value: 3 }, odds: '10', exclusive: true, cap_points: null }], mixed_tier_policy: 'max_all', cap_points: null, rounding: 'half_up', rounding_scope: 'order',
  limits: { max_combinations: 100, max_multiplier: '100', max_bet_points: null },
}
const version = {
  id: versionId, brand_id: brandId, game_id: gameId, play_id: playId, version_no: 1, version: 3,
  definition, definition_hash: 'a'.repeat(64), status: 'draft', effect_mode: 'immediate', created_by: '55555555-5555-4555-8555-555555555555',
  reviewed_by: '', review_comment: '', created_at: '2026-10-10T01:02:03Z', updated_at: '2026-10-10T01:02:03Z',
}
const reply = (data: unknown, status = 200) => new Response(JSON.stringify({ success: status < 400, data, ...(status >= 400 ? { error: { code: 'PERMISSION_DENIED', message: 'Forbidden' } } : {}) }), { status })

describe('platform rules API', () => {
  it('uses the exact read routes, paging, brand header, and same-origin credentials', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(reply({ plays: [{ id: playId, brand_id: brandId, game_id: gameId, code: 'main', name: 'Main', status: 'active', active_version_id: '', version: 1 }], limit: 51, offset: 0 }))
      .mockResolvedValueOnce(reply({ versions: [version], limit: 20, offset: 4 }))
      .mockResolvedValueOnce(reply(version))
    const api = createPlatformRulesApi(fetcher)
    await expect(api.plays(brandId, gameId)).resolves.toHaveLength(1)
    await expect(api.versions(brandId, gameId, playId, 20, 4)).resolves.toEqual([version])
    await expect(api.read(brandId, gameId, playId, versionId)).resolves.toEqual(version)
    expect(fetcher.mock.calls.map(call => [call[0], call[1]])).toEqual([
      [`/api/v1/platform/games/${gameId}/plays?limit=51&offset=0`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } }],
      [`/api/v1/platform/plays/${playId}/rule-versions?limit=20&offset=4`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } }],
      [`/api/v1/platform/rule-versions/${versionId}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } }],
    ])
  })

  it('rejects invalid scope, resource IDs, and pagination before fetching', async () => {
    const fetcher = vi.fn()
    const api = createPlatformRulesApi(fetcher)
    await expect(api.plays('', gameId)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.plays(brandId, 'bad')).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.versions(brandId, gameId, playId, 101)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    await expect(api.versions(brandId, gameId, playId, 1, -1)).rejects.toMatchObject({ code: 'REQUEST_INVALID' })
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('rejects mismatched DTO scopes and preserves server permission errors', async () => {
    const wrongBrand = createPlatformRulesApi(async () => reply({ versions: [{ ...version, brand_id: '66666666-6666-4666-8666-666666666666' }], limit: 51, offset: 0 }))
    await expect(wrongBrand.versions(brandId, gameId, playId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongPlay = createPlatformRulesApi(async () => reply({ ...version, play_id: gameId }))
    await expect(wrongPlay.read(brandId, gameId, playId, versionId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const denied = createPlatformRulesApi(async () => reply(null, 403))
    await expect(denied.plays(brandId, gameId)).rejects.toBeInstanceOf(PlatformApiError)
    await expect(denied.plays(brandId, gameId)).rejects.toMatchObject({ status: 403, code: 'PERMISSION_DENIED', message: 'Forbidden' })
  })

  it('rejects malformed current DTO fields and page metadata', async () => {
    const malformed = createPlatformRulesApi(async () => reply({ plays: [{ id: playId, brand_id: brandId, game_id: gameId, code: 'main', name: 'Main', status: 'active', active_version_id: null, version: 1 }], limit: 51, offset: 0 }))
    await expect(malformed.plays(brandId, gameId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
    const wrongPage = createPlatformRulesApi(async () => reply({ versions: [version], limit: 50, offset: 0 }))
    await expect(wrongPage.versions(brandId, gameId, playId)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  })
})
