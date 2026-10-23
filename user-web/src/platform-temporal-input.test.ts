import { createSSRApp } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { afterEach, expect, it } from 'vitest'
import { platformDisplaySettings, setPlatformDisplaySettings } from '@lottery/shared'
import Control from '@lottery/shared/PlatformTemporalInput.vue'
afterEach(() => { platformDisplaySettings.value = null })
it('shows the configured day-first date and localized twelve-hour controls without changing the canonical value', async () => {
  setPlatformDisplaySettings({ timezone: 'Asia/Singapore', date_format: 'DD/MM/YYYY', time_format: 'hh:mm:ss A' })
  const html = await renderToString(createSSRApp(Control, { type: 'datetime-local', modelValue: '2026-10-17T16:21:53', locale: 'zh-CN', required: true }))
  expect(html).toContain('value="17/10/2026"'); expect(html).toContain('value="2026-10-17T16:21:53"')
  expect(html).toContain('下午'); expect(html).toMatch(/value="04"[^>]*selected/)
  expect(html).not.toContain('type="datetime-local"')
})
it('supports optional disabled time controls and canonical minute precision', async () => {
  setPlatformDisplaySettings({ timezone: 'UTC', date_format: 'MM/DD/YYYY', time_format: 'HH:mm:ss' })
  const html = await renderToString(createSSRApp(Control, { type: 'time', modelValue: '16:21', locale: 'en', disabled: true }))
  expect(html).toMatch(/value="16"[^>]*selected/); expect(html).toMatch(/value="00"[^>]*selected/)
  expect(html).toContain('disabled'); expect(html).not.toContain('required')
})
