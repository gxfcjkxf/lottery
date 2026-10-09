import { PlatformApiError } from './platform-api'
import type { RuleDefinition } from '../../shared/src/rules'

export interface PlatformPlay {
  id: string
  brand_id: string
  game_id: string
  code: string
  name: string
  status: string
  active_version_id: string
  version: number
}

export interface PlatformRuleValidationFinding {
  code: string
  message: string
  blocking: boolean
}

export interface PlatformRuleValidationReport {
  passed: boolean
  definition_hash: string
  cases: unknown[]
  warnings: string[]
  findings: PlatformRuleValidationFinding[]
}

export interface PlatformRuleVersion {
  id: string
  brand_id: string
  game_id: string
  play_id: string
  version_no: number
  version: number
  definition: RuleDefinition
  definition_hash: string
  status: string
  effect_mode: string
  created_by: string
  reviewed_by: string
  review_comment: string
  created_at: string
  updated_at: string
  effective_at?: string
  effective_period_id?: string
  effective_sequence?: number
  source_version_id?: string
  validation?: PlatformRuleValidationReport
  audit_log_id?: string
}

export type Play = PlatformPlay
export type RuleVersion = PlatformRuleVersion

type Obj = Record<string, unknown>
const BASE = '/api/v1/platform'
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const isObj = (value: unknown): value is Obj => Boolean(value && typeof value === 'object' && !Array.isArray(value))
const isText = (value: unknown): value is string => typeof value === 'string'
const isInt = (value: unknown): value is number => Number.isSafeInteger(value)
const dateTime = (value: unknown): value is string => isText(value) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value))
function invalid(message: string): never { throw new PlatformApiError(message, 0, 'INVALID_RESPONSE') }
function requireUUID(value: string, label: string) {
  if (!UUID.test(value)) throw new PlatformApiError(`Invalid ${label}`, 400, 'REQUEST_INVALID')
}
function page(limit: number, offset: number) {
  if (!Number.isInteger(limit) || limit < 1 || limit > 100 || !Number.isInteger(offset) || offset < 0 || offset > 1_000_000) {
    throw new PlatformApiError('Invalid pagination', 400, 'REQUEST_INVALID')
  }
}
function isDefinition(value: unknown): value is RuleDefinition {
  if (!isObj(value) || !Number.isSafeInteger(value.schema_version) || !isObj(value.model) || !isObj(value.selection) || !isObj(value.limits)) return false
  const model = value.model
  const selection = value.selection
  const limits = value.limits
  const pool = (v: unknown) => isObj(v) && typeof v.allow_repeat === 'boolean' &&
    (v.min === undefined || isInt(v.min)) && (v.max === undefined || isInt(v.max)) &&
    (v.values === undefined || v.values === null || (Array.isArray(v.values) && v.values.every(isInt)))
  return isText(model.model) && pool(model.regular_pool) && pool(model.special_pool) &&
    ['regular_count', 'special_count', 'pool_size', 'total_count', 'length'].every(key => isInt(model[key])) &&
    typeof model.allow_repeat === 'boolean' && typeof model.ordered === 'boolean' &&
    isText(selection.mode) && ['regular_count', 'special_count', 'exclude_count'].every(key => isInt(selection[key])) &&
    (selection.attribute_groups === null || (Array.isArray(selection.attribute_groups) && selection.attribute_groups.every(isText))) &&
    (selection.feature_choices === null || isObj(selection.feature_choices)) &&
    (value.number_attributes === null || isObj(value.number_attributes)) && isText(value.unit_points) &&
    (value.prize_tiers === null || Array.isArray(value.prize_tiers)) && isText(value.mixed_tier_policy) &&
    (value.cap_points === null || isText(value.cap_points)) && isText(value.rounding) && isText(value.rounding_scope) &&
    isInt(limits.max_combinations) && isText(limits.max_multiplier) && (limits.max_bet_points === null || isText(limits.max_bet_points))
}
function isValidation(value: unknown): value is PlatformRuleValidationReport {
  return isObj(value) && typeof value.passed === 'boolean' && isText(value.definition_hash) && Array.isArray(value.cases) &&
    Array.isArray(value.warnings) && value.warnings.every(isText) && Array.isArray(value.findings) && value.findings.every(finding =>
      isObj(finding) && isText(finding.code) && isText(finding.message) && typeof finding.blocking === 'boolean')
}
function isPlay(value: unknown, brandId: string, gameId: string): value is PlatformPlay {
  return isObj(value) && UUID.test(String(value.id)) && value.brand_id === brandId && value.game_id === gameId &&
    isText(value.code) && isText(value.name) && isText(value.status) && isText(value.active_version_id) && isInt(value.version)
}
function isRuleVersion(value: unknown, brandId: string, gameId: string, playId?: string): value is PlatformRuleVersion {
  return isObj(value) && UUID.test(String(value.id)) && value.brand_id === brandId && value.game_id === gameId &&
    (playId === undefined || value.play_id === playId) && UUID.test(String(value.play_id)) &&
    isInt(value.version_no) && isInt(value.version) && isDefinition(value.definition) && isText(value.definition_hash) &&
    isText(value.status) && isText(value.effect_mode) && isText(value.created_by) && isText(value.reviewed_by) &&
    isText(value.review_comment) && dateTime(value.created_at) && dateTime(value.updated_at) &&
    (value.effective_at === undefined || dateTime(value.effective_at)) &&
    (value.effective_period_id === undefined || isText(value.effective_period_id)) &&
    (value.effective_sequence === undefined || isInt(value.effective_sequence)) &&
    (value.source_version_id === undefined || isText(value.source_version_id)) &&
    (value.validation === undefined || isValidation(value.validation)) &&
    (value.audit_log_id === undefined || isText(value.audit_log_id))
}

export function createPlatformRulesApi(fetcher: typeof fetch = fetch) {
  async function get(path: string, brandId: string): Promise<unknown> {
    requireUUID(brandId, 'brand ID')
    let response: Response
    try {
      response = await fetcher(`${BASE}${path}`, { method: 'GET', credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Brand-ID': brandId } })
    } catch (cause) {
      throw new PlatformApiError(cause instanceof Error ? cause.message : 'Network request failed', 0, 'NETWORK_ERROR', true)
    }
    let envelope: unknown
    try { envelope = await response.json() } catch {
      throw new PlatformApiError(response.ok ? 'Invalid server response' : `Request failed (${response.status})`, response.status, 'INVALID_RESPONSE')
    }
    if (!isObj(envelope) || envelope.success !== true || envelope.data === undefined || !response.ok) {
      const error = isObj(envelope) && isObj(envelope.error) ? envelope.error : undefined
      throw new PlatformApiError(isText(error?.message) ? error.message : `Request failed (${response.status})`, response.status, isText(error?.code) ? error.code : undefined)
    }
    return envelope.data
  }
  return {
    async plays(brandId: string, gameId: string, limit = 51, offset = 0): Promise<PlatformPlay[]> {
      requireUUID(gameId, 'game ID')
      page(limit, offset)
      const data = await get(`/games/${gameId}/plays?limit=${limit}&offset=${offset}`, brandId)
      if (!isObj(data) || !Array.isArray(data.plays) || data.plays.length > limit || !data.plays.every(item => isPlay(item, brandId, gameId)) || data.limit !== limit || data.offset !== offset) invalid('Invalid plays response')
      return data.plays as PlatformPlay[]
    },
    async versions(brandId: string, gameId: string, playId: string, limit = 51, offset = 0): Promise<PlatformRuleVersion[]> {
      requireUUID(gameId, 'game ID')
      requireUUID(playId, 'play ID')
      page(limit, offset)
      const data = await get(`/plays/${playId}/rule-versions?limit=${limit}&offset=${offset}`, brandId)
      if (!isObj(data) || !Array.isArray(data.versions) || data.versions.length > limit || !data.versions.every(item => isRuleVersion(item, brandId, gameId, playId)) || data.limit !== limit || data.offset !== offset) invalid('Invalid rule versions response')
      return data.versions as PlatformRuleVersion[]
    },
    async read(brandId: string, gameId: string, playId: string, versionId: string): Promise<PlatformRuleVersion> {
      requireUUID(gameId, 'game ID')
      requireUUID(playId, 'play ID')
      requireUUID(versionId, 'rule version ID')
      const value = await get(`/rule-versions/${versionId}`, brandId)
      if (!isRuleVersion(value, brandId, gameId, playId) || value.id !== versionId) invalid('Invalid rule version response')
      return value
    },
  }
}
