export type Language = 'en' | 'zh'
export type LocaleCode = 'en' | 'zh-CN'

export interface BrandLocaleContent {
  tagline: string
  announcement: string
}

export interface BrandTheme {
  name: string
  logoText: string
  logoUrl?: string | null
  faviconUrl?: string | null
  content?: Partial<Record<Language, BrandLocaleContent>>
  defaultLanguage: Language
  languages: Language[]
  version: string
}

export interface PlatformContext {
  brand: BrandTheme & { id?: string; code?: string; status?: string; timezone?: string }
  paused: boolean
  availableLanguages: string[]
  features?: { pwa: boolean; real_payments: boolean }
  configVersion?: number
}

export const defaultBrand: BrandTheme = {
  name: 'Luma Play', logoText: 'L', logoUrl: null, faviconUrl: null,
  content: {
    en: { tagline: 'Lottery platform', announcement: '' },
    zh: { tagline: '彩票平台', announcement: '' },
  },
  defaultLanguage: 'en', languages: ['en', 'zh'], version: 'prototype-1',
}
