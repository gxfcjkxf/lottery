import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { afterEach, expect, it } from 'vitest'
import { platformDisplaySettings, setPlatformDisplaySettings } from '@lottery/shared'
import Control from '@lottery/shared/PlatformTemporalInput.vue'
afterEach(() => { platformDisplaySettings.value = null })

function selectedTimeOptions(html: string) {
  return [...html.matchAll(/<div\b[^>]*role="option"[^>]*aria-selected="true"[^>]*>\s*<span\b[^>]*>([^<]*)<\/span>/g)].map(match => match[1])
}

function timeControlState(html: string) {
  const triggers = [...html.matchAll(/<button\b[^>]*role="combobox"[^>]*>/g)].map(([tag]) => tag)
  return [...html.matchAll(/<input\b[^>]*class="select-control-validation"[^>]*>/g)].map(([tag], index) => ({
    label: triggers[index]!.match(/aria-label="([^"]*)"/)![1],
    value: tag.match(/value="([^"]*)"/)![1],
    required: /\srequired(?:\s|=|>)/.test(tag),
    disabled: /\sdisabled(?:\s|=|>)/.test(tag),
  }))
}

it('shows the configured day-first date and localized twelve-hour controls without changing the canonical value', async () => {
  setPlatformDisplaySettings({ timezone: 'Asia/Singapore', date_format: 'DD/MM/YYYY', time_format: 'hh:mm:ss A' })
  const html = await renderToString(createSSRApp(Control, { type: 'datetime-local', modelValue: '2026-10-17T16:21:53', locale: 'zh-CN', required: true }))
  expect(html).toContain('value="17/10/2026"'); expect(html).toContain('value="2026-10-17T16:21:53"')
  expect(selectedTimeOptions(html)).toEqual(['04', '21', '53', '下午'])
  expect(timeControlState(html)).toEqual([
    { label: '时间 · 时', value: '04', required: true, disabled: false },
    { label: '时间 · 分', value: '21', required: true, disabled: false },
    { label: '时间 · 秒', value: '53', required: true, disabled: false },
    { label: '时间 · 上午/下午', value: 'PM', required: false, disabled: false },
  ])
  expect(html).not.toContain('type="datetime-local"')
})
it('supports optional disabled time controls and canonical minute precision', async () => {
  setPlatformDisplaySettings({ timezone: 'UTC', date_format: 'MM/DD/YYYY', time_format: 'HH:mm:ss' })
  const html = await renderToString(createSSRApp(Control, { type: 'time', modelValue: '16:21', locale: 'en', disabled: true }))
  expect(html).toContain('value="16:21"')
  expect(selectedTimeOptions(html)).toEqual(['16', '21', '00'])
  expect(timeControlState(html)).toEqual([
    { label: 'Time · Hour', value: '16', required: false, disabled: true },
    { label: 'Time · Minute', value: '21', required: false, disabled: true },
    { label: 'Time · Second', value: '00', required: false, disabled: true },
  ])
  const triggers = [...html.matchAll(/<button\b[^>]*role="combobox"[^>]*>/g)].map(([tag]) => tag)
  expect(triggers).toHaveLength(3)
  for (const trigger of triggers) expect(trigger).toMatch(/\sdisabled(?:\s|=|>)/)
  expect(html).not.toContain('required')
})
