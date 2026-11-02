import { describe, expect, it } from 'vitest'
import { createCatalogI18n } from './localization'
import { formConstraintMessage, formValidationCatalog } from './form-validation'

describe('inline form validation translations', () => {
  const constraints = { field: 'Password', minLength: '6', maxLength: '30', min: '1', max: '100', step: '0.5' }
  it('has matching catalogs for all supported languages', () => {
    expect(() => createCatalogI18n(formValidationCatalog)).not.toThrow()
  })
  const cases = [
    ['valueMissing', 'required'], ['badInput', 'number'],
    ['typeMismatch', 'format'], ['patternMismatch', 'format'],
    ['tooShort', 'short'], ['tooLong', 'long'],
    ['rangeUnderflow', 'minimum'], ['rangeOverflow', 'maximum'],
    ['stepMismatch', 'step'], ['customError', 'invalid'],
  ] as const
  for (const locale of ['en', 'zh-CN'] as const) for (const [flag, key] of cases) {
    it(`translates ${flag} using selected ${locale}, not browser validationMessage`, () => {
      expect(formConstraintMessage({ [flag]: true } as unknown as ValidityState, locale, constraints))
        .toBe(formValidationCatalog[locale][key].replace(/\{(\w+)\}/g, (_, name: keyof typeof constraints) => constraints[name]))
    })
  }
  it('states the password field and exact length requirement in both languages', () => {
    expect(formConstraintMessage({ tooShort: true } as ValidityState, 'zh-CN', { ...constraints, field: '密码' })).toBe('密码长度不得少于 6 个字符。')
    expect(formConstraintMessage({ tooLong: true } as ValidityState, 'en', constraints)).toBe('Password must contain no more than 30 characters.')
  })
  it('uses the actual field format and type requirements', () => {
    expect(formConstraintMessage({ patternMismatch: true } as ValidityState, 'en', { ...constraints, format: 'Use 3–32 lowercase letters, digits or underscores.' })).toContain('3–32')
    expect(formConstraintMessage({ typeMismatch: true } as ValidityState, 'zh-CN', { ...constraints, type: 'email' })).toContain('name@example.com')
    expect(formConstraintMessage({ stepMismatch: true } as ValidityState, 'en', constraints)).toBe('Password must use increments of 0.5.')
  })
  it('reports malformed numeric input before its missing value', () => {
    expect(formConstraintMessage({ badInput: true, valueMissing: true } as ValidityState, 'zh-CN', { ...constraints, field: '积分' }))
      .toBe('积分必须为有效数字。')
  })
})
