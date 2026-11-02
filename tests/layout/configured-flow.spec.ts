import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {gameId,origins,expectNoPageOverflow} from './fixture'
import {selectControl} from '../select-control'
const mark=JSON.parse(readFileSync('backend/internal/rules/config/mark_six_v2.json','utf8'))
const canada=JSON.parse(readFileSync('backend/internal/rules/config/canada_28.json','utf8'))
for(const locale of ['en','zh-CN'] as const)test(`configured first rules preserve the chosen model ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN';let saved:Record<string,unknown>|null=null;const writes:unknown[]=[]
 await page.route('**/api/v1/**',async route=>{
  const r=route.request(),url=new URL(r.url()),path=url.pathname
  const ok=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/rule-preview/template'))return ok(url.searchParams.get('template')==='mark_six'?mark:canada)
  expect(path).toBe(`/api/v1/platform/platform-games/${gameId}/configured/rules`)
  if(r.method()==='GET')return ok(saved)
  expect(r.method()).toBe('POST');const body=r.postDataJSON();writes.push(body)
  expect(body.snapshot.catalog.model).toEqual(canada.model)
  saved={id:gameId,game_id:gameId,revision:1,snapshot:body.snapshot,snapshot_hash:'test',status:'published',cases:[],validation:null}
  return ok(saved)
 })
 const props={surface:'platform',brandId:'',gameId,locale,model:canada.model,canWrite:true}
 await page.route('**/configured-flow-harness',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><main id="app" class="management-console panel" style="padding:12px;margin:12px;min-width:0"></main><script id="harness-data" type="application/json">${JSON.stringify({component:'ConfiguredRuleWorkbench',props,locale})}</script><script type="module" src="/tests/layout-harness.ts"></script></body></html>`}))
 await page.goto(`${origins.brand}/configured-flow-harness`)
 const load=page.getByRole('button',{name:zh?'载入通用示例配置':'Load sample template',exact:true})
 await load.click()
 await expect(page.getByRole('alert')).toContainText(zh?'模板的号码模型与彩种不一致':'This template uses a different number model')
 await expect(page.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeDisabled()
 expect(writes).toHaveLength(0)
 await selectControl(page.getByRole('combobox',{name:zh?'规则模板':'Rule template',exact:true}),'canada_28')
 await load.click()
 await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
 await expect.poll(()=>writes.length).toBe(1)
 await expect(page.getByRole('alert')).toHaveCount(0)
 if(info.project.name.endsWith('reflow'))for(const width of [320,480,768,1280]){await page.setViewportSize({width,height:800});await expectNoPageOverflow(page)}
 await expectNoPageOverflow(page)
 await page.screenshot({path:info.outputPath(`configured-first-rules-${locale}.png`),fullPage:true})
})
