import { inject, readonly, ref, type InjectionKey } from 'vue'
import type { LocaleCode } from './brand'
import { publicApiErrorCatalog, publicApiErrorKey, publicApiErrorText } from './public-api-errors'

export type MessageParameters = Readonly<Record<string, string | number>>
export type TranslationCatalog = Readonly<Record<LocaleCode, Readonly<Record<string, string>>>>
export type CatalogI18n = ReturnType<typeof createCatalogI18n>
export interface LocalizedMessage {
  readonly zh: string
  readonly en: string
  readonly parameters?: MessageParameters
}

/** Store display messages, not translated strings, so in-flight notices follow the current locale. */
export function localizedMessage(zh: string, en: string, parameters?: MessageParameters): LocalizedMessage {
  return Object.freeze({ zh, en, parameters: parameters ? Object.freeze({ ...parameters }) : undefined })
}

/** Substitute once: values are plain text, never HTML, keys, or recursively evaluated templates. */
export function translateMessage(locale: LocaleCode, value: string | LocalizedMessage, english?: string, parameters?: MessageParameters): string {
  const source = typeof value === 'string' ? (locale === 'en' ? english ?? value : value) : value[locale === 'en' ? 'en' : 'zh']
  const values = typeof value === 'string' ? parameters : value.parameters
  return source.replace(/\{([A-Za-z][A-Za-z0-9_]*)\}/g, (placeholder, key: string) =>
    values && Object.prototype.hasOwnProperty.call(values, key) ? String(values[key]) : placeholder)
}

export function supportedLocale(value: unknown): LocaleCode | null {
  return value === 'en' || value === 'zh-CN' ? value : null
}

/** Language names are proper names and stay in their native language. */
export function nativeLocaleName(locale: LocaleCode): string {
  return locale === 'en' ? 'English' : '简体中文'
}

/** Application-level i18n for interface catalogs. Missing keys are errors. */
export function createCatalogI18n(catalog: TranslationCatalog, initial: LocaleCode = 'zh-CN') {
  const expectedKeys = Object.keys(catalog.en).sort()
  for (const locale of ['en', 'zh-CN'] as const) {
    const actualKeys = Object.keys(catalog[locale]).sort()
    if (actualKeys.length !== expectedKeys.length || actualKeys.some((key, index) => key !== expectedKeys[index])) {
      throw new Error(`Translation catalog key mismatch for ${locale}`)
    }
  }
  const locale = ref<LocaleCode>(initial)
  const setLocale = (value: unknown): boolean => {
    const next = supportedLocale(value)
    if (!next) return false
    locale.value = next
    return true
  }
  const t = (key: string, parameters?: MessageParameters): string => {
    const value = catalog[locale.value][key]
    if (value === undefined) throw new Error(`Missing ${locale.value} translation key: ${key}`)
    return value.replace(/\{([A-Za-z][A-Za-z0-9_]*)\}/g, (placeholder, name: string) =>
      parameters && Object.prototype.hasOwnProperty.call(parameters, name) ? String(parameters[name]) : placeholder)
  }
  const errorText = (cause: unknown): string => publicApiErrorText(cause, locale.value)
  const errorMessage = (cause: unknown, operation: 'read' | 'write' = 'write'): LocalizedMessage => {
    let key = publicApiErrorKey(cause)
    if (operation === 'read' && (key === 'unknown' || key === 'network')) key = 'readFailed'
    return localizedMessage(publicApiErrorCatalog['zh-CN'][key], publicApiErrorCatalog.en[key])
  }
  const displayError = (value: string | LocalizedMessage): string => translateMessage(locale.value, value)
  return { locale: readonly(locale), setLocale, t, errorText, errorMessage, displayError }
}

export function createI18nKey<T extends object>(name: string): InjectionKey<T> {
  return Symbol(name)
}

export function useCatalogI18n<T extends CatalogI18n>(key: InjectionKey<T>): T {
  const context = inject(key)
  if (!context) throw new Error('I18n provider is missing')
  return context
}
