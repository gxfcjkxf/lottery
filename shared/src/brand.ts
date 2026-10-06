export type Language = 'en' | 'zh'
export type LocaleCode = 'en' | 'zh-CN'
export type BrandFontFamily = 'system' | 'serif' | 'mono'
export type BrandFontScale = 'compact' | 'standard' | 'large'
export type BrandRadius = 'square' | 'soft' | 'round'
export type BrandShadow = 'none' | 'subtle' | 'lifted'

export interface BrandLocaleContent {
  tagline: string
  announcement: string
}

export interface BrandTheme {
  name: string
  logoText: string
  logoUrl?: string | null
  faviconUrl?: string | null
  primary: string
  accent: string
  success: string
  warning: string
  danger: string
  fontFamily: string
  fontScale?: BrandFontScale
  radius: string
  shadow: string
  content?: Partial<Record<Language, BrandLocaleContent>>
  /** @deprecated Use faviconUrl. */
  favicon?: string
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
  name: 'Luma Play', logoText: 'L', logoUrl: null, faviconUrl: null,
  primary: '#20594c', accent: '#d9ef9b', success: '#287b59', warning: '#a96d20', danger: '#bc4b43',
  fontFamily: 'system', fontScale: 'standard', radius: 'round', shadow: 'subtle',
  content: {
    en: { tagline: 'Lottery platform', announcement: '' },
    zh: { tagline: '彩票平台', announcement: '' },
  },
  defaultLanguage: 'en', languages: ['en', 'zh'], version: 'prototype-1',
}

export function resolveBrand(base: BrandTheme, override: Partial<BrandTheme> = {}): BrandTheme {
  return { ...base, ...override, languages: override.languages ?? base.languages }
}
