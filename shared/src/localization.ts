import type { LocaleCode } from './brand'

export type MessageParameters = Readonly<Record<string, string | number>>
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
