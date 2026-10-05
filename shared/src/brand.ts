export type Language = 'en' | 'zh'

export interface BrandTheme {
  name: string
  logoText: string
  favicon?: string
  primary: string
  accent: string
  success: string
  warning: string
  danger: string
  fontFamily: string
  radius: string
  shadow: string
  defaultLanguage: Language
  languages: Language[]
  version: string
}

export interface PlatformContext {
  brand: BrandTheme & { id?: string; code?: string; status?: string; timezone?: string }
  paused: boolean
  availableLanguages: string[]
  features?: { pwa: boolean; real_payments: boolean }
  terms?: { privacy_policy_version: string; service_terms_version: string }
  configVersion?: number
}

export const defaultBrand: BrandTheme = {
  name: 'Luma Play', logoText: 'luma', primary: '#20594c', accent: '#d9ef9b',
  success: '#287b59', warning: '#a96d20', danger: '#bc4b43',
  fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  radius: '20px', shadow: '0 18px 55px rgba(28, 54, 44, .08)', defaultLanguage: 'en', languages: ['en', 'zh'], version: 'prototype-1',
}

export function resolveBrand(base: BrandTheme, override: Partial<BrandTheme> = {}): BrandTheme {
  return { ...base, ...override, languages: override.languages ?? base.languages }
}
