import { defineComponent, h, inject, toRaw, type InjectionKey, type PropType, type Ref } from 'vue'
import type { LocaleCode } from './brand'
import type { TranslationCatalog } from './localization'
import { formatDisplayDateTime } from './display-settings'

export type AssociationKind = 'brand' | 'role' | 'member' | 'user' | 'admin' | 'agent' | 'game' | 'period' | 'play' | 'actor' | 'cycle'
type Name = Readonly<{ name: string; built_in?: boolean; from?: string; to?: string }>
type Names = Readonly<Record<string, Name>>
// Exact response-object ownership prevents names leaking between brands,
// sessions or stale reads. No browser storage or global identifier cache.
const namesByRecord = new WeakMap<object, { names: Names; failed: boolean }>()
export const associationContextKey: InjectionKey<Readonly<Ref<object | null>>> = Symbol('association-context')

export function copyAssociationNames(source: unknown, target: unknown): void {
 if (!source || !target || typeof source !== 'object' || typeof target !== 'object') return
 const metadata = namesByRecord.get(toRaw(source))
 if (metadata) {
  const previous = namesByRecord.get(toRaw(target))
  namesByRecord.set(toRaw(target), previous ? {names:Object.freeze({...previous.names,...metadata.names}),failed:previous.failed || metadata.failed} : metadata)
 }
 for (const key of Object.keys(source)) copyAssociationNames((source as Record<string, unknown>)[key], (target as Record<string, unknown>)[key])
}

export const associationCatalog = {
 en: { 'association.builtinReadOnly': 'System built-in role. Changes are not allowed.', 'association.roleDetails': 'Role details', 'association.close': 'Close', 'association.unavailable': 'Unable to load associated information.', 'association.missing': 'Associated record is unavailable.', 'association.unnamed': 'Account name has not been set.', 'association.bootstrap': 'System administrator', 'association.roles': 'Roles', 'association.details': 'View details', 'association.member': 'Member account', 'association.agent': 'Agent account' },
 'zh-CN': { 'association.builtinReadOnly': '系统内置角色，不可修改', 'association.roleDetails': '角色详情', 'association.close': '关闭', 'association.unavailable': '无法读取关联信息。', 'association.missing': '关联记录当前不可用。', 'association.unnamed': '尚未设置账号名。', 'association.bootstrap': '系统管理员', 'association.roles': '角色', 'association.details': '查看详情', 'association.member': '会员账号', 'association.agent': '代理账号' },
} satisfies TranslationCatalog

/** Preserve the business response; attach display metadata outside DTO keys. */
export async function readApiJson(response: Pick<Response, 'json'>): Promise<any> {
 const value = await response.json()
 if (!value || typeof value !== 'object' || value.success !== true) return value
 const source = value.associations
 let failed = value.association_error === true || !source || typeof source !== 'object' || Array.isArray(source)
 const names: Record<string, Name> = {}
 if (!failed) for (const [key, entry] of Object.entries(source)) {
  if (!entry || typeof entry !== 'object' || typeof (entry as Name).name !== 'string' || ((entry as Name).built_in !== undefined && typeof (entry as Name).built_in !== 'boolean')) { failed = true; break }
  if (key.startsWith('cycle:') && (typeof (entry as Name).from !== 'string' || typeof (entry as Name).to !== 'string' || !Number.isFinite(Date.parse((entry as Name).from!)) || !Number.isFinite(Date.parse((entry as Name).to!)))) { failed = true; break }
  names[key] = Object.freeze({ ...(entry as Name) })
 }
 const metadata = { names: Object.freeze(names), failed }
 const visit = (item: unknown) => {
  if (!item || typeof item !== 'object') return
  namesByRecord.set(item, metadata)
  for (const child of Object.values(item)) visit(child)
 }
 visit(value.data)
 return value
}

export function associationLabel(record: object | null | undefined, kind: AssociationKind, id: string | null | undefined, locale: LocaleCode): string {
 if (!id) return '—'
 const text = associationCatalog[locale]
 const entity = record as Record<string, unknown> | null | undefined
 if (entity?.id === id) {
  if (kind === 'role' && entity.is_bootstrap === true) return text['association.bootstrap']
  if (['brand','role','game','play'].includes(kind) && typeof entity.name === 'string') return entity.name
  if (kind === 'period' && typeof entity.period_no === 'string') return entity.period_no
  if (['member','user','admin'].includes(kind) && typeof entity.username === 'string') return entity.username || text['association.unnamed']
 }
 const metadata = record && namesByRecord.get(toRaw(record))
 if (!metadata || metadata.failed) return text['association.unavailable']
 const entry = metadata.names[`${kind}:${id}`]
 if (!entry) return text['association.missing']
 if (kind === 'cycle' && entry.from && entry.to) return `${formatDisplayDateTime(entry.from, locale)} – ${formatDisplayDateTime(entry.to, locale)}`
 if (kind === 'role' && entry.built_in) return text['association.bootstrap']
 return entry.name || text['association.unnamed']
}

export function associationGroupLabel(record: object, group: string, key: string, label: string, locale: LocaleCode): string {
 if (['member','agent','game','cycle'].includes(group) && /^[0-9a-f-]{36}$/i.test(key)) {
  if (label !== key && label.trim()) return label
  return associationLabel(record, group as AssociationKind, key, locale)
 }
 return label
}

export const AssociationName = defineComponent({
 name: 'AssociationName',
 props: {
  record: { type: Object as PropType<object | null>, default: null },
  kind: { type: String as PropType<AssociationKind>, required: true },
  id: { type: String, default: '' },
  locale: { type: String as PropType<LocaleCode | 'zh'>, required: true },
 },
 setup: props => {
  const context = inject(associationContextKey, null)
  return () => h('span', { class: 'association-name' }, associationLabel(props.record ?? context?.value, props.kind, props.id, props.locale === 'zh' ? 'zh-CN' : props.locale))
 },
})
