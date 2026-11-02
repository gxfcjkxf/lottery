import {expect,test} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {gameId,origins,installLayoutApi,expectNoPageOverflow} from './fixture'
import {selectControl} from '../select-control'
import {platformLotteryCatalog} from '../../shared/src/platform-lottery'
const model=JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')).model
for(const locale of ['en','zh-CN'] as const)test(`platform catalogue uses current routes ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',copy=platformLotteryCatalog[locale],seen:string[]=[],unexpected:string[]=[],errors:string[]=[]
 await installLayoutApi(page,locale);page.on('pageerror',e=>errors.push(e.message))
 await page.addInitScript(({id})=>sessionStorage.setItem('lottery.platform.selectedBrand',JSON.stringify({accountId:id,brandId:id})),{id:gameId})
 const game={id:gameId,code:'mark_six',name:'Mark Six',model,timezone:'Asia/Singapore',version:1,connected:false,status:'active',deletable:true,brand_enabled:false,brand_version:0,rule_published:false,sales_status:'unconfigured'}
 await page.route('**/api/v1/platform/**',route=>{
  const req=route.request(),url=new URL(req.url()),path=url.pathname;seen.push(path)
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  expect(req.method()).toBe('GET')
  if(path.endsWith('/presentation'))return ok({display_name:'Trial platform',logo_text:'T',logo_url:''})
  if(path.endsWith('/me'))return ok({account:{id:gameId,username:'catalog_reviewer',super_admin:true,platform_permissions:['platform_game.view.platform','platform_game.write.platform','bet.view.platform']}})
  if(path.endsWith('/platform-settings'))return ok({version:1,display_name:'Trial platform',logo_text:'T',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:'2026-11-01T00:00:00Z'})
  if(path.endsWith('/brands'))return ok({items:[{id:gameId,code:'trial',name:'Trial Brand',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:'2026-11-01T00:00:00Z'}]})
  if(path==='/api/v1/platform/platform-games')return ok({games:[game],limit:50,offset:0,total_count:'1'})
  if(path==='/api/v1/platform/platform-games/official-calendar')return ok({current_month:'2026-11',months:[],status:'pending',last_success_at:null,last_attempt_at:null,next_attempt_at:null})
  if(path===`/api/v1/platform/platform-games/${gameId}`)return ok({game,periods:[],total_count:'0'})
  if(path===`/api/v1/platform/platform-games/${gameId}/configured/rules`)return ok(null)
  if(path===`/api/v1/platform/platform-games/${gameId}/draw-schedule`)return ok({version:1,enabled:false,mode:'daily',draw_time:'21:30:00',open_before_hours:12,close_before_seconds:60,collect_after_seconds:30})
  if(path===`/api/v1/platform/platform-games/${gameId}/automation`)return ok({plan:null,source_check:null,allowed_origins:[],attempts:[]})
  if(path===`/api/v1/platform/platform-games/${gameId}/draw-sources`)return ok({items:[],active_source_id:'',active_source_revision:0,version:0})
  if(path===`/api/v1/platform/platform-games/${gameId}/draw-overview`)return ok({years:['2026'],year:'2026',has_more:false,pending_has_more:false,items:[],pending:[],total_count:'0'})
  if(path==='/api/v1/platform/filter-options'){
   expect(url.searchParams.get('resource')).toBe('bet');expect(url.searchParams.get('kind')).toBe('game');expect(req.headers()['x-brand-id']).toBe(gameId)
   return ok({brand_id:gameId,items:[{id:gameId,name:'Mark Six'}],limit:100,offset:0})
  }
  if(path==='/api/v1/platform/configured-orders')return ok({items:[],total_count:'0'})
  unexpected.push(path);return route.fulfill({status:404,json:{success:false,error:{code:'NOT_FOUND',message:'Unexpected route'}}})
 })
 await page.goto(origins.platform)
 await expect(page.locator('.management-console')).toBeVisible()
 const go=async(label:string)=>{const more=page.locator('.platform-mobile-nav button[aria-controls="platform-navigation"]');if(await more.isVisible())await more.click();await page.locator('.sidebar').getByRole('button',{name:label,exact:true}).click()}
 await go(zh?'彩种':'Games')
 await expect(page.locator('.platform-lottery-catalog-table')).toContainText('Mark Six')
 await expect(page.locator('.topbar-brand-picker')).toHaveCount(0)
 for(const label of [copy.periods,copy.plays,copy.model,copy.timeSettings,copy.drawSources]){
  await page.locator('.platform-lottery-catalog-table tbody tr').first().getByRole('button',{name:label,exact:true}).click()
  await expect(page.getByRole('button',{name:copy.backToGames,exact:true})).toBeVisible()
  await expectNoPageOverflow(page)
  await page.getByRole('button',{name:copy.backToGames,exact:true}).click()
 }
 await page.screenshot({path:info.outputPath('catalog.png'),fullPage:true})
 await go(zh?'开奖':'Draws')
 await selectControl(page.locator('.draws-game-picker').getByRole('combobox'),`platform:${gameId}`)
 await expect.poll(()=>seen.includes(`/api/v1/platform/platform-games/${gameId}/draw-overview`)).toBe(true)
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath('draws.png'),fullPage:true})
 await go(zh?'投注订单':'Bet orders')
 await expect.poll(()=>seen.includes('/api/v1/platform/filter-options')).toBe(true)
 await selectControl(page.locator('.brand-picker').getByRole('combobox'),gameId)
 await expect(page.locator('.configured-orders')).toBeVisible();await expectNoPageOverflow(page)
 await expect(page.locator('.operation-toast--error')).toHaveCount(0)
 expect(unexpected).toEqual([]);expect(errors).toEqual([])
})
