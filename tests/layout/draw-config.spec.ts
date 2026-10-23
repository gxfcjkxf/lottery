import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {readFileSync} from 'node:fs'
const origin='http://127.0.0.1:15273',id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
for(const locale of ['en','zh-CN'] as const)for(const surface of ['platform','admin'] as const){
 test(`unified rules and draw configuration ${surface} ${locale}`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[],writes:any[]=[];page.on('pageerror',e=>errors.push(e.message))
  const model=JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8')).catalog.model
  const game={id,name:zh?'试用六合彩':'Trial Mark Six',code:'trial',timezone:'UTC',model,version:1,status:'active',deletable:false,connected:true,brand_enabled:true,brand_version:1}
  const period={id,game_id:id,period_no:'20261020001',bet_start_at:'2026-10-20T00:00:00Z',bet_end_at:'2026-10-20T00:01:00Z',draw_at:'2026-10-20T00:01:00Z',result:null as any,published_at:null}
  const registry={items:[] as any[],active_source_id:'',active_source_revision:0,version:0};let plan:any=null,first=true;const receipts=new Map<string,any>()
  await page.route('**/api/**',route=>{
   const r=route.request(),path=new URL(r.url()).pathname,ok=(data:any,status=200)=>route.fulfill({status,json:{success:true,data}})
   if(r.method()==='GET'){
    if(path.endsWith('/configured/sales'))return route.fulfill({status:404,json:{success:false,error:{code:'RESOURCE_NOT_FOUND'}}})
    if(path.endsWith('/configured/versions')||path.endsWith('/configured/orders'))return ok({items:[]})
    if(path.endsWith('/draw-sources'))return ok(registry)
    if(path.endsWith('/automation'))return ok({plan,brand:null,allowed_origins:['https://1680660.com','https://second.example'],attempts:[],source_check:null})
    if(path.endsWith('/platform-games'))return ok({games:[game],limit:50,offset:0})
    if(path.endsWith('/'+id))return ok({game,rule_protocol:'configured',mark_six_configured:false,plays:[],periods:[period]})
    throw new Error('Unexpected GET '+path)
   }
   const body=r.postDataJSON(),key=r.headers()['idempotency-key'];writes.push({path,body,key});expect(key).toBeTruthy()
   if(path.endsWith('/draw-source-preview')){expect(body.plan.mapping.format).toBeTruthy();return ok({completed:{period_no:'sample-1',drawn_at:'2026-10-20T00:01:00Z',result:{regular:[1,2,3,4,5,6],special:[7],digits:[]}},next_period_no:'sample-2',next_draw_at:'2026-10-21T00:00:00Z'})}
   if(path.endsWith('/draw-sources')){const item={...body,id:crypto.randomUUID(),revision:1};registry.items.push(item);return ok(item)}
   if(path.endsWith('/activate')){
    if(receipts.has(key!))return ok(receipts.get(key!))
    const item=registry.items.find(x=>path.includes(x.id));expect(body).toEqual({revision:item.revision,version:registry.version});registry.version++;registry.active_source_id=item.id;registry.active_source_revision=item.revision;plan={...item.plan,version:registry.version};receipts.set(key!,plan)
    if(first){first=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE'}}})}
    return ok(plan)
   }
   if(path.endsWith('/result')){expect(body.result).toEqual({regular:[1,2,3,4,5,6],special:[7],digits:[]});period.result=body.result;return ok({...period,published_at:'2026-10-20T00:02:00Z'},201)}
   throw new Error('Unexpected POST '+path)
  })
  const props={surface,locale,brandId:surface==='admin'?id:'',canManage:surface==='platform',canConnect:surface==='admin',canSell:surface==='admin',displaySettings:{timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}}
  await page.route('**/__draw-config',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'/@fs/${resolve('shared/src/PlatformLottery.vue')}';import{setPlatformDisplaySettings}from'/@fs/${resolve('shared/src/index.ts')}';setPlatformDisplaySettings(${JSON.stringify(props.displaySettings)});import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve(surface+'-web/src/style.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Component,${JSON.stringify(props)}).mount('#app')</script></html>`}))
  await page.goto(origin+'/__draw-config')
  const row=page.getByRole('row').filter({hasText:game.name});await expect(row.getByRole('button',{name:zh?'玩法规则':'Play rules',exact:true})).toHaveCount(1)
  await expect(row).not.toContainText(zh?'加拿大投注':'Canada betting');await expect(row).not.toContainText(zh?'自动开奖':'Automatic draws')
  await row.getByRole('button',{name:zh?'玩法规则':'Play rules',exact:true}).click();await expect(page.getByTestId('configured-workbench').last()).toBeVisible();await expect(page.getByTestId('mark-six-settings')).toHaveCount(0)
  await page.getByRole('button',{name:zh?'返回彩种列表':'Back to game list',exact:true}).click()
  await row.getByRole('button',{name:zh?'开奖配置':'Draw configuration',exact:true}).click()
  if(surface==='platform'){
   for(const name of ['Source A','Source B']){
    if(name==='Source B')await page.getByRole('button',{name:zh?'新增数据源':'Add data source',exact:true}).click()
    await page.getByLabel(zh?'数据源名称':'Source name',{exact:true}).fill(name)
    const dom=name==='Source B'
    await page.getByRole('combobox',{name:zh?'期次排期方式':'Period schedule',exact:true}).selectOption('source')
    await page.getByRole('combobox',{name:zh?'响应格式':'Response format',exact:true}).selectOption(dom?'html':'json')
    await page.getByLabel(zh?'数据源地址':'Source URL',{exact:true}).fill(dom?'https://second.example/results':'https://1680660.com/api/results')
    const pathLabel=zh?'字段路径 / CSS 选择器':'Path / CSS selector'
    for(const [zhName,enName,path] of [['开奖结果期号','Result period number',dom?'#issue':'/issue'],['开奖结果时间','Result draw time',dom?'time.result':'/time'],['下期期号','Next period number',dom?'#next':'/next/id'],['下期开奖时间','Next draw time',dom?'time.next':'/next/time']])await page.getByLabel((zh?zhName:enName)+' · '+pathLabel,{exact:true}).fill(path)
    for(const [zhName,enName,path,count] of [['普通号码','Regular numbers',dom?'.regular .ball':'/regular','6'],['特别号码','Special numbers',dom?'.special':'/special','1']]){
     await page.getByRole('checkbox',{name:zh?zhName:enName,exact:true}).check()
     await page.getByLabel((zh?zhName:enName)+' · '+pathLabel,{exact:true}).fill(path)
     await page.getByLabel((zh?zhName:enName)+' · '+(zh?'号码数量':'Number count'),{exact:true}).fill(count)
    }
    await page.getByLabel(zh?'响应样例（JSON / HTML）':'Response sample (JSON / HTML)',{exact:true}).fill(dom?'<html><b id="issue">sample-1</b></html>':'{"issue":"sample-1"}')
    await page.getByRole('button',{name:zh?'解析样例':'Parse sample',exact:true}).click()
    await expect(page.getByTestId('feed-preview')).toContainText('sample-1')
    if(dom){
     await page.getByRole('combobox',{name:zh?'响应格式':'Response format',exact:true}).selectOption('browser')
     await page.getByLabel(zh?'就绪元素 CSS 选择器':'Ready element CSS selector',{exact:true}).fill('[data-ready=yes]')
     await page.getByLabel(zh?'渲染超时（毫秒）':'Render timeout (milliseconds)',{exact:true}).fill('5000')
     await page.getByLabel(zh?'会话入口地址（选填）':'Session entry URL (optional)',{exact:true}).fill('https://second.example/entry')
     await page.getByLabel(zh?'同源导航最大重定向次数':'Maximum same-origin navigation redirects',{exact:true}).fill('2')
     await page.getByLabel(zh?'忽略的辅助请求（每行一个不含查询参数的完整地址）':'Blocked auxiliary requests (one URL per line, without query)',{exact:true}).fill('https://second.example/telemetry')
     await page.getByRole('button',{name:zh?'添加 JSON 查询':'Add JSON request',exact:true}).click()
     await page.getByLabel(zh?'响应名称':'Response name',{exact:true}).fill('history')
     await page.getByLabel(zh?'查询地址':'Query URL',{exact:true}).fill('https://second.example/history')
     await page.getByRole('combobox',{name:zh?'查询方法':'Query method',exact:true}).selectOption('POST')
     await page.getByLabel(zh?'表单正文（URL 编码）':'Form body (URL encoded)',{exact:true}).fill('top=1')
     await page.getByRole('combobox',{name:(zh?'开奖结果时间':'Result draw time')+' · '+(zh?'取值来源':'Value source'),exact:true}).selectOption('history')
     await page.getByLabel((zh?'开奖结果时间':'Result draw time')+' · '+pathLabel,{exact:true}).fill('/data/0/time')
     await page.getByRole('combobox',{name:zh?'来源时间格式':'Source time format',exact:true}).selectOption('custom')
     await page.getByLabel(zh?'时间格式模板（Go 参考时间）':'Time layout (Go reference time)',{exact:true}).fill('2006-01-02 15:04:05')
     await page.getByLabel(zh?'来源时区（IANA）':'Source timezone (IANA)',{exact:true}).fill('Asia/Shanghai')
     await page.getByRole('checkbox',{name:zh?'校验时间与开奖结果期号一致':'Verify the time belongs to the result period',exact:true}).check()
     await page.getByRole('combobox',{name:(zh?'时间响应中的期号':'Time response period number')+' · '+(zh?'取值来源':'Value source'),exact:true}).selectOption('history')
     await page.getByLabel((zh?'时间响应中的期号':'Time response period number')+' · '+pathLabel,{exact:true}).fill('/data/0/issue')
     await page.getByRole('checkbox',{name:zh?'读取下期公告（关闭则只采集已建期次）':'Read the next-period announcement (off: collect existing periods only)',exact:true}).uncheck()
     await page.getByLabel((zh?'普通号码':'Regular numbers')+' · '+(zh?'HTML 属性（选填）':'HTML attribute (optional)'),{exact:true}).fill('class')
     await page.getByLabel((zh?'普通号码':'Regular numbers')+' · '+(zh?'提取正则（一个捕获组，选填）':'Capture pattern (one group, optional)'),{exact:true}).fill('ball_([0-9]+)')
     await expect(page.getByRole('button',{name:zh?'解析样例':'Parse sample',exact:true})).toHaveCount(0)
     await page.getByRole('button',{name:zh?'抓取并预览':'Fetch and preview',exact:true}).click()
     await expect(page.getByTestId('feed-preview')).toContainText('sample-1')
    }
    await page.getByRole('button',{name:zh?'保存数据源':'Save data source',exact:true}).click();await expect(page.locator('.draw-source-list')).toContainText(name)
   }
   expect(registry.active_source_id).toBe('')
   expect(registry.items[1].plan.mapping.browser.json_requests).toEqual([{id:'history',url:'https://second.example/history',method:'POST',body:'top=1'}])
   expect(registry.items[1].plan.mapping.draw_time.response).toBe('history')
   expect(registry.items[1].plan.mapping.next_period).toBeNull()
   expect(registry.items[1].plan.mapping.regular.capture).toBe('ball_([0-9]+)')
   await page.locator('.draw-source-list article').filter({hasText:'Source A'}).getByRole('button',{name:zh?'使用此版本':'Use this revision',exact:true}).click()
   await page.getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true}).click()
   const calls=writes.filter(x=>x.path.endsWith('/activate'));expect(calls).toHaveLength(2);expect(calls[1]).toEqual(calls[0])
   await page.locator('.draw-source-list article').filter({hasText:'Source B'}).getByRole('button',{name:zh?'使用此版本':'Use this revision',exact:true}).click();await expect(page.locator('.automation-settings')).toContainText((zh?'当前数据源':'Current data source')+': Source B')
   await page.screenshot({path:info.outputPath(`sources-${locale}.png`),fullPage:true})
  }else await expect(page.getByRole('button',{name:zh?'新增数据源':'Add data source',exact:true})).toHaveCount(0)
  await page.getByRole('button',{name:zh?'手动开奖结果':'Manual results',exact:true}).click()
  if(surface==='platform'){
   const publish=page.getByRole('button',{name:zh?'发布开奖结果':'Publish result',exact:true}).filter({visible:true});await publish.click()
   await page.getByLabel(zh?'普通号码':'Regular numbers',{exact:true}).fill('1,2,3,4,5,6');await page.getByLabel(zh?'特别号码':'Special numbers',{exact:true}).fill('7');await page.getByRole('button',{name:zh?'保存':'Save',exact:true}).click();await expect(page.locator('.lottery-detail-page')).toContainText('1, 2, 3, 4, 5, 6, 7')
  }else await expect(page.getByRole('button',{name:zh?'发布开奖结果':'Publish result',exact:true})).toHaveCount(0)
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`manual-${locale}.png`),fullPage:true});expect(errors).toEqual([])
 })
}

for(const locale of ['en','zh-CN'] as const)test(`retired draw source is read-only and new configuration is mapped ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN',writes:string[]=[]
 const plan={source_type:'hk_marksix_6hch',version:1,enabled:true,anchor_draw_at:null,anchor_period_no:null,interval_seconds:null,close_before_seconds:30,source_url:'https://historical.example/smallSix/findSmallSixInfo.do',credential_ref:''}
 await page.route('**/api/**',route=>{
  if(route.request().method()!=='GET'){writes.push(route.request().url());return route.abort()}
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/draw-sources'))return route.fulfill({json:{success:true,data:{items:[{id,name:'Historical source',revision:1,plan}],active_source_id:id,active_source_revision:1,version:1}}})
  if(path.endsWith('/automation'))return route.fulfill({json:{success:true,data:{plan,brand:null,allowed_origins:['https://historical.example'],attempts:[],source_check:{at:'2026-10-20T00:00:00Z',code:'ready',next_period_no:'2026002',next_draw_at:'2026-10-21T00:00:00Z',last_result:{period_no:'2026001',drawn_at:'2026-10-20T00:00:00Z',result:{regular:[1,2,3,4,5,6],special:[7],digits:[]},attributes:{waves:{'07':'red'},zodiac:{'07':'horse'},elements:{'07':'water'}}}}}}})
  throw new Error('Unexpected read '+path)
 })
 const props={surface:'platform',gameId:id,brandId:'',locale,canWrite:true}
 await page.route('**/__retired-source',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><main id="app" class="management-console"></main><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'/@fs/${resolve('shared/src/AutomationSettings.vue')}';import'/@fs/${resolve('shared/src/styles.css')}';import'/@fs/${resolve('platform-web/src/style.css')}';import'/@fs/${resolve('shared/src/controls.css')}';createApp(Component,${JSON.stringify(props)}).mount('#app')</script></html>`}))
 await page.goto(origin+'/__retired-source')
 await expect(page.getByRole('status')).toContainText(zh?'已退役协议':'retired protocol')
 await expect(page.getByLabel(zh?'数据源名称':'Source name',{exact:true})).toBeDisabled()
 await expect(page.getByRole('button',{name:zh?'使用此版本':'Use this revision',exact:true})).toBeDisabled()
 await expect(page.getByText('2026001 · 01, 02, 03, 04, 05, 06, 07',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:zh?'新增数据源':'Add data source',exact:true}).click()
 await expect(page.getByLabel(zh?'数据源名称':'Source name',{exact:true})).toBeEnabled()
 await expect(page.getByRole('combobox',{name:zh?'响应格式':'Response format',exact:true})).toHaveValue('json')
 await expect(page.getByRole('status')).toHaveCount(0)
 expect(writes).toEqual([])
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
 await page.screenshot({path:info.outputPath(`retired-source-${locale}.png`),fullPage:true})
})
