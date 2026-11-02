import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {configuredDemo} from '../../shared/src/configured-rules'
import {selectControl} from '../select-control'
const origin=`http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE??15273}`,gameId='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8')))
for(const locale of ['en','zh-CN'] as const)for(const surface of ['platform','admin'] as const){
 test(`focused game rules ${surface} ${locale}`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[],writes:unknown[]=[]
  let saved=structuredClone(snapshot),failSave=false
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const r=route.request(),path=new URL(r.url()).pathname
   const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/configured/rules')){
    if(r.method()==='POST'){
     writes.push({body:r.postDataJSON(),key:r.headers()['idempotency-key']})
     if(failSave)return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Unavailable'}}})
     saved=r.postDataJSON().snapshot
    }
    return ok({id:gameId,game_id:gameId,revision:1,snapshot:saved,snapshot_hash:'test',cases:[],status:'published',validation:null})
   }
   if(path.endsWith('/configured/sales'))return route.fulfill({status:404,json:{success:false,error:{code:'RESOURCE_NOT_FOUND',message:'Not configured'}}})
   if(path.endsWith('/rule-preview'))return ok({definition_hash:'test',snapshot_hash:'test',price_keys:['test']})
   throw new Error('Unexpected API request: '+path)
  })
  const props={surface,brandId:surface==='admin'?gameId:'',gameId,locale,canWrite:surface==='platform'}
  await page.route('**/__focused-rules',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html lang="${locale}"><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console panel" style="margin:12px;padding:12px;min-width:0"></main><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'/@fs/${resolve('shared/src/ConfiguredRuleWorkbench.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve(surface+'-web/src/style.css')}';import'/@fs/${resolve('shared/src/management.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Component,${JSON.stringify(props)}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__focused-rules')
  const editor=page.getByTestId('configured-workbench'),search=page.getByLabel(zh?'搜索玩法':'Search plays',{exact:true})
  const check=async()=>{
   expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
   if(await page.getByRole('dialog').count()){
    const box=await page.getByRole('dialog').boundingBox();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.width).toBeLessThanOrEqual(page.viewportSize()!.width)
    await expect(page.getByRole('button',{name:zh?'关闭':'Close',exact:true})).toBeInViewport()
   }
  }
  await expect(editor.locator('tbody tr')).toHaveCount(snapshot.catalog.plays.length)
  await check();await page.screenshot({path:info.outputPath('rules-table.png'),fullPage:true})
  await search.fill(zh?'特码':'Special number')
  await expect(editor.locator('tbody tr')).not.toHaveCount(snapshot.catalog.plays.length)
  await search.fill('unmatched-rule');await expect(editor.getByText(zh?'暂无匹配玩法':'No matching plays')).toBeVisible()
  await search.fill('')
  await selectControl(editor.getByRole('combobox',{name:zh?'分组':'Group',exact:true}),'numbers')
  await expect(editor.locator('tbody tr')).toHaveCount(snapshot.catalog.plays.filter(p=>p.group==='numbers').length)
  await editor.locator('tbody tr').first().getByRole('button').first().click()
  const dialog=page.getByRole('dialog')
  await expect(dialog).toBeVisible();await check()
  await expect(dialog.getByLabel(zh?'最高倍数':'Maximum multiplier',{exact:true})).toBeVisible()
  await page.screenshot({path:info.outputPath('rule-basic.png'),fullPage:false})
  if(surface==='platform'){
   await dialog.getByLabel(zh?'分组':'Group',{exact:true}).fill('changed_group')
   await dialog.getByRole('button',{name:zh?'应用配置':'Apply configuration',exact:true}).click()
   await expect(dialog).toHaveCount(0)
   await expect(editor.getByText(zh?'有未保存的修改':'Unsaved changes',{exact:true})).toBeVisible()
   expect(writes).toHaveLength(0)
   await selectControl(editor.getByRole('combobox',{name:zh?'分组':'Group',exact:true}),'')
   await editor.locator('tbody tr').first().getByRole('button').first().click()
   await dialog.getByLabel(zh?'分组':'Group',{exact:true}).fill('discard_me')
   page.once('dialog',d=>d.accept())
   await dialog.locator('.detail-modal-close').click()
   await editor.locator('tbody tr').first().getByRole('button').first().click()
   await expect(dialog.getByLabel(zh?'分组':'Group',{exact:true})).toHaveValue('changed_group')
  }else await expect(dialog.getByLabel(zh?'分组':'Group',{exact:true})).toBeDisabled()
  await dialog.getByRole('button',{name:zh?'中奖与结算':'Winning & settlement',exact:true}).click()
  await expect(dialog.getByRole('combobox',{name:zh?'舍入规则':'Rounding',exact:true})).toBeVisible();await check()
  await page.screenshot({path:info.outputPath('rule-settlement.png'),fullPage:false})
  await dialog.getByRole('button',{name:zh?'规则试算':'Test rules',exact:true}).click()
  await dialog.getByRole('button',{name:zh?'验证配置':'Validate configuration',exact:true}).click()
  await expect(page.getByTestId('configured-selection')).toBeVisible();await check()
  await dialog.getByRole('button',{name:zh?'基础与选号':'Basics & selection',exact:true}).click()
  await dialog.getByRole('button',{name:zh?'选号选项域':'Option domain',exact:true}).click()
  await expect(dialog.getByRole('status')).toContainText(zh?'此选号域影响':'Shared options affect')
  await dialog.locator('.detail-modal-close').click()
  if(surface==='platform'){
   failSave=true
   await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
   await expect(page.getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true})).toBeVisible()
   failSave=false
   await page.getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true}).click()
   await expect.poll(()=>writes.length).toBe(2)
   expect(writes[0]).toEqual(writes[1])
   await expect(editor.getByText(zh?'有未保存的修改':'Unsaved changes',{exact:true})).toHaveCount(0)
   expect(saved.catalog.plays[0].group).toBe('changed_group')
  }
  await check();expect(errors).toEqual([])
 })
}
