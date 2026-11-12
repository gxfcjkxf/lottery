import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { type ConfiguredCatalog } from '../../shared/src/configured-rules'

const origin = `http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE??15273}`
const catalog = JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json', 'utf8')) as ConfiguredCatalog
async function noOverflow(page: import('@playwright/test').Page) { expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true) }

for (const locale of ['en', 'zh-CN'] as const) {
  test(`configured editor and portable draft export (${locale})`, async ({ page }, info) => {
    const zh = locale === 'zh-CN', errors: string[] = [], requests: unknown[] = []
    page.on('pageerror', error => errors.push(error.message))
    // Browser component harness mounts the actual shared editor with synthetic HTTP receipts.
    // Go configured_preview_test executes the same DTO through the real calculation engine.
    await page.route('**/__configured-rule-harness', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html lang="${locale}"><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import {createApp} from '/node_modules/.vite/deps/vue.js';import Workbench from '/@fs/${resolve('shared/src/ConfiguredRuleWorkbench.vue')}';import '/@fs/${resolve('shared/src/styles.css')}';import '/@fs/${resolve('platform-web/src/style.css')}';import '/@fs/${resolve('shared/src/controls.css')}';import '/@fs/${resolve('shared/src/management.css')}';createApp(Workbench,{surface:'platform',brandId:'',locale:${JSON.stringify(locale)}}).mount('#app');</script></html>` }))
    await page.route('**/api/**', route => {
      const req = route.request(), path = new URL(req.url()).pathname
      const reply = (data: unknown) => route.fulfill({ json: { success: true, data } })
      if (path === '/api/v1/display-settings') return reply({ theme:'jade',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss' })
      if (path.endsWith('/rule-preview/template')) return reply(catalog)
      expect(path).toBe('/api/v1/platform/platform-games/rule-preview')
      const input = req.postDataJSON(); requests.push(input)
      const response: Record<string,unknown> = { definition_hash:'definition',snapshot_hash:'snapshot',price_keys:Object.keys(input.snapshot.prices.plates.A) }
      if (input.requests.length) {
        expect(input.requests[0].play_id).toBe('operator_lucky')
        expect(input.requests[0].options).toEqual(['choice'])
        response.quote = {bet_points:'10',lines:[{play_id:'operator_lucky',options:['choice'],plate:'A',stake:'10',prices:{hit:'2.5'}}]}
        if (input.draw) response.result = {bet_points:'10',prize_points:'25',refund_points:'0',effective_points:'10',lines:[{play_id:'operator_lucky',options:['choice'],outcome:'won',prize_points:'25',refund_points:'0'}],rounding:[]}
      }
      return reply(response)
    })
    await page.goto(`${origin}/__configured-rule-harness`)
    const editor = page.getByTestId('configured-workbench')
    await editor.locator('> summary').click()
    await page.getByRole('button', {name:zh?'载入通用示例配置':'Load sample template',exact:true}).click()
    await expect(editor.locator('tbody tr')).toHaveCount(87)
    await editor.getByRole('button',{name:zh?'添加玩法':'Add play',exact:true}).click()
    await page.getByLabel(zh?'新玩法标识':'New play ID',{exact:true}).fill('operator_copy')
    await page.getByRole('button',{name:zh?'继续配置':'Continue',exact:true}).click()
    await page.getByRole('button',{name:zh?'应用配置':'Apply configuration',exact:true}).click()
    await editor.getByRole('button',{name:zh?'添加玩法':'Add play',exact:true}).click()
    await page.getByLabel(zh?'新玩法标识':'New play ID',{exact:true}).fill('operator_lucky')
    await page.getByRole('button',{name:zh?'新建号码组':'New number group',exact:true}).click()
    await page.getByLabel(zh?'英文名称':'English name',{exact:true}).fill('Lucky group')
    await page.getByLabel(zh?'中文名称':'Chinese name',{exact:true}).fill('幸运组')
    await page.getByLabel(zh?'覆盖号码':'Covered numbers',{exact:true}).fill('2,7,19,31')
    await selectControl(page.getByRole('combobox',{name:zh?'开奖范围':'Draw scope',exact:true}), 'regular')
    await page.getByLabel(zh?'位置（从1起，留空为整个范围）':'Position (1 based, blank for full scope)',{exact:true}).fill('3')
    await page.getByLabel(zh?'此号码退本（留空则关闭）':'Push on this number (blank to disable)',{exact:true}).fill('49')
    await page.getByLabel(zh?'A盘赔率':'Odds for plate A',{exact:true}).fill('2.5')
    await page.getByRole('button',{name:zh?'继续配置':'Continue',exact:true}).click()
    await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
    await page.getByLabel(zh?'B盘赔率':'Odds for plate B',{exact:true}).fill('3.5')
    await page.getByRole('button',{name:zh?'继续配置':'Continue',exact:true}).click()
    await expect(page.getByRole('dialog')).toHaveAttribute('aria-label',zh?'幸运组':'Lucky group')
    await expect(page.getByRole('dialog').getByRole('alert')).toHaveCount(0)
    await page.getByRole('dialog').getByRole('button',{name:zh?'规则试算':'Test rules',exact:true}).click()
    await page.getByRole('button',{name:zh?'验证配置':'Validate configuration',exact:true}).click()
    const selection = page.getByTestId('configured-selection')
    await expect(selection).toBeVisible()
    await selection.locator('.configured-options button').first().click()
    await page.getByRole('button',{name:zh?'计算注数':'Calculate entries',exact:true}).click()
    await expect(page.getByRole('dialog').locator('dd').first()).toHaveText('1')
    await page.getByLabel(zh?'开奖正码':'Regular draw numbers',{exact:true}).fill('1,3,7,4,5,6')
    await page.getByRole('button',{name:zh?'模拟开奖':'Simulate draw',exact:true}).click()
    await expect(page.getByRole('dialog')).toContainText(zh?'中奖':'Won')
    await noOverflow(page)
    await page.getByRole('button',{name:zh?'应用配置':'Apply configuration',exact:true}).click()
    const download = page.waitForEvent('download', { timeout: 10000 })
    await editor.locator('summary').filter({hasText:zh?'模板与导入':'Templates and import'}).click()
    await page.getByRole('button',{name:zh?'导出规则':'Export rules',exact:true}).click()
    const file = await download
    const saved = JSON.parse(readFileSync((await file.path())!,'utf8'))
    expect(saved.catalog.plays).toHaveLength(89)
    expect(saved.catalog.plays.find((p:{id:string})=>p.id==='operator_copy')).toBeTruthy()
    await page.screenshot({path:info.outputPath(`editor-${locale}.png`),fullPage:true})
    await editor.getByRole('button',{name:zh?'幸运组':'Lucky group',exact:true}).click()
    await page.getByRole('dialog').getByLabel(zh?'分组':'Group',{exact:true}).fill('new_group')
    await page.getByRole('dialog').getByRole('button',{name:zh?'规则试算':'Test rules',exact:true}).click()
    await expect(selection).toHaveCount(0)
    expect(requests.length).toBe(3)
    await noOverflow(page)
    expect(errors).toEqual([])
  })
}
