import { describe, expect, it } from 'vitest'
import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import BusinessDetails from '@lottery/shared/BusinessDetails.vue'
import { businessDetailsCatalog } from '../../shared/src/business-details-catalog'
import { createCatalogI18n } from '@lottery/shared'

async function render(value: unknown, locale: 'en' | 'zh-CN' = 'zh-CN') {
  return renderToString(createSSRApp({ render: () => h(BusinessDetails, { value, locale }) }))
}
describe('business details presentation', () => {
  it('shows business changes without internal request data or private fields', async () => {
    const value = { points: '9007199254740993', status: 'active', enabled: false,
      config: { min_points: '10', max_points: '100', review_mode: 'manual' },
      idempotency_key: 'private-write-key', definition_hash: 'private-rule-hash', version: 3,
      password: 'private-password', actor_id: 'private-actor', request_id: 'private-request' }
    const html = await render(value)
    expect(html).toContain('9007199254740993')
    expect(html).toContain('最低积分'); expect(html).toContain('人工'); expect(html).toContain('否')
    expect(html).not.toMatch(/private-|idempotency|definition_hash|actor_id|request_id|password|<pre/)
    expect(value.version).toBe(3); expect(value.password).toBe('private-password')
    const english = await render(value, 'en')
    expect(english).toContain('Minimum points'); expect(english).toContain('Manual'); expect(english).toContain('No')
  })
  it('renders nested point sources, draw numbers and history as business facts', async () => {
    const html = await render({ before: { recharge: { available: '37' } }, after: { recharge: { available: '48' } }, regular: [1, 2, 3], special: [7] })
    expect(html).toContain('变更前'); expect(html).toContain('变更后'); expect(html).toContain('充值')
    expect(html).toContain('37'); expect(html).toContain('48'); expect(html).toContain('特别号码')
    expect(html).not.toContain('available')
  })
  it('does not expose technical values as ordinary text or imply success for unknown data', async () => {
    expect(await render({ status: 'PRIVATE_SYSTEM_ERROR', reason: '{"sql":"private"}' })).not.toMatch(/PRIVATE_SYSTEM_ERROR|sql|private/)
    expect(await render({ internal_only: 'private' })).toContain('暂无业务明细')
    expect(await render({ status: 'PRIVATE_SYSTEM_ERROR' })).toContain('请联系管理员')
  })
  it('has matching supported language catalogs', () => {
    expect(() => createCatalogI18n(businessDetailsCatalog)).not.toThrow()
  })
})

import ConfiguredNodeEditor from '../../shared/src/ConfiguredNodeEditor.vue'
it('uses business names for rule operations while preserving saved enum values', async () => {
  const value = { op: 'match_count', counting: 'occurrences', rounding: 'half_up' }
  const html = await renderToString(createSSRApp({ render: () => h(ConfiguredNodeEditor, { modelValue: value, locale: 'zh-CN' }) }))
  expect(html).toContain('value="match_count"'); expect(html).toContain('命中数量')
  expect(html).toContain('按出现次数'); expect(html).toContain('四舍五入')
  expect(value).toEqual({ op: 'match_count', counting: 'occurrences', rounding: 'half_up' })
})
it('shows ordinary number positions starting at one without changing stored positions', async () => {
  const positions = [0, 2]
  const html = await renderToString(createSSRApp({ render: () => h(ConfiguredNodeEditor, { modelValue: positions, field: 'positions', locale: 'zh-CN' }) }))
  expect(html).toContain('value="1"'); expect(html).toContain('value="3"')
  expect(positions).toEqual([0, 2])
})
