import {test,expect,type Page,type Locator} from '@playwright/test'
import {readFileSync,writeFileSync} from 'node:fs'
import {createHash} from 'node:crypto'
import {selectControl} from '../select-control'
import {openBrandPage,openPlatformPage} from '../layout/real-console-session'

const fixture=JSON.parse(readFileSync(process.env.RETRY_REAL_FIXTURE_FILE!,'utf8'))
const port=Number(process.env.RETRY_REAL_PORT_BASE??16473)
const origin={user:`http://127.0.0.1:${port}`,brand:`http://127.0.0.1:${port+1}`,platform:`http://127.0.0.1:${port+2}`,agent:`http://127.0.0.1:${port+3}`}
const password='retry-test-password-2026',adminPassword='root-test-password-2026'

test('four real apps replay committed requests without duplicate identities or money',async({browser},info)=>{
 const contexts=await Promise.all(Object.keys(origin).map(()=>browser.newContext({viewport:info.project.use.viewport,userAgent:info.project.use.userAgent,deviceScaleFactor:info.project.use.deviceScaleFactor,isMobile:info.project.use.isMobile,hasTouch:info.project.use.hasTouch,screen:info.project.use.screen,locale:'zh-CN'})))
 const pages=await Promise.all(contexts.map(c=>c.newPage()))
 const [user,brand,platform,agent]=pages
 const errors:string[]=[]
 for(const page of pages){page.setDefaultTimeout(12000);page.on('pageerror',e=>errors.push(e.message));await page.context().addCookies([{name:'lottery-locale',value:'zh-CN',url:origin.user}]);await page.addInitScript(()=>{localStorage.setItem('lottery.admin.locale','zh-CN');localStorage.setItem('lottery.platform.locale','zh-CN')})}
 const evidence:Array<{name:string;path:string;key:string;body_sha256:string;responses:number}>=[]
 async function login(page:Page,url:string,account:string,pass:string){
  await page.goto(url)
  await page.locator('input[autocomplete=username]').fill(account)
  await page.locator('input[type=password]').fill(pass)
  await page.locator('form .button-primary').click()
  await expect(page.locator('.management-console, .workspace-overview-control')).toBeVisible()
 }
 // Forward every write to the real API. Discard only the first successful
 // response, after PostgreSQL has committed; subsequent responses are genuine.
 async function replay(page:Page,path:string,scope:()=>Locator,submit:()=>Promise<void>,done:()=>Promise<void>,name:string){
  const calls:Array<{body:string;key:string;headers:Record<string,string>;url:string;data:unknown;status:number}>=[]
  let release!:()=>void
  const first=new Promise<void>(resolve=>{release=resolve})
  await page.route('**'+path,async route=>{
   const request=route.request();if(request.method()==='GET')return route.continue()
   const response=await route.fetch()
   const json=await response.json()
   expect(response.ok(),`${name}: ${response.status()} ${JSON.stringify(json)}`).toBe(true)
   calls.push({body:request.postData()!,key:request.headers()['idempotency-key'],headers:request.headers(),url:request.url(),data:json.data,status:response.status()})
   if(calls.length===1){await first;return route.abort('failed')}
   expect(calls[1].key).toBe(calls[0].key);expect(calls[1].body).toBe(calls[0].body);expect(calls[1].data).toEqual(calls[0].data)
   return route.fulfill({response})
  })
  await submit()
  await expect.poll(()=>calls.length).toBe(1)
  await expect(page.locator('.global-loading-overlay')).toBeVisible()
  await expect(scope().getByRole('button',{name:'重试',exact:true})).toHaveCount(0)
  await page.screenshot({path:info.outputPath(`${name}-committed-waiting.png`)})
  release()
  await expect(page.locator('.global-loading-overlay')).toHaveCount(0)
  const retry=scope().getByRole('button',{name:'重试',exact:true})
  await expect(retry).toBeVisible()
  if(name.startsWith('agent-')){
   await expect(page.locator('.toast-error')).toContainText('暂时无法确认操作结果')
   await expect(page.locator('.toast-error')).not.toContainText('操作未能完成')
  }
  await page.screenshot({path:info.outputPath(`${name}-unknown.png`)})
  await retry.click()
  await expect.poll(()=>calls.length).toBe(2)
  await done()
  await page.unroute('**'+path)
  // Concurrent retries after acknowledgement still return the same receipt.
  const original=calls[0]
  const responses=await Promise.all(Array.from({length:3},()=>page.request.fetch(original.url,{method:'POST',headers:{'Content-Type':'application/json','Idempotency-Key':original.key,Origin:new URL(original.url).origin,...(original.headers['x-brand-id']?{'X-Brand-ID':original.headers['x-brand-id']}:{})},data:original.body})))
  for(const response of responses){expect(response.status()).toBe(original.status);expect((await response.json()).data).toEqual(original.data)}
  evidence.push({name,path,key:original.key,body_sha256:createHash('sha256').update(original.body).digest('hex'),responses:5})
 }
 try{
  await login(platform,origin.platform,'retry_platform',adminPassword)
  await openPlatformPage(platform,'管理')
  await platform.getByRole('button',{name:'新建账号',exact:true}).click()
  let dialog=platform.getByRole('dialog')
  await dialog.getByLabel('账号',{exact:true}).fill('retry_operator')
  await dialog.getByLabel('密码',{exact:true}).fill(password)
  await dialog.locator(`input[type=checkbox][value="${fixture.platform_role_id}"]`).check()
  await replay(platform,'/api/v1/platform/platform-accounts',()=>platform.getByRole('dialog'),()=>dialog.locator('button[type=submit]').click(),()=>expect(platform.getByRole('dialog')).toHaveCount(0),'platform-create-account')
  await openPlatformPage(platform,'资金')
  await openPlatformPage(platform,'额度管理',fixture.brand_id)
  await platform.getByRole('button',{name:'额度操作',exact:true}).click()
  dialog=platform.getByRole('dialog')
  await dialog.locator('input[inputmode=decimal]').fill('100')
  await replay(platform,'/api/v1/platform/recharge-quota/grants',()=>platform.getByRole('dialog'),()=>dialog.locator('button[type=submit]').click(),()=>expect(platform.getByRole('dialog')).toHaveCount(0),'platform-quota')

  await login(agent,origin.agent,'retry_agent',password)
  await agent.getByRole('button',{name:'添加会员',exact:true}).click()
  dialog=agent.getByRole('dialog')
  await dialog.getByLabel('账号',{exact:true}).fill('retry_member');await dialog.getByLabel('密码',{exact:true}).fill(password)
  await replay(agent,'/api/v1/agent/members',()=>agent.getByRole('dialog'),()=>dialog.locator('button[type=submit]').click(),()=>expect(agent.getByRole('dialog')).toHaveCount(0),'agent-create-member')
  const row=agent.locator('.member-directory tbody tr').filter({hasText:'retry_member'})
  await row.locator('.member-menu-trigger').click();await agent.getByRole('menuitem',{name:'转移积分',exact:true}).click()
  dialog=agent.getByRole('dialog');await dialog.getByLabel('积分',{exact:true}).fill('20')
  await replay(agent,'/api/v1/agent/transfers',()=>agent.getByRole('dialog'),()=>dialog.locator('button[type=submit]').click(),()=>expect(agent.getByRole('dialog')).toHaveCount(0),'agent-transfer')
  await agent.goto(origin.agent+'/#/children');await agent.getByRole('button',{name:'添加下级代理',exact:true}).click()
  dialog=agent.getByRole('dialog');await dialog.getByLabel('账号',{exact:true}).fill('retry_child');await dialog.getByLabel('密码',{exact:true}).fill(password);await dialog.getByLabel('占成比例',{exact:true}).fill('0')
  await replay(agent,'/api/v1/agent/children',()=>agent.getByRole('dialog'),()=>dialog.locator('button[type=submit]').click(),()=>expect(agent.getByRole('dialog')).toHaveCount(0),'agent-create-child')

  await login(brand,origin.brand,'retry_brand',adminPassword)
  await openBrandPage(brand,'会员管理')
  const memberRow=brand.locator('.member-directory-table tbody tr').filter({hasText:'retry_member'})
  await memberRow.locator('.member-menu-trigger').click();await brand.getByRole('menuitem',{name:'充值',exact:true}).click()
  dialog=brand.getByRole('dialog');await dialog.locator('input[inputmode=decimal]').fill('50')
  await replay(brand,'/api/v1/admin/recharges/manual',()=>brand.getByRole('dialog'),()=>dialog.locator('button[type=submit]').click(),()=>expect(brand.getByRole('dialog')).toHaveCount(0),'brand-manual-recharge')

  await login(user,origin.user+'/login','retry_member',password)
  await user.goto(origin.user+'/configured/'+fixture.game_id)
  await user.locator('.configured-options button').first().click()
  await user.locator('.configured-points-control input').fill('10')
  await user.getByRole('button',{name:'投注',exact:true}).click()
  await replay(user,'/api/v1/configured/orders',()=>user.locator('.betting-sheet'),()=>user.getByRole('button',{name:'确认投注',exact:true}).click(),()=>expect(user.locator('.betting-sheet')).toHaveCount(0),'user-bet')
  const wallet=await user.request.get(origin.user+'/api/v1/wallet');expect(wallet.ok()).toBe(true);expect((await wallet.json()).data.available_points).toBe('60')
  const agentWallet=await agent.request.get(origin.agent+'/api/v1/agent/wallet');expect((await agentWallet.json()).data.available_points).toBe('80')
  const quota=await platform.request.get(origin.platform+'/api/v1/platform/recharge-quota',{headers:{'X-Brand-ID':fixture.brand_id}});expect((await quota.json()).data.available).toBe('1050')
  const other='0199a000-0000-7000-8000-000000000002'
  expect((await brand.request.get(origin.brand+'/api/v1/admin/users',{headers:{'X-Brand-ID':other}})).status()).toBe(403)
  expect((await user.request.get(origin.user+'/api/v1/b/harbor/wallet')).status()).toBe(401)
  expect(errors).toEqual([])
  expect(evidence).toHaveLength(7)
  writeFileSync(process.env.RETRY_REAL_FIXTURE_FILE!+'.browser-passed',JSON.stringify(evidence,null,2),{mode:0o600})
 }finally{await Promise.all(contexts.map(c=>c.close()))}
})
