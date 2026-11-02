import { selectControl } from '../select-control'
import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { gameId, installLayoutApi, installOrdersLayoutApi, installWalletLayoutApi, origins } from './fixture'

test('account matching stays quiet while paging and retry track explicit actions', async ({ page }, info) => {
  const source = await (await page.request.get(origins.brand + '/src/ReportsManagement.vue')).text()
  const vuePath = source.match(/from "([^"]*deps\/vue\.js[^"]*)"/)![1]
  let release!: () => void, requests = 0, fail = false
  let gate: Promise<void>
  const block = () => { gate = new Promise<void>(resolve => { release = resolve }) }
  await page.route('**/api/**/filter-options?*', async route => {
    requests++; await gate
    if (fail) return route.fulfill({ status: 503, json: { success: false, error: { code: 'SERVICE_UNAVAILABLE', message: 'Synthetic failure' } } })
    const offset = Number(new URL(route.request().url()).searchParams.get('offset'))
    return route.fulfill({ json: { success: true, data: { brand_id: '11111111-1111-4111-8111-111111111111', limit: 100, offset,
      items: Array.from({ length: offset ? 1 : 100 }, (_, index) => ({ id: `33333333-3333-4333-8333-${String(offset + index + 1).padStart(12, '0')}`, name: `agent_${offset + index}` })),
     total_count: String(offset ? 1 : 100)} } })
  })
  await page.route(origins.brand + '/__loading-search', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'${vuePath}';import Harness from'/@fs/${resolve('tests/layout/LoadingSearchHarness.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Harness).mount('#app')</script></html>` }))
  await page.goto(origins.brand + '/__loading-search')
  const input = page.getByRole('searchbox', { name: 'Agent account' }), overlay = page.locator('.global-loading-overlay')
  await expect(page.getByRole('button', {name:'Search',exact:true})).toHaveCount(0)
  block()
  try {
    await input.fill('agent'); await expect.poll(() => requests).toBe(1)
    await expect(overlay).toBeHidden()
  } finally { release() }
  await expect(page.getByRole('button', { name: 'agent_0', exact: true })).toBeVisible()
  block()
  try {
    await page.getByRole('button',{name:'More results',exact:true}).click()
    await expect.poll(()=>requests).toBe(2); await expect(overlay).toBeVisible()
  } finally { release() }
  await expect(overlay).toBeHidden()
  block(); fail=true
  try {
    await input.fill('missing');await expect.poll(()=>requests).toBe(3)
    await expect(overlay).toBeHidden()
  } finally { release() }
  await expect(page.getByRole('alert')).toBeVisible()
  block();fail=false
  try {
    await page.getByRole('button',{name:'Retry',exact:true}).click()
    await expect.poll(()=>requests).toBe(4);await expect(overlay).toBeVisible()
  } finally {release()}
  await expect(overlay).toBeHidden()
  await page.getByRole('button',{name:'agent_0',exact:true}).click()
  await input.press('Enter');expect(requests).toBe(4)
  await page.screenshot({path:info.outputPath('account-autocomplete.png')})
})

test('all three surfaces keep background requests quiet while an explicit submission remains visible', async ({ page }) => {
  await installLayoutApi(page, 'en')
  for (const url of [`${origins.user}/login`, origins.brand, origins.platform]) {
    let backgroundStarted = false, submitted = false
    let releaseBackground!: () => void, releaseSubmit!: () => void
    const background = new Promise<void>(resolve => { releaseBackground = resolve })
    const submission = new Promise<void>(resolve => { releaseSubmit = resolve })
    await page.route('**/api/v1/loading-probe', async route => {
      backgroundStarted = true
      await background
      await route.fulfill({ json: { success: true, data: {} } })
    })
    await page.route(/\/api\/v1\/(?:admin\/|platform\/)?auth\/login$/, async route => {
      expect(route.request().method()).toBe('POST')
      submitted = true
      await submission
      await route.fulfill({ status: 401, json: { success: false, error: { code: 'INVALID_CREDENTIALS', message: 'Synthetic rejected login' } } })
    })
    try {
      await page.goto(url)
      const form = page.locator('form').first()
      await expect(form).toBeVisible()
      await expect(page.locator('.global-loading-overlay')).toBeHidden()
      await page.evaluate(() => { void fetch('/api/v1/loading-probe').then(response => response.json()) })
      await expect.poll(() => backgroundStarted).toBe(true)
      await expect(page.locator('.global-loading-overlay')).toBeHidden()
      await form.locator('input[autocomplete="username"]').fill('layout-user')
      await form.locator('input[type="password"]').fill('layout-only-password')
      await form.locator('button[type="submit"], button:not([type])').last().click()
      await expect.poll(() => submitted).toBe(true)
      await expect(page.locator('.global-loading-overlay')).toBeVisible()
      releaseSubmit()
      // A still-pending background request must not keep a completed foreground action open.
      await expect(page.locator('.global-loading-overlay')).toBeHidden()
    } finally { releaseSubmit(); releaseBackground() }
  }
})

test('wallet snapshots update balances without background reads while manual refresh shows progress', async ({ page }) => {
  await page.clock.install()
  await installLayoutApi(page, 'en');await installWalletLayoutApi(page)
  await page.goto(`${origins.user}/wallet`)
  await expect(page.locator('.total-primary strong')).toHaveText('170')
  const snapshot=await page.evaluate(async()=>{const body=await(await fetch('/api/v1/wallet')).json();return body.data})
  let requests=0,release!:()=>void
  const gate=new Promise<void>(resolve=>{release=resolve})
  await page.route('**/api/v1/wallet',async route=>{requests++;await gate;await route.fallback()})
  try{
    await page.clock.fastForward(35000);expect(requests).toBe(0)
    const updated={...snapshot,version:snapshot.version+1,display_points:'180',available_points:'130',by_source:{...snapshot.by_source,recharge:{...snapshot.by_source.recharge,available:'130'}}}
    await page.evaluate(value=>window.dispatchEvent(new CustomEvent('lottery:wallet-snapshot',{detail:value})),updated)
    await expect(page.locator('.total-primary strong')).toHaveText('180');expect(requests).toBe(0)
    for(const stale of [{...updated,version:snapshot.version},{...updated,member_id:'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'},{...updated,brand_id:'cccccccc-cccc-4ccc-8ccc-cccccccccccc'}]){
      await page.evaluate(value=>window.dispatchEvent(new CustomEvent('lottery:wallet-snapshot',{detail:{...value,display_points:'999'}})),stale)
      await expect(page.locator('.total-primary strong')).toHaveText('180')
    }
    await page.getByRole('button',{name:'Menu',exact:true}).click()
    await page.locator('#user-page-refresh button').click();await expect.poll(()=>requests).toBe(1)
    await expect(page.locator('.global-loading-overlay')).toBeVisible()
    release();await expect(page.locator('.global-loading-overlay')).toBeHidden()
    await expect(page.locator('.total-primary strong')).toHaveText('180')
  }finally{release()}
})

test('quick stake edits remain local until save and failed saves preserve the draft', async ({ page }) => {
  await page.clock.install()
  await installLayoutApi(page, 'en')
  await installOrdersLayoutApi(page)
  let values = ['10'], writes = 0, rejectWrite = false, release!: () => void
  let gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/me/betting-preferences', async route => {
    if (route.request().method() === 'PUT') {
      writes++
      const requested = route.request().postDataJSON().quick_amounts
      await gate
      if (rejectWrite) return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic save failure'}}})
      values = requested
    }
    await route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,quick_amounts:values}}})
  })
  try {
    await page.goto(`${origins.user}/account`)
    const settings = page.getByTestId('betting-preferences'), overlay = page.locator('.global-loading-overlay')
    const input = settings.locator('input').first()
    await expect(input).toHaveValue('10')
    await input.fill('20')
    await input.blur()
    await page.clock.fastForward(1000)
    expect(writes).toBe(0)
    const save = settings.getByRole('button', {name:'Save',exact:true})
    await save.click()
    await expect.poll(() => writes).toBe(1)
    await expect(overlay).toBeVisible()
    await expect(input).toBeDisabled()
    release()
    await expect(overlay).toBeHidden()
    await expect.poll(() => values).toEqual(['20'])
    await settings.locator('.chip-delete').click()
    expect(writes).toBe(1)
    gate = new Promise<void>(resolve => { release = resolve })
    rejectWrite = true
    await save.click()
    await expect.poll(() => writes).toBe(2)
    await expect(overlay).toBeVisible()
    release()
    await expect(settings.getByRole('alert')).toBeVisible()
    await expect(overlay).toBeHidden()
    await expect(input).toHaveValue('')
    expect(values).toEqual(['20'])
    gate = new Promise<void>(resolve => { release = resolve })
    rejectWrite = false
    await settings.getByRole('alert').getByRole('button').click()
    await expect.poll(() => writes).toBe(3)
    await expect(overlay).toBeVisible()
    release()
    await expect(overlay).toBeHidden()
    await expect.poll(() => values).toEqual([])
    await expect(settings.getByRole('alert')).toHaveCount(0)
  } finally { release() }
})

test('configured wallet retry waits for the response and preserves unavailable-wallet protection', async ({ page }) => {
  await installLayoutApi(page, 'en')
  await installOrdersLayoutApi(page)
  let requests = 0, delayResponse = false, release!: () => void
  const gate = new Promise<void>(resolve => { release = resolve })
  await page.route('**/api/v1/wallet', async route => {
    requests++
    if (delayResponse) await gate
    await route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic wallet failure'}}})
  })
  try {
    await page.goto(`${origins.user}/configured/${gameId}`)
    const warning = page.locator('.configured-funds-warning'), overlay = page.locator('.global-loading-overlay')
    await expect(warning).toBeVisible()
    await expect(overlay).toBeHidden()
    const initialRequests = requests
    delayResponse = true
    await warning.getByRole('button').click()
    await expect.poll(() => requests).toBe(initialRequests + 1)
    await expect(overlay).toBeVisible()
    release()
    await expect(overlay).toBeHidden()
    await expect(warning).toBeVisible()
    await expect(page.locator('.configured-available strong')).toHaveText('—')
    await expect(page.getByRole('button', {name:'Confirm bet',exact:true})).toHaveCount(0)
  } finally { release() }
})


test('draw overview polling stays quiet while refresh, year and pagination reads show progress', async ({ page }, info) => {
  await page.clock.install()
  let reads = 0
  let blocked = false
  let release!: () => void
  let gate: Promise<void>
  const block = () => { blocked = true; gate = new Promise<void>(resolve => { release = resolve }) }
  await page.route('**/api/**', async route => {
    const request = route.request(), url = new URL(request.url())
    expect(request.method()).toBe('GET')
    if (url.pathname.endsWith('/automation')) return route.fulfill({ json: { success: true, data: { plan: null, brand: null, allowed_origins: [], attempts: [], source_check: null } } })
    expect(url.pathname).toBe(`/api/v1/platform/platform-games/${gameId}/draw-overview`)
    reads++
    if (blocked) await gate
    return route.fulfill({ json: { success: true, data: { years: ['2026', '2025'], year: url.searchParams.get('year') || '2026', items: [], has_more: url.searchParams.get('offset') === '0', pending: [], pending_has_more: false , total_count: '0'} } })
  })
  await page.route('**/__draw-overview-progress', route => route.fulfill({ contentType: 'text/html', body: `<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import{createApp,h}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('shared/src/PlatformDrawOverview.vue')}';import Loading from'/@fs${resolve('shared/src/LoadingOverlay.vue')}';import{setPlatformDisplaySettings}from'/@fs${resolve('shared/src/display-settings.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/refresh-button.css')}';setPlatformDisplaySettings({timezone:'UTC',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'});createApp({render:()=>h('main',[h(C,{gameId:'${gameId}',locale:'en',canManage:false}),h(Loading,{label:'Loading…'})])}).mount('#app')</script></body></html>` }))
  await page.goto(origins.user + '/__draw-overview-progress')
  const overlay = page.locator('.global-loading-overlay'), overview = page.getByTestId('draw-overview')
  await expect(overview.locator(".select-control-trigger")).toHaveAttribute('data-value', '2026')
  await expect(overlay).toBeHidden()
  const refresh = overview.locator('button[data-refresh-button]')
  await expect(refresh).toHaveAccessibleName('Refresh')
  await expect(refresh.locator('[data-refresh-icon]')).toHaveCount(1)
  block()
  try {
    const before = reads
    await page.clock.fastForward(5_000)
    await expect.poll(() => reads).toBeGreaterThan(before)
    await expect(overlay).toBeHidden()
  } finally { blocked = false; release() }
  await expect(overview).toHaveAttribute('aria-busy', 'false')
  for (const action of [() => refresh.click(), () => selectControl(overview.locator(".select-control-trigger"), '2025'), () => overview.getByRole('button', { name: 'Next page', exact: true }).click()]) {
    block()
    try {
      await action()
      await expect(overlay).toBeVisible()
      await expect(overview.getByText('Draw history', { exact: true })).toBeVisible()
    } finally { blocked = false; release() }
    await expect(overlay).toBeHidden()
  }
  const preset = page.viewportSize()!
  if (info.project.name.endsWith('reflow')) await page.setViewportSize({ width: 320, height: 720 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true)
  await page.screenshot({ path: info.outputPath('draw-overview-refresh.png'), fullPage: true })
  if (info.project.name.endsWith('reflow')) await page.setViewportSize(preset)
})
