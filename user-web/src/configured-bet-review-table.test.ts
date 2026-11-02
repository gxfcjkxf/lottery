import { readFileSync } from 'node:fs'
import { createSSRApp, h } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { describe, expect, it } from 'vitest'
import { configuredDemo, type ConfiguredQuotedLine } from '@lottery/shared'
import ConfiguredBetReviewTable from './ConfiguredBetReviewTable.vue'
import { createUserI18n, userI18nKey } from './i18n'

const snapshot = configuredDemo(JSON.parse(readFileSync(new URL('../../backend/internal/rules/config/mark_six_v2.json', import.meta.url), 'utf8')))
const play = snapshot.catalog.plays.find(play => play.id === 'parlay')!
const options = snapshot.catalog.domains[play.selection.domain]!.options
const selected = [options[0]!, options.find(option => option.slot !== options[0]!.slot)!]
const line: ConfiguredQuotedLine = {
  play_id: play.id, options: selected.map(option => option.id), plate: 'A', stake: '40', prices: {},
  legs: selected.map((option, index) => ({ play_id: option.reference!.play_id, options: [option.reference!.option_id], plate: 'A', stake: '40', prices: { hit: ['48.99', '13.14'][index]! } })),
}

describe('bet confirmation parlay prices', () => {
  for (const locale of ['en', 'zh-CN'] as const) it(`keeps each leg label separate from its unbroken price (${locale})`, async () => {
    const i18n = createUserI18n(); i18n.setLocale(locale)
    const html = await renderToString(createSSRApp({render: () => h(ConfiguredBetReviewTable, {snapshot, lines: [line], locale})}).provide(userI18nKey, i18n))
    expect([...html.matchAll(/class="bet-review-price-value"[^>]*>([^<]+)</g)].map(match => match[1])).toEqual(['48.99', '13.14'])
    expect(html.match(/class="bet-review-price-label"/g)).toHaveLength(2)
    for (const option of selected) expect(html).toContain(option.label[locale])
    expect(html).not.toContain('[object Object]')
  })
})
