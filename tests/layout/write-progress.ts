import {resolve} from 'node:path'
import {origins} from './fixture'
import {expect, test, type Page} from '@playwright/test'
import {expectNoPageOverflow} from './fixture'

// Called while the route is held open: assert the actual pending UI, before
// returning either a receipt or an unknown response. All APIs are intercepted.
export async function expectWriteProgress(page:Page, selector:string, phase:string) {
 const scope=page.locator(selector)
 await expect(page.locator('.global-loading-overlay')).toBeVisible()
 await expect(scope.getByRole('button',{name:/^(Retry|重试)$/})).toHaveCount(0)
 await expect.poll(()=>scope.locator('p').evaluateAll(nodes=>nodes.filter(node=>
  !node.closest('.toast') && node.getClientRects().length &&
  /unconfirmed|result.*unknown|结果.*确认|结果未知|响应未知/i.test(node.textContent??'')
 ).map(node=>node.textContent))).toEqual([])
 await expectNoPageOverflow(page)
 await page.screenshot({path:test.info().outputPath(`progress-${phase}.png`)})
}

const settings={timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}

export async function mountWriteProgress(page:Page,component:string,props:Record<string,unknown>,surface:'admin'|'platform',locale:string) {
 const origin=surface==='admin'?origins.brand:origins.platform
 const source=await(await page.request.get(origin+'/src/App.vue')).text()
 const vue=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 const i18n=surface==='admin'?"import{createAdminI18n as make,adminI18nKey as key}from'/src/i18n.ts';":"import{createPlatformI18n as make,platformI18nKey as key}from'/src/i18n.ts';"
 await page.route('**/__write-progress',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html lang="${locale}"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console" style="padding:12px;min-width:0"></main><script type="module">import{createApp,h,provide}from'${vue}';import C from'/@fs${resolve(component)}';import L from'/@fs${resolve('shared/src/LoadingOverlay.vue')}';import H from'/@fs${resolve('shared/src/OperationToastHost.vue')}';import{setPlatformDisplaySettings,setInlineValidationLocale}from'/@fs${resolve('shared/src/index.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/src/style.css';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/form-validation.css')}';${i18n}setPlatformDisplaySettings(${JSON.stringify(settings)});setInlineValidationLocale('${locale}');createApp({setup(){const i=make();i.setLocale('${locale}');provide(key,i);return()=>[h(C,${JSON.stringify(props)}),h(L,{label:'${locale==='en'?'Loading…':'加载中…'}'}),h(H,{locale:'${locale}'})]}}).mount('#app')</script></html>`}))
 await page.goto(origin+'/__write-progress')
 await expect(page.locator('#app button').first()).toBeVisible()
 await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
}
