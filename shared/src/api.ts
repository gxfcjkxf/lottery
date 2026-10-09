import type { BrandLocaleContent, BrandTheme, Language, PlatformContext } from './brand'
import { safeBrandAssetUrl, safeBrandColor } from './presentation'

export class ApiError extends Error {
  constructor(message: string, readonly status?: number, readonly code?: string) {
    super(message)
    this.name = 'ApiError'
  }
}

export interface ApiClientOptions {
  baseUrl?: string
  brandCode?: string
  fetcher?: typeof fetch
}

interface ApiEnvelope<T> {
  success: boolean
  data?: T
  error?: { code?: string; message?: string; details?: unknown }
}

interface BackendTheme {
  display_name: string
  logo_text: string
  logo_url: string | null
  favicon_url: string | null
  primary_color: string
  accent_color: string
  success_color: string
  warning_color: string
  danger_color: string
  font_family: 'system' | 'serif' | 'mono'
  font_scale: 'compact' | 'standard' | 'large'
  radius: 'square' | 'soft' | 'round'
  shadow: 'none' | 'subtle' | 'lifted'
  default_locale: 'en' | 'zh-CN'
  available_locales: ('en' | 'zh-CN')[]
  content: {
    en: BrandLocaleContent
    'zh-CN': BrandLocaleContent
  }
}

interface BackendContext {
  brand: {
    id: string
    code: string
    name: string
    status: string
    default_locale: 'en' | 'zh-CN'
    timezone: string
    theme: BackendTheme
    config_version: number
  }
  available_locales: ('en' | 'zh-CN')[]
  features: { pwa: boolean; real_payments: boolean }
  terms: { privacy_policy_version: string; service_terms_version: string }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isLocale(value: unknown): value is 'en' | 'zh-CN' {
  return value === 'en' || value === 'zh-CN'
}

function isLocaleList(value: unknown): value is ('en' | 'zh-CN')[] {
  return Array.isArray(value) && value.length > 0 && value.every(isLocale) && new Set(value).size === value.length
}

function isStringRecord(value: unknown, keys: readonly string[]): value is Record<string, string> {
  return isRecord(value) && keys.every(key => typeof value[key] === 'string')
}

function isBackendContext(value: unknown): value is BackendContext {
  if (!isRecord(value) || !isRecord(value.brand) || !isRecord(value.features) || !isRecord(value.terms)) return false
  const brand = value.brand
  const theme = brand.theme
  if (!isRecord(theme) || !isLocale(brand.default_locale) || !isLocale(theme.default_locale)) return false
  if (!isLocaleList(theme.available_locales) || !isLocaleList(value.available_locales)) return false
  if (!theme.available_locales.includes(theme.default_locale)) return false
  if (typeof theme.display_name !== 'string' || !theme.display_name.trim() || typeof theme.logo_text !== 'string' || !theme.logo_text.trim()) return false
  if (theme.logo_url !== null && typeof theme.logo_url !== 'string') return false
  if (theme.favicon_url !== null && typeof theme.favicon_url !== 'string') return false
  for (const color of ['primary_color', 'accent_color', 'success_color', 'warning_color', 'danger_color']) {
    if (typeof theme[color] !== 'string' || !/^#[\da-f]{6}$/i.test(theme[color] as string)) return false
  }
  if (!['system', 'serif', 'mono'].includes(theme.font_family as string)) return false
  if (!['compact', 'standard', 'large'].includes(theme.font_scale as string)) return false
  if (!['square', 'soft', 'round'].includes(theme.radius as string)) return false
  if (!['none', 'subtle', 'lifted'].includes(theme.shadow as string)) return false
  if (!isRecord(theme.content) || !isStringRecord(theme.content.en, ['tagline', 'announcement']) || !isStringRecord(theme.content['zh-CN'], ['tagline', 'announcement'])) return false
  if (typeof brand.id !== 'string' || typeof brand.code !== 'string' || typeof brand.name !== 'string' || typeof brand.status !== 'string' || typeof brand.timezone !== 'string' || typeof brand.config_version !== 'number') return false
  if (typeof value.features.pwa !== 'boolean' || typeof value.features.real_payments !== 'boolean') return false
  return typeof value.terms.privacy_policy_version === 'string' && typeof value.terms.service_terms_version === 'string'
}

function normalizeLocale(value: 'en' | 'zh-CN'): Language {
  return value === 'zh-CN' ? 'zh' : 'en'
}

function assetUrl(value: string | null): string | null {
  if (value === null) return null
  const url = safeBrandAssetUrl(value)
  if (url === null) throw new ApiError('Invalid brand context: unsafe asset URL')
  return url
}

function toPlatformContext(value: BackendContext): PlatformContext {
  const theme = value.brand.theme
  const languages = theme.available_locales.map(normalizeLocale)
  const brand: BrandTheme & { id?: string; code?: string; status?: string; timezone?: string } = {
    id: value.brand.id,
    code: value.brand.code,
    name: theme.display_name,
    logoText: theme.logo_text,
    logoUrl: assetUrl(theme.logo_url),
    faviconUrl: assetUrl(theme.favicon_url),
    primary: safeBrandColor(theme.primary_color),
    accent: safeBrandColor(theme.accent_color),
    success: safeBrandColor(theme.success_color),
    warning: safeBrandColor(theme.warning_color),
    danger: safeBrandColor(theme.danger_color),
    fontFamily: theme.font_family,
    fontScale: theme.font_scale,
    radius: theme.radius,
    shadow: theme.shadow,
    content: {
      en: theme.content.en,
      zh: theme.content['zh-CN'],
    },
    defaultLanguage: normalizeLocale(theme.default_locale),
    languages,
    version: String(value.brand.config_version),
    status: value.brand.status,
    timezone: value.brand.timezone,
  }
  return {
    brand,
    paused: value.brand.status === 'paused',
    availableLanguages: [...theme.available_locales],
    features: value.features,
    terms: value.terms,
    configVersion: value.brand.config_version,
  }
}

export function createApiClient({ baseUrl = '', brandCode, fetcher = fetch }: ApiClientOptions = {}) {
  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    let response: Response
    try {
      response = await fetcher(`${baseUrl}${path}`, {
        ...init,
        headers: { Accept: 'application/json', ...init.headers },
        credentials: 'same-origin',
      })
    } catch (error) {
      throw new ApiError(error instanceof Error ? error.message : 'Network unavailable')
    }
    const body: unknown = await response.json().catch(() => undefined)
    if (!isRecord(body) || typeof body.success !== 'boolean') throw new ApiError('Invalid API response', response.status)
    const envelope = body as unknown as ApiEnvelope<T>
    if (!response.ok || !envelope.success) {
      throw new ApiError(envelope.error?.message ?? `Request failed (${response.status})`, response.status, envelope.error?.code)
    }
    if (!('data' in body)) throw new ApiError('Invalid API response', response.status)
    return envelope.data as T
  }

  return {
    request,
    getContext: async () => {
      const path = brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}/context` : '/api/v1/context'
      const value: unknown = await request(path)
      if (!isBackendContext(value)) throw new ApiError('Invalid brand context response')
      return toPlatformContext(value)
    },
  }
}

export type ApiClient = ReturnType<typeof createApiClient>
