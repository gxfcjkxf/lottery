import {expect,test} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
const origin='http://127.0.0.1:15273'
const gameID='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
for(const locale of ['en','zh-CN'] as const)for(const surface of ['platform','admin'] as const){
 test(`game readiness, rename and saved rules ${surface} ${locale}`,async({page},info)=>{
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
   if(path.endsWith('/display-settings'))return ok({timezone:'UTC',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'})
   if(req.method()==='POST'){
    expect(surface).toBe('platform');expect(path).toBe(`/api/v1/platform/platform-games/${gameID}/name`)
    const body=req.postDataJSON();expect(body).toEqual({name:'Renamed game',expected_version:1});expect(req.headers()['idempotency-key']).toBeTruthy();writes.push(body);game.name=body.name;game.version++;return ok(game)
   }
   if(path.endsWith('/configured/versions'))return ok({items:[version]})
   if(path.endsWith('/configured/sales'))return ok({version:1,rule_id:version.id,enabled:true,prices:snapshot.prices,min_points:'1',max_points:'100',max_period_points:'10000',max_user_period_points:'5000'})
   if(path.endsWith('/configured/orders'))return ok({items:[]})
   if(path.endsWith('/'+gameID))return ok({game,rule_protocol:'configured',plays:[],periods:[],mark_six_configured:false})
   expect(path).toBe(`/api/v1/${surface}/platform-games`);return ok({games:rows,limit:50,offset:0})
  })
  await page.route('**/__game-readiness',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs/${resolve('shared/src/PlatformLottery.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve(surface+'-web/src/style.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(C,${JSON.stringify({surface,brandId:surface==='admin'?'0199a000-0000-7000-8000-000000000001':'',locale,canManage:surface==='platform',canConnect:true,canSell:true,displaySettings:{timezone:'UTC',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}})}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__game-readiness')
  const table=page.locator('.platform-lottery-catalog-table')
  await expect(table.getByText(zh?'未发布规则':'Rules not published',{exact:true})).toBeVisible()
  await expect(table.getByText(zh?'未配置销售':'Sales not configured',{exact:true})).toHaveCount(2)
  if(surface==='platform'){
   await table.getByRole('button',{name:zh?'修改彩种名称':'Rename game',exact:true}).first().click()
   await page.getByLabel(zh?'名称':'Name',{exact:true}).fill('Renamed game')
   await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
   await expect(table.getByText('Renamed game',{exact:true})).toBeVisible();expect(writes).toHaveLength(1)
  }else await expect(table.getByRole('button',{name:zh?'修改彩种名称':'Rename game',exact:true})).toHaveCount(0)
  await table.getByRole('button',{name:zh?'玩法规则':'Play rules',exact:true}).first().click()
  const workbench=page.getByTestId('configured-workbench')
  await expect(workbench).toHaveAttribute('open','')
  await expect(workbench.getByRole('combobox',{name:zh?'玩法':'Play',exact:true})).toBeVisible()
  await expect(page.getByRole('combobox',{name:zh?'已保存版本':'Saved version',exact:true})).toHaveValue(version.id)
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`${surface}-${locale}.png`),fullPage:true});expect(errors).toEqual([])
 })
}
