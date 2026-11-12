import {test,expect,type Locator} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,gameId,theme,expectNoPageOverflow} from './fixture'
import {selectControl} from '../select-control'
const actor='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',at='2026-10-21T00:00:00Z'
for(const locale of ['en','zh-CN'] as const)for(const view of ['presentation','withdrawal','archive','preferences','automation'] as const){
 test(`ordinary page actions align ${view} (${locale})`,async({page},info)=>{
  page.setDefaultTimeout(8000)
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const path=new URL(route.request().url()).pathname
   expect(route.request().method()).toBe('GET')
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   const plan={source_type:'hk_marksix_6hch',version:1,enabled:true,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,close_before_seconds:30,source_url:'https://historical.example/feed',credential_ref:''}
   if(path.endsWith('/draw-sources'))return reply({items:[{id:actor,name:'Historical source',revision:1,plan}],active_source_id:actor,active_source_revision:1,version:1})
   if(path.endsWith('/automation'))return reply({plan,brand:null,allowed_origins:['https://historical.example'],attempts:[]})
   if(path.endsWith('/brand-presentation/defaults'))return reply(theme)
   if(path.endsWith('/brand-presentation'))return reply({brand_id:gameId,version:1,status:'active',base_name:theme.display_name,config:Object.fromEntries(Object.keys(theme).map(key=>[key,null])),effective:theme,updated_at:at,audit_log_id:gameId})
   if(path.endsWith('/filter-options'))return reply({brand_id:gameId,items:[{id:actor,name:'Trial game'}],limit:100,offset:0})
   if(path.endsWith('/games/'+actor+'/withdrawal-policy'))return reply({brand_id:gameId,game_id:actor,version:1,config:{turnover_multiple:null},effective:{turnover_multiple:'1',source:'brand',brand_version:1,game_version:1},updated_at:at})
   if(path.endsWith('/withdrawal-policy'))return reply({brand_id:gameId,version:1,points_step:'1',config:{enabled:false,min_points:'1',max_points:null,allowed_sources:['recharge','winning','gift'],review_mode:'manual',turnover_multiple:'1'},updated_at:at,audit_log_id:gameId})
   if(path.endsWith('/data-retention-policy'))return reply({version:1,enabled:false,execution_day:10,execution_time:'02:00',timezone:'Asia/Singapore',next_run:null,updated_at:at})
   if(path.endsWith('/data-retention-tasks'))return reply({items:[],total_count:'0',limit:50,offset:0})
   if(path.endsWith('/betting-preferences'))return reply({brand_id:gameId,member_id:actor,quick_amounts:['10','100']})
   throw Error('Unexpected page action read '+path)
  })
  const origin=view==='preferences'?origins.user:(view==='archive'||view==='automation')?origins.platform:origins.brand
  const app=view==='preferences'?'user-web':(view==='archive'||view==='automation')?'platform-web':'admin-web'
  await page.route('**/__page-actions',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Harness from'/@fs${resolve('tests/layout/PageActionsHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(app+'/src/style.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';createApp(Harness,${JSON.stringify({view,locale})}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__page-actions')
  if(view==='archive')await page.getByRole('tab',{name:locale==='en'?'Retention policy':'保留策略'}).click()
  if(view==='automation')await page.getByRole('button',{name:locale==='en'?'Add data source':'新增数据源',exact:true}).click()
  const actions=page.locator(view==='presentation'?'.reset-settings':view==='withdrawal'?'.action-row':view==='archive'?'.retention-form > .ui-form-actions':view==='automation'?'.automation-settings .ui-form-actions':'.betting-preferences .ui-form-actions')
  await expect(actions.first()).toBeVisible()
  if(view==='withdrawal'){
   await selectControl(page.locator('#withdraw-game-select'),actor)
   await expect(actions).toHaveCount(2)
  }
  async function checkRight(row:Locator){
   const buttons=row.locator('button').filter({visible:true})
   await buttons.last().scrollIntoViewIfNeeded()
   await expect(buttons.last()).toBeInViewport()
   const geometry=await row.evaluate(el=>{
    const style=getComputedStyle(el),box=el.getBoundingClientRect()
    const buttons=[...el.querySelectorAll('button')].filter(b=>b.getBoundingClientRect().width>0).map(b=>b.getBoundingClientRect())
    return {right:box.right-parseFloat(style.paddingRight)-parseFloat(style.borderRightWidth),lastRight:Math.max(...buttons.map(b=>b.right)),rows:[...new Set(buttons.map(b=>Math.round(b.top)))].map(top=>Math.max(...buttons.filter(b=>Math.round(b.top)===top).map(b=>b.right)))}
   })
   expect(Math.abs(geometry.right-geometry.lastRight)).toBeLessThanOrEqual(2)
   for(const right of geometry.rows)expect(Math.abs(geometry.right-right)).toBeLessThanOrEqual(2)
  }
  const preset=page.viewportSize()!
  for(const width of info.project.name.endsWith('reflow')?[320,640,900,1440]:[preset.width]){
   if(info.project.name.endsWith('reflow'))await page.setViewportSize({width,height:preset.height})
   for(const row of await actions.all())await checkRight(row)
   if(view==='presentation'){
    await actions.locator('button').first().click()
    await checkRight(actions)
    await actions.getByRole('button',{name:locale==='en'?'Cancel confirmation':'取消确认',exact:true}).click()
   }
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`page-actions-${view}-${locale}-${width}.png`)})
  }
  expect(errors).toEqual([])
 })
}
