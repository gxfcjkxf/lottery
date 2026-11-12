export const contactKinds = ['wechat', 'telegram', 'whatsapp'] as const
export type ContactKind = typeof contactKinds[number]
export interface BrandContact { id: string; kind: ContactKind; value: string; image_url: string | null }
export interface BrandContactConfig { items: BrandContact[]; default_id: string | null }
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export const contactAssetPattern = /^\/api\/v1\/brand-assets\/[a-f0-9]{64}\.(png|jpg)$/
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)
const keys = (value: Record<string, unknown>, names: string[]) => Object.keys(value).length === names.length && names.every(key => Object.hasOwn(value, key))
export function isBrandContactConfig(value: unknown): value is BrandContactConfig {
  if (!object(value) || !keys(value, ['items', 'default_id']) || !Array.isArray(value.items)) return false
  const ids = new Set<string>()
  for (const item of value.items) {
    if (!object(item) || !keys(item, ['id', 'kind', 'value', 'image_url']) || typeof item.id !== 'string' || !uuid.test(item.id) || ids.has(item.id) || !contactKinds.includes(item.kind as ContactKind) || typeof item.value !== 'string' || item.value !== item.value.trim() || Array.from(item.value).length > 120 || /[\u0000-\u001f\u007f-\u009f]/.test(item.value) || !(item.image_url === null || typeof item.image_url === 'string' && contactAssetPattern.test(item.image_url)) || !item.value && item.image_url === null) return false
    ids.add(item.id)
  }
  return ids.size === 0 ? value.default_id === null : typeof value.default_id === 'string' && ids.has(value.default_id)
}
