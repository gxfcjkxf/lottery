import { supportedLocale } from './localization'
import type { LocaleCode } from './brand'

export const LOCALE_COOKIE_NAME = 'lottery-locale'
const LOCALE_COOKIE_MAX_AGE = 60 * 60 * 24 * 365

export function readLocaleCookie(cookie = typeof document === 'undefined' ? '' : document.cookie): LocaleCode | null {
  const value = cookie.split(';').map((item) => item.trim()).find((item) => item.startsWith(`${LOCALE_COOKIE_NAME}=`))?.slice(LOCALE_COOKIE_NAME.length + 1)
  if (!value) return null
  return supportedLocale(decodeURIComponent(value))
}

export function writeLocaleCookie(locale: LocaleCode): void {
  document.cookie = `${LOCALE_COOKIE_NAME}=${encodeURIComponent(locale)}; Max-Age=${LOCALE_COOKIE_MAX_AGE}; Path=/; SameSite=Lax`
}

export function detectBrowserLocale(languages: readonly string[] = typeof navigator === 'undefined' ? [] : navigator.languages.length ? navigator.languages : [navigator.language]): LocaleCode {
  return languages.some((language) => language.toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en'
}

export function preferredLocale(): LocaleCode {
  return readLocaleCookie() ?? detectBrowserLocale()
}
