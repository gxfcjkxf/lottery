import {expect,test} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
const origin=`http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE??15273}`
const gameID='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
for(const locale of ['en','zh-CN'] as const)for(const surface of ['platform','admin'] as const){
 test(`game status, action menu and saved rules ${surface} ${locale}`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[],writes:unknown[]=[]
  page.on('pageerror',e=>{errors.push(e.message);console.error(e.message)})
  const snapshot=JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8'))
  const game={id:gameID,code:'ready_game',name:'Ready game',model:snapshot.catalog.model,timezone:'UTC',version:1,connected:true,status:'active',deletable:false,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'ready'}
  const rows=[game,{...game,id:'aaaaaaaa-aaaa-4aaa-8aaa-000000000002',name:'Unpublished game',rule_published:false,sales_status:'unconfigured'},{...game,id:'aaaaaaaa-aaaa-4aaa-8aaa-000000000003',name:'Unsold game',sales_status:'unconfigured'}]
  const version={id:'bbbbbbbb-bbbb-4bbb-8bbb-000000000001',game_id:gameID,revision:5,status:'published',snapshot,snapshot_hash:'frozen',cases:[],validation:null}
  // All API requests are intercepted. This harness never connects to a development database.
  await page.route('**/api/**',route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/display-settings'))return ok({theme:'jade',timezone:'UTC',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
   if(req.method()==='POST'){
    expect(surface).toBe('platform');expect(path).toBe(`/api/v1/platform/platform-games/${gameID}/name`)
    const body=req.postDataJSON();expect(body).toEqual({name:'Renamed game',expected_version:1});expect(req.headers()['idempotency-key']).toBeTruthy();writes.push(body);game.name=body.name;game.version++;return ok(game)
   }
   if(path.endsWith('/configured/rules'))return ok(version)
   if(path.endsWith('/configured/sales'))return ok({version:1,rule_id:version.id,enabled:true,prices:snapshot.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'})
   if(path.endsWith('/configured/orders'))return ok({items:[]})
   if(path.endsWith('/'+gameID))return ok({game,rule_protocol:'configured',plays:[],periods:[],total_count:'0',mark_six_configured:false})
   expect(path).toBe(`/api/v1/${surface}/platform-games`);return ok({games:rows,limit:50,offset:0,total_count:String(rows.length)})
  })
  await page.route('**/__game-readiness',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs/${resolve('shared/src/PlatformLottery.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve(surface+'-web/src/style.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(C,${JSON.stringify({surface,brandId:surface==='admin'?'0199a000-0000-7000-8000-000000000001':'',locale,canManage:surface==='platform',canConnect:true,canSell:true,displaySettings:{timezone:'UTC',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}})}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__game-readiness')
  const table=page.locator(surface==='admin'?'.brand-lottery-table':'.platform-lottery-catalog-table')
  if(surface==='platform')await expect(table.locator('.status-chip')).toHaveText(Array(3).fill(zh?'已上架':'Listed'))
  else await expect(table.locator('tbody tr td:nth-child(3)')).toHaveText(Array(3).fill(zh?'启用':'Enabled'))
  await expect(table.locator('.platform-lottery-readiness')).toHaveCount(0)
  await expect(table.getByText(zh?'未配置投注':'Betting not configured',{exact:true})).toHaveCount(surface==='platform'?0:2)
  if(surface==='platform'){
   await expect(page.getByRole('menuitem')).toHaveCount(0)
   await table.getByRole('button',{name:zh?'操作':'Actions',exact:true}).first().click()
   const menu=page.getByRole('menu')
   await expect(menu.getByRole('menuitem')).toHaveText(zh?['编辑','下架','删除']:['Edit','Take offline','Delete'])
   await expect(menu.getByRole('menuitem',{name:zh?'删除':'Delete',exact:true})).toBeDisabled()
   const box=await menu.boundingBox(),viewport=page.viewportSize()!
   expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(viewport.width)
   expect(box!.y).toBeGreaterThanOrEqual(0);expect(box!.y+box!.height).toBeLessThanOrEqual(viewport.height)
   await page.screenshot({path:info.outputPath(`menu-${locale}.png`),fullPage:true})
   await page.keyboard.press('Escape');await expect(menu).toHaveCount(0)
   await table.getByRole('button',{name:zh?'操作':'Actions',exact:true}).first().click()
   await page.getByRole('menuitem',{name:zh?'编辑':'Edit',exact:true}).click()
   await page.getByLabel(zh?'名称':'Name',{exact:true}).fill('Renamed game')
   await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
   await expect(table.getByText('Renamed game',{exact:true})).toBeVisible();expect(writes).toHaveLength(1)
  }else {
   await expect(table.locator('tbody tr')).toHaveCount(3)
   await expect(table.locator('.member-menu-trigger')).toHaveCount(3);await table.locator('.member-menu-trigger').first().click();await expect(page.getByRole('menuitem')).toHaveText(zh?['投注配置','停用']:['Betting configuration','Disable']);await page.keyboard.press('Escape')
   await expect(page.locator('.lottery-detail-page')).toHaveCount(0)
   await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
   await page.screenshot({path:info.outputPath(`${surface}-${locale}.png`),fullPage:true})
   expect(errors).toEqual([])
   return
  }
  await table.getByRole('button',{name:zh?'玩法规则':'Play rules',exact:true}).first().click()
  const workbench=page.getByTestId('configured-workbench')
  await expect(workbench).toBeVisible()
  await expect(workbench.getByRole('table')).toBeVisible()
  await expect(workbench.getByLabel(zh?'搜索玩法':'Search plays',{exact:true})).toBeVisible()
  await expect(page.getByRole('combobox',{name:zh?'已保存版本':'Saved version',exact:true})).toHaveCount(0)
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`${surface}-${locale}.png`),fullPage:true});expect(errors).toEqual([])
 })
}
