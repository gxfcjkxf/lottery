import { describe, expect, it } from 'vitest'
import { createCatalogI18n } from './localization'
import { formConstraintMessage, formValidationCatalog } from './form-validation'

describe('inline form validation translations', () => {
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
      expect(formConstraintMessage({ [flag]: true } as unknown as ValidityState, locale))
        .toBe(formValidationCatalog[locale][key])
    })
  }
  it('reports malformed numeric input before its missing value', () => {
    expect(formConstraintMessage({ badInput: true, valueMissing: true } as ValidityState, 'zh-CN'))
      .toBe(formValidationCatalog['zh-CN'].number)
  })
})
