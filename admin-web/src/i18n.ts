import { inject, readonly, ref, type InjectionKey } from 'vue'
import { localizedMessage, supportedLocale, translateMessage, type LocaleCode, type LocalizedMessage, type MessageParameters } from '@lottery/shared'

export const ADMIN_LOCALE_STORAGE_KEY = 'lottery.admin.locale'
interface PreferenceStore { getItem(key: string): string | null; setItem(key: string, value: string): void }

export function createAdminI18n(storage: PreferenceStore | null = null) {
  let preference: LocaleCode | null = null
  try { preference = supportedLocale(storage?.getItem(ADMIN_LOCALE_STORAGE_KEY)) } catch { /* Optional browser storage may be unavailable. */ }
  const locale = ref<LocaleCode>(preference ?? 'zh-CN')
  const availableLocales = ref<LocaleCode[]>(['zh-CN', 'en'])
  let defaultLocale: LocaleCode = 'zh-CN'
  function configure(defaultValue: unknown, availableValues: readonly unknown[]) {
    const accepted = [...new Set(availableValues.map(supportedLocale).filter((value): value is LocaleCode => value !== null))]
    defaultLocale = supportedLocale(defaultValue) ?? 'zh-CN'
    if (!accepted.includes(defaultLocale)) accepted.push(defaultLocale)
    availableLocales.value = accepted
    locale.value = preference && accepted.includes(preference) ? preference : defaultLocale
  }
  function setLocale(value: unknown) {
    const accepted = supportedLocale(value)
    if (!accepted || !availableLocales.value.includes(accepted)) return false
    preference = accepted
    locale.value = accepted
    try { storage?.setItem(ADMIN_LOCALE_STORAGE_KEY, accepted) } catch { /* Translation is not dependent on persistence. */ }
    return true
  }
  function resetBrand() { configure('zh-CN', ['zh-CN', 'en']) }
  function t(value: string | LocalizedMessage, english?: string, parameters?: MessageParameters) {
    return translateMessage(locale.value, value, english, parameters)
  }
  return { locale: readonly(locale), availableLocales: readonly(availableLocales), configure, resetBrand, setLocale, t, message: localizedMessage }
}

export type AdminI18n = ReturnType<typeof createAdminI18n>
export const adminI18nKey: InjectionKey<AdminI18n> = Symbol('admin-i18n')
export function useAdminI18n(): AdminI18n {
  const context = inject(adminI18nKey)
  if (!context) throw new Error('Admin locale provider is missing')
  return context
}
