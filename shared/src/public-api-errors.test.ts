import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { createCatalogI18n } from './localization'
import { publicApiErrorCatalog, publicApiErrorKey, publicApiErrorText } from './public-api-errors'

describe('public API error presentation', () => {
  it.each([
    ['COMMISSION_PAYMENT_CORRECTION_REQUIRED', 409, 'commissionCorrection'],
    ['COMMISSION_CORRECTION_AVAILABLE_INSUFFICIENT', 409, 'commissionPoints'],
    ['ROLE_ASSIGNED', 409, 'assigned'], ['POINTS_INSUFFICIENT', 409, 'points'],
    ['BET_LEGACY_PERIOD',409,'legacyPeriod'], ['BET_PERIOD_CLOSED', 409, 'closed'], ['AUTH_INVALID_CREDENTIALS', 401, 'credentials'],
    ['PERMISSION_DENIED', 403, 'denied'], ['WITHDRAWAL_INELIGIBLE', 409, 'ineligible'],
    ['WITHDRAWAL_ACTIVE_ORDER', 409, 'activeWithdrawal'], ['REPORT_EXPORT_TOO_LARGE', 400, 'tooLarge'],
    ['AGENT_LIMIT_CONFLICT', 409, 'agentLimit'],
    ['REWARD_POLICY_LIMIT', 409, 'limit'], ['COMMISSION_PAYMENT_EVIDENCE_CONFLICT', 409, 'changed'],
    ['REPORT_ARCHIVE_INPUT_INVALID', 400, 'input'], ['REPORT_ARCHIVE_BUSY', 409, 'busy'],
    ['REPORT_ARCHIVE_NOT_FOUND', 404, 'missing'], ['SERVICE_UNAVAILABLE', 503, 'unavailable'],
    ['INVALID_RESPONSE', 502, 'unknown'], ['NETWORK_ERROR', 0, 'network'],
    ['AUTH_SESSION_REPLACED', 401, 'replaced'], ['BET_CONFIRMATION_ACCOUNT_CHANGED', 403, 'accountChanged'],
  ])('keeps the business meaning of %s without changing the failure', (code, status, key) => {
    const cause = Object.freeze({ code, status, message: 'SQL private detail / secret-token' })
    expect(publicApiErrorKey(cause)).toBe(key)
    for (const locale of ['en', 'zh-CN'] as const) {
      const text = publicApiErrorText(cause, locale)
      expect(text).toBe(publicApiErrorCatalog[locale][key as keyof typeof publicApiErrorCatalog.en])
      expect(text).not.toMatch(/SQL|secret-token|[A-Z]+_[A-Z_]+/)
    }
    expect(cause).toEqual({ code, status, message: 'SQL private detail / secret-token' })
  })

  it.each([null, 'private raw text', new Error('Failed to fetch / SQL'), { code: 'NEW_UNDOCUMENTED_ERROR', status: 418 }])('never renders unknown technical details: %s', cause => {
    for (const locale of ['en', 'zh-CN'] as const) {
      expect(publicApiErrorText(cause, locale)).toBe(publicApiErrorCatalog[locale].failed)
    }
  })

  it('does not present an unconfirmed write as a confirmed failure or success', () => {
    expect(publicApiErrorText({ code: 'NEW_ERROR', status: 503 }, 'zh-CN')).toContain('无法确认操作结果')
    expect(publicApiErrorText({ status: 0 }, 'en')).toContain('before retrying')
  })

  it('uses the current catalog language', () => {
    const i18n = createCatalogI18n(publicApiErrorCatalog)
    const cause = ref({ code: 'ROLE_ASSIGNED', status: 409 })
    const text = computed(() => i18n.errorText(cause.value))
    expect(text.value).toBe('角色仍被账号使用，请先解除关联后再删除')
    i18n.setLocale('en')
    expect(text.value).toBe(publicApiErrorCatalog.en.assigned)
  })

  it('stores a bilingual error and distinguishes failed reads from unconfirmed writes', () => {
    const i18n = createCatalogI18n(publicApiErrorCatalog)
    const cause = { code: 'NEW_ERROR', status: 503, message: 'private detail' }
    const write = i18n.errorMessage(cause)
    const read = i18n.errorMessage(cause, 'read')
    const shown = computed(() => i18n.displayError(write))
    expect(shown.value).toBe(publicApiErrorCatalog['zh-CN'].unknown)
    expect(i18n.displayError(read)).toBe(publicApiErrorCatalog['zh-CN'].readFailed)
    i18n.setLocale('en')
    expect(shown.value).toBe(publicApiErrorCatalog.en.unknown)
    expect(i18n.displayError(read)).toBe(publicApiErrorCatalog.en.readFailed)
  })
})
