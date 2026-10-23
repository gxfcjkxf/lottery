import { describe, expect, it } from 'vitest'
import { createCatalogI18n } from '@lottery/shared'
import { platformExtraCatalog } from './i18n-catalog'

describe('access deletion notices', () => {
  it('identifies the deleted account or role and follows the selected language', () => {
    const i18n = createCatalogI18n(platformExtraCatalog, 'zh-CN')
    const name = 'test_001'
    expect(i18n.t('access.accountDeleted', { name })).toBe('删除账号「test_001」成功。')
    expect(i18n.t('access.roleDeleted', { name })).toBe('删除角色「test_001」成功。')
    i18n.setLocale('en')
    expect(i18n.t('access.accountDeleted', { name })).toBe('Account “test_001” deleted successfully.')
    expect(i18n.t('access.roleDeleted', { name })).toBe('Role “test_001” deleted successfully.')
  })
})
