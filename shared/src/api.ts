import { defaultBrand, type BrandLocaleContent, type Language, type PlatformContext } from './brand'
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
  message?: string
  code?: string
}

interface BackendContext {
  brand: {
    id: string
    code: string
    name: string
    status: string
    default_locale: string
    timezone: string
    theme?: {
      display_name?: unknown
      logo_text?: unknown
      logo_url?: unknown
      favicon_url?: unknown
      primary_color?: unknown
      accent_color?: unknown
      success_color?: unknown
      warning_color?: unknown
      danger_color?: unknown
      font_family?: unknown
      font_scale?: unknown
      radius?: unknown
      shadow?: unknown
      default_locale?: unknown
      available_locales?: unknown
      content?: unknown
    }
    config_version: number
  }
  available_locales: unknown[]
  features: { pwa: boolean; real_payments: boolean }
  terms: { privacy_policy_version: string; service_terms_version: string }
}

function isEnvelope<T>(value: unknown): value is ApiEnvelope<T> {
  return typeof value === 'object' && value !== null && 'success' in value
}

function normalizeLocale(value: unknown): Language | null {
  if (value === 'en') return 'en'
  if (value === 'zh-CN') return 'zh'
  return null
}

function normalizeLocaleCode(value: unknown): 'en' | 'zh-CN' | null {
  return value === 'en' || value === 'zh-CN' ? value : null
}

function localizedContent(value: unknown, locale: 'en' | 'zh-CN', fallback: BrandLocaleContent): BrandLocaleContent {
  if (typeof value !== 'object' || value === null) return fallback
  const entry = (value as Record<string, unknown>)[locale]
  if (typeof entry !== 'object' || entry === null) return fallback
  const record = entry as Record<string, unknown>
  return {
    tagline: typeof record.tagline === 'string' ? record.tagline : fallback.tagline,
    announcement: typeof record.announcement === 'string' ? record.announcement : fallback.announcement,
  }
}

function firstRune(value: string): string {
  return Array.from(value)[0] ?? 'L'
}

function enumValue<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  return typeof value === 'string' && (allowed as readonly string[]).includes(value) ? value as T : fallback
}

function toPlatformContext(value: BackendContext): PlatformContext {
  const theme = value.brand.theme ?? {}
  const localeSource = Array.isArray(theme.available_locales) ? theme.available_locales : value.available_locales
  const availableLanguages = [...new Set((Array.isArray(localeSource) ? localeSource : [])
    .map(normalizeLocaleCode).filter((locale): locale is 'en' | 'zh-CN' => locale !== null))]
  const supportedLanguages: Language[] = [...new Set(availableLanguages.map(normalizeLocale).filter((locale): locale is Language => locale !== null))]
  if (!supportedLanguages.length) supportedLanguages.push('en')
  const requestedDefault = normalizeLocale(typeof theme.default_locale === 'string' ? theme.default_locale : value.brand.default_locale)
  const defaultLanguage = requestedDefault && supportedLanguages.includes(requestedDefault)
    ? requestedDefault
    : supportedLanguages.includes('en') ? 'en' : supportedLanguages[0]!
  const displayName = typeof theme.display_name === 'string' && theme.display_name.trim()
    ? theme.display_name
    : value.brand.name
  const logoText = typeof theme.logo_text === 'string' && theme.logo_text.trim()
    ? theme.logo_text
    : firstRune(displayName)
  return {
    brand: {
      ...defaultBrand,
      id: value.brand.id,
      code: value.brand.code,
      name: displayName,
      logoText,
      logoUrl: safeBrandAssetUrl(theme.logo_url),
      faviconUrl: safeBrandAssetUrl(theme.favicon_url),
      primary: safeBrandColor(theme.primary_color, defaultBrand.primary),
      accent: safeBrandColor(theme.accent_color, defaultBrand.accent),
      success: safeBrandColor(theme.success_color, defaultBrand.success),
      warning: safeBrandColor(theme.warning_color, defaultBrand.warning),
      danger: safeBrandColor(theme.danger_color, defaultBrand.danger),
      fontFamily: enumValue(theme.font_family, ['system', 'serif', 'mono'] as const, defaultBrand.fontFamily),
      fontScale: enumValue(theme.font_scale, ['compact', 'standard', 'large'] as const, defaultBrand.fontScale ?? 'standard'),
      radius: enumValue(theme.radius, ['square', 'soft', 'round'] as const, defaultBrand.radius),
      shadow: enumValue(theme.shadow, ['none', 'subtle', 'lifted'] as const, defaultBrand.shadow),
      content: {
        en: localizedContent(theme.content, 'en', defaultBrand.content!.en!),
        zh: localizedContent(theme.content, 'zh-CN', defaultBrand.content!.zh!),
      },
      defaultLanguage,
      languages: supportedLanguages,
      version: String(value.brand.config_version),
      status: value.brand.status,
      timezone: value.brand.timezone,
    },
    paused: value.brand.status === 'paused',
    availableLanguages,
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
    if (isEnvelope<T>(body)) {
      if (!response.ok || !body.success) {
        throw new ApiError(body.error?.message ?? body.message ?? `Request failed (${response.status})`, response.status, body.error?.code ?? body.code)
      }
      return body.data as T
    }
    if (!response.ok) throw new ApiError(`Request failed (${response.status})`, response.status)
    return body as T
  }

  return {
    request,
    getContext: async () => {
      const path = brandCode ? `/api/v1/b/${encodeURIComponent(brandCode)}/context` : '/api/v1/context'
      return toPlatformContext(await request<BackendContext>(path))
    },
  }
}

export type ApiClient = ReturnType<typeof createApiClient>
