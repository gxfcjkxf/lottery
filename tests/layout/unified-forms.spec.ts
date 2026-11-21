import {expectDialogFieldColumns} from './dialog-form-columns'
import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow,expectInsideViewport} from './fixture'
for(const locale of ['en','zh-CN'] as const)test(`four surfaces share filter and form geometry (${locale})`,async({page},info)=>{
 const zh=locale==='zh-CN',preset=page.viewportSize()!
 const measured:Record<string,unknown>={}
 for(const surface of ['user','brand','platform','agent'] as const){
  const directory=surface==='brand'?'admin-web':surface+'-web',origin=origins[surface]
  const source=await(await page.request.get(origin+'/src/'+(surface==='user'?'Page':'App')+'.vue')).text()
  const vuePath=source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  const url=origin+'/__unified-forms'
  await page.route(url,route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/UnifiedFormsHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';${surface==='agent'?'':`import'/@fs/${resolve(directory+'/src/style.css')}';`}${surface==='user'?`import'/@fs/${resolve('user-web/src/workspace.css')}';`:`import'/@fs/${resolve('shared/src/management.css')}';`}import'/@fs/${resolve('shared/src/key-value.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({locale,surface})}).mount('#app')</script></html>`}))
  await page.goto(url)
  const search=page.locator('.ui-filter-bar input'),query=page.locator('.ui-filter-bar button[type=submit]')
  await search.fill('member001');await query.click();await expect(page.getByTestId('query-count')).toHaveText('1')
  await page.locator('.ui-filter-bar').getByRole('button',{name:zh?'清除输入':'Clear input',exact:true}).click();await expect(search).toHaveValue('');await expect(search).toBeFocused();await expect(page.getByTestId('query-count')).toHaveText('1')
  const viewportSizes=info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]
  for(const width of viewportSizes){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectNoPageOverflow(page)
   const geometry=await page.locator('.ui-filter-bar').evaluate(el=>{
    const style=getComputedStyle(el),input=getComputedStyle(el.querySelector('input')!),button=getComputedStyle(el.querySelector('button')!)
    return {gap:style.gap,padding:style.padding,border:style.borderWidth,inputHeight:input.height,inputRadius:input.borderRadius,inputPadding:input.padding,buttonHeight:button.height,buttonRadius:button.borderRadius}
   })
   const mobile=await page.evaluate(()=>matchMedia('(max-width:900px), (pointer:coarse)').matches)
   const shortLandscape=await page.evaluate(()=>matchMedia('(max-height:450px) and (orientation:landscape)').matches)
   expect(geometry).toMatchObject({gap:mobile||shortLandscape?'4px 10px':'10px',padding:'0px',border:'0px'})
   const key=`filter-${width}`
   if(surface==='user')measured[key]=geometry;else expect(geometry).toEqual(measured[key])
   await page.getByRole('button',{name:zh?'创建会员':'Create member',exact:true}).click()
   const dialog=page.getByRole('dialog')
   await expectInsideViewport(dialog);await expectNoPageOverflow(page)
   expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(520)
   await expect(dialog.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeDisabled()
   await page.mouse.move(0,0)
   await dialog.locator('.detail-modal-close').evaluate(async el=>{getComputedStyle(el).backgroundColor;await Promise.all(el.getAnimations().map(animation=>animation.finished))})
   const divider=await dialog.evaluate(el=>{
    const heading=el.querySelector('.detail-modal-heading')!,content=el.querySelector('.detail-modal-content')!
    return {headingMargin:getComputedStyle(heading).margin,gap:content.getBoundingClientRect().top-heading.getBoundingClientRect().bottom,padding:getComputedStyle(content).paddingTop}
   })
   expect(divider).toEqual({headingMargin:'0px',gap:0,padding:mobile?'8px':'12px'})
   const form=await dialog.locator('.ui-form').evaluate(el=>{
    const s=getComputedStyle(el),label=getComputedStyle(el.querySelector('label')!),actions=getComputedStyle(el.querySelector('.ui-form-actions')!),input=getComputedStyle(el.querySelector('input')!)
    return {gap:s.rowGap,labelGap:label.rowGap,actionsJustify:actions.justifyContent,actionsDirection:actions.flexDirection,inputHeight:input.height,inputRadius:input.borderRadius,labelLineHeight:label.lineHeight,close:(()=>{const st=getComputedStyle(el.closest('dialog')!.querySelector('.detail-modal-close')!);return {width:st.width,height:st.height,border:st.borderWidth,background:st.backgroundColor,shadow:st.boxShadow}})(),fields:[...el.querySelectorAll('input:not([data-select-validation])')].map(field=>{const st=getComputedStyle(field);return {width:st.width,fontFamily:st.fontFamily,fontSize:st.fontSize,fontWeight:st.fontWeight,padding:st.padding}})}
   })
   expect(form).toMatchObject({gap:mobile?'8px':'12px',labelGap:mobile?'4px':'6px',actionsJustify:'flex-end',actionsDirection:'row'})
   const formKey=`form-${width}`
   if(surface==='user')measured[formKey]=form;else expect(form).toEqual(measured[formKey])
   await expectDialogFieldColumns(dialog)
   const actions=dialog.locator('.ui-form-actions'),submit=actions.getByRole('button')
   await expect(actions.getByRole('button')).toHaveCount(1)
   const box=(await submit.boundingBox())!,parent=(await actions.boundingBox())!
   expect(box.width).toBeLessThan(parent.width-30)
   if(!info.project.name.includes('reflow')||[320,1440].includes(width))await page.screenshot({path:info.outputPath(`unified-form-${surface}-${locale}-${width}.png`)})
   await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
  }
  if(info.project.name.includes('reflow'))await page.setViewportSize(preset)
 }
})

// Exercise the real creation dialog without writing to a shared development database.
for (const locale of ['en', 'zh-CN'] as const) test(`brand creation submits directly with Chinese default (${locale})`, async ({ page }, info) => {
 const { installLayoutApi, gameId } = await import('./fixture')
 const { installConsoleSession, openPlatformPage } = await import('./real-console-session')
 const zh = locale === 'zh-CN'
 await installLayoutApi(page, locale)
 await installConsoleSession(page, 'platform', locale, gameId, ['brand.create.platform'])
 const attempts: Array<{body: Record<string, unknown>; key: string | undefined}> = []
 await page.route('**/api/v1/platform/brands', async route => {
  if (route.request().method() !== 'POST') return route.fallback()
  attempts.push({body: route.request().postDataJSON(), key: route.request().headers()['idempotency-key']})
  if (attempts.length === 1) return route.fulfill({status: 503, json: {success: false, error: {code: 'SERVICE_UNAVAILABLE', message: 'Unavailable'}}})
  return route.fulfill({status: 201, json: {success: true, data: {...attempts[0].body, id: gameId, status: 'paused', version: 1, created_at: '2026-11-18T00:00:00Z', audit_log_id: gameId}}})
 })
 await page.goto(origins.platform)
 await expect(page.locator('.app-frame')).toBeVisible()
 await openPlatformPage(page, zh ? '品牌' : 'Brands')
 await openPlatformPage(page, zh ? '品牌列表' : 'Brand list')
 await page.getByRole('button', {name: zh ? '新建品牌' : 'New brand', exact: true}).click()
 const dialog = page.getByRole('dialog')
 const code = dialog.getByLabel(zh ? '品牌代码' : 'Brand code', {exact: true})
 await expect(code).toHaveValue('')
 await expect(code).not.toHaveAttribute('placeholder')
 await expect(dialog.getByRole('combobox')).toHaveAttribute('data-value', 'zh-CN')
 const preset = page.viewportSize()!
 for (const width of info.project.name.includes('reflow') ? [320, 620, preset.width] : [preset.width]) {
  if (info.project.name.includes('reflow')) await page.setViewportSize({width, height: preset.height})
  await expectNoPageOverflow(page)
  await expectInsideViewport(dialog.getByRole('button', {name: zh ? '提交' : 'Submit', exact: true}))
 }
 await code.fill('trial_brand')
 await dialog.getByLabel(zh ? '品牌名称' : 'Brand name', {exact: true}).fill('Trial Brand')
 await page.screenshot({path: info.outputPath('brand-create-direct.png')})
 await dialog.getByRole('button', {name: zh ? '提交' : 'Submit', exact: true}).click()
 const retry = dialog.getByRole('button', {name: zh ? '重试' : 'Retry', exact: true})
 await expect(retry).toBeVisible()
 expect(attempts).toHaveLength(1)
 expect(attempts[0].body).toEqual({code: 'trial_brand', name: 'Trial Brand', default_locale: 'zh-CN', timezone: 'Asia/Singapore'})
 expect(attempts[0].key).toBeTruthy()
 await page.locator('.operation-toast-stack .toast-close').click()
 await retry.click()
 await expect(dialog).toHaveCount(0)
 expect(attempts).toHaveLength(2)
 expect(attempts[1]).toEqual(attempts[0])
})

for (const locale of ['en', 'zh-CN'] as const) test(`domain add and edit save directly and preserve retry requests (${locale})`, async ({ page }, info) => {
 const { gameId } = await import('./fixture')
 const zh = locale === 'zh-CN', at = '2026-11-18T00:00:00Z'
 let record = {brand_id: gameId, status: 'active', version: 1, domains: [] as {id: string; domain: string; enabled: boolean}[], updated_at: at}
 const writes: Array<{method: string; body: unknown; key: string | undefined}> = []
 await page.route('**/api/**', route => {
  const req = route.request()
  if (req.method() === 'GET') return route.fulfill({json: {success: true, data: record}})
  writes.push({method: req.method(), body: req.postDataJSON(), key: req.headers()['idempotency-key']})
  if (writes.length === 1) return route.fulfill({status: 503, json: {success: false, error: {code: 'SERVICE_UNAVAILABLE', message: 'Unavailable'}}})
  const body = req.postDataJSON()
  expect(body.version).toBe(record.version)
  record = {...record, version: record.version + 1, domains: [{id: gameId, domain: 'play.example.com', enabled: body.enabled}]}
  return route.fulfill({status: req.method() === 'POST' ? 201 : 200, json: {success: true, data: {...record, audit_log_id: gameId}}})
 })
 const source = await (await page.request.get(origins.platform + '/src/App.vue')).text()
 const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
 await page.route('**/__direct-domain-form', route => route.fulfill({contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/ManagementListsHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('admin-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({kind: 'domains', id: gameId, locale})}).mount('#app')</script></html>`}))
 await page.goto(origins.platform + '/__direct-domain-form')
 await page.getByRole('button', {name: zh ? '添加域名' : 'Add domain', exact: true}).click()
 const dialog = page.getByRole('dialog'), save = dialog.getByRole('button', {name: zh ? '保存' : 'Save', exact: true})
 await dialog.locator('input[autocomplete=off]').fill('play.example.com')
 await dialog.getByRole('checkbox').check()
 const preset = page.viewportSize()!
 for (const width of info.project.name.includes('reflow') ? [320, 620, preset.width] : [preset.width]) {
  if (info.project.name.includes('reflow')) await page.setViewportSize({width, height: preset.height})
  await expectNoPageOverflow(page)
  await expectInsideViewport(save)
 }
 await page.screenshot({path: info.outputPath('domain-direct-save.png')})
 await save.click()
 const retry = dialog.getByRole('button', {name: zh ? '重试' : 'Retry', exact: true})
 await expect(retry).toBeEnabled()
 await expect(dialog.locator('fieldset input').first()).toBeDisabled()
 expect(writes).toHaveLength(1)
 await retry.click()
 await expect(dialog).toHaveCount(0)
 expect(writes).toHaveLength(2)
 expect(writes[0].key).toBeTruthy()
 expect(writes[1]).toEqual(writes[0])
 await page.locator('.member-menu-trigger').click()
 await page.getByRole('menuitem', {name: zh ? '编辑' : 'Edit', exact: true}).click()
 await dialog.getByRole('checkbox').uncheck()
 await save.click()
 await expect(dialog).toHaveCount(0)
 expect(writes).toHaveLength(3)
 expect(writes[2].method).toBe('PATCH')
 expect(writes[2].body).toEqual({version: 2, enabled: false, reason: ''})
 await page.locator('.member-menu-trigger').click()
 await page.getByRole('menuitem', {name: zh ? '删除' : 'Delete', exact: true}).click()
 await expect(dialog.getByRole('button', {name: zh ? '确认删除' : 'Confirm deletion', exact: true})).toBeVisible()
 expect(writes).toHaveLength(3)
})
