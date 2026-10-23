import { describe, expect, it } from 'vitest'
import { notificationCatalog, resolveNotificationContent } from './notification-catalog'

describe('notification i18n references', () => {
  it('resolves both languages without changing the stored reference or sharing mutable catalog values', () => {
    const reference = Object.freeze({ i18n_key: 'member.joined', i18n_version: 1 })
    const copy = resolveNotificationContent(reference, 'member.joined')
    expect(copy['zh-CN'].body).toBe('您的会员账号已准备就绪，欢迎加入。')
    expect(copy.en.body).toBe('Your membership is ready. Welcome aboard.')
    copy['zh-CN'].body = 'Custom draft'
    expect(notificationCatalog['member.joined']['zh-CN'].body).toBe('您的会员账号已准备就绪，欢迎加入。')
    expect(Object.keys(reference)).toEqual(['i18n_key', 'i18n_version'])
  })
  it('rejects missing, mismatched, unsupported and mixed translation references', () => {
    for (const value of [
      { i18n_key: 'unknown', i18n_version: 1 },
      { i18n_key: 'recharge.confirmed', i18n_version: 1 },
      { i18n_key: 'member.joined', i18n_version: 2 },
      { i18n_key: 'member.joined' },
      { i18n_key: 'member.joined', i18n_version: 1, en: { title: 'Extra', body: 'Extra' } },
    ]) expect(() => resolveNotificationContent(value, 'member.joined')).toThrow()
  })
  it('keeps explicitly authored bilingual templates and event placeholders intact', () => {
    const custom = { en: { title: 'Receipt', body: '{points}: {resource_id}' }, 'zh-CN': { title: '回执', body: '{points}：{resource_id}' } }
    expect(resolveNotificationContent(custom, 'recharge.confirmed')).toEqual(custom)
  })
})
