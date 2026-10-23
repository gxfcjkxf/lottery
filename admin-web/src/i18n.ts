import { inject, readonly, ref, type InjectionKey } from 'vue'
import { createCatalogI18n, localizedMessage, supportedLocale, translateMessage, type LocaleCode, type LocalizedMessage, type MessageParameters, type TranslationCatalog } from '@lottery/shared'
import { adminCatalog } from './i18n-catalog'

export const ADMIN_LOCALE_STORAGE_KEY = 'lottery.admin.locale'
interface PreferenceStore { getItem(key: string): string | null; setItem(key: string, value: string): void }
type AdminTranslationKey = keyof typeof adminCatalog.en

export function createAdminI18n(storage: PreferenceStore | null = null) {
  let preference: LocaleCode | null = null
  try { preference = supportedLocale(storage?.getItem(ADMIN_LOCALE_STORAGE_KEY)) } catch { /* Optional browser storage may be unavailable. */ }
  const base = createCatalogI18n(adminCatalog, preference ?? 'zh-CN')
  const messages = adminCatalog as TranslationCatalog
  const availableLocales = ref<LocaleCode[]>(['zh-CN', 'en'])
  let defaultLocale: LocaleCode = 'zh-CN'

  function configure(defaultValue: unknown, availableValues: readonly unknown[]) {
    const accepted = [...new Set(availableValues.map(supportedLocale).filter((value): value is LocaleCode => value !== null))]
    const configuredDefault = supportedLocale(defaultValue)
    defaultLocale = configuredDefault ?? 'zh-CN'
    if (!accepted.includes(defaultLocale)) accepted.unshift(defaultLocale)
    availableLocales.value = accepted
    base.setLocale(preference && accepted.includes(preference) ? preference : defaultLocale)
  }

  function setLocale(value: unknown) {
    const accepted = supportedLocale(value)
    if (!accepted || !availableLocales.value.includes(accepted)) return false
    preference = accepted
    base.setLocale(accepted)
    try { storage?.setItem(ADMIN_LOCALE_STORAGE_KEY, accepted) } catch { /* Translation is not dependent on persistence. */ }
    return true
  }

  function t(value: AdminTranslationKey | LocalizedMessage, parameters?: MessageParameters) {
    if (typeof value !== 'string') return translateMessage(base.locale.value, value, undefined, parameters)
    return base.t(value, parameters)
  }

  function message(value: AdminTranslationKey | LocalizedMessage, parameters?: MessageParameters): LocalizedMessage {
    if (typeof value !== 'string') return value
    const zh = messages['zh-CN'][value]
    const en = messages.en[value]
    if (zh === undefined || en === undefined) throw new Error(`Missing admin translation key: ${value}`)
    return localizedMessage(zh, en, parameters)
  }

  function display(value: string | LocalizedMessage) {
    return typeof value === 'string' ? value : translateMessage(base.locale.value, value)
  }

  function resetBrand() { configure('zh-CN', ['zh-CN', 'en']) }
  return { locale: readonly(base.locale), availableLocales: readonly(availableLocales), configure, resetBrand, setLocale, t, message, display, errorText: base.errorText, errorMessage: base.errorMessage, displayError: base.displayError }
}

export type AdminI18n = ReturnType<typeof createAdminI18n>
export type { AdminTranslationKey }
export const adminKey = (value: string) => value as AdminTranslationKey
export const adminI18nKey: InjectionKey<AdminI18n> = Symbol('admin-i18n')
export function useAdminI18n(): AdminI18n {
  const context = inject(adminI18nKey)
  if (!context) throw new Error('Admin locale provider is missing')
  return context
}
