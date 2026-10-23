import {test,expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {createRequire} from 'node:module'
import {resolve} from 'node:path'
const require=createRequire(resolve('shared/package.json')),jsQR=require('jsqr'),{PNG}=require('pngjs')
import {configuredDemo} from '../../shared/src/configured-rules'
import {installLayoutApi,installOrdersLayoutApi,origins,gameId,theme,expectNoPageOverflow,expectInsideViewport} from './fixture'
async function expectMobileLinkShare(page:import('@playwright/test').Page,container:import('@playwright/test').Locator,title:string,url:string,zh:boolean){
 const mobile=await page.evaluate(()=>matchMedia('(max-width:700px), (pointer:coarse)').matches)
 await page.evaluate(()=>{(window as any).__sharedLinks=[];Object.defineProperty(navigator,'share',{configurable:true,value:async(data:ShareData)=>{(window as any).__sharedLinks.push(data)}})})
 const share=container.getByRole('button',{name:zh?'分享':'Share',exact:true})
 await expect(share).toBeVisible({visible:mobile})
 if(mobile){await share.click();await expect.poll(()=>page.evaluate(()=>(window as any).__sharedLinks.length)).toBe(1);expect(await page.evaluate(()=>(window as any).__sharedLinks)).toEqual([{title,url}])}
}
export function registerAgentPortalTests(reflow=false){
 for(const locale of ['en','zh-CN'] as const)test(`agent portal shares console navigation and account security (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',memberId='aaaaaaaa-aaaa-4aaa-8aaa-000000000009',at='2026-10-21T00:00:00Z'
  await page.addInitScript(()=>{Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async(value:string)=>{const w=window as any;if(w.__copyFailure)throw Error('Denied');(w.__copiedLinks??=[]).push(value)}}})})
  await installLayoutApi(page,locale)
  await page.route('**/api/v1/context',route=>route.fulfill({json:{success:true,data:{brand:{id:gameId,code:'layout',name:'Base brand',status:'active',default_locale:'en',timezone:'Asia/Singapore',config_version:1,theme:{...theme,display_name:'Partner Brand',logo_text:'PB',logo_url:'/icons/agent-brand.png',favicon_url:'/icons/agent-brand.png'}},available_locales:['en','zh-CN'],features:{pwa:false,real_payments:false},terms:{privacy_policy_version:'layout-1',service_terms_version:'layout-1'}}}}))
  await page.route('**/icons/agent-brand.png',route=>route.fulfill({contentType:'image/png',body:readFileSync('user-web/public/icons/icon-192.png')}))
  const reads:string[]=[],writes:{key:string|undefined;body:string|null}[]=[]
  const root={id:gameId,agent_account_id:gameId,brand_id:gameId,depth:1,config:{can_create_children:true,mode:'loss',ratio:'0.2',status:'active'},effective_mode:'loss',policy_version:1,version:1}
  const member={id:memberId,username:'member_a',display_name:'Member A',phone:'',status:'active',joined_at:at,available_points:'100',agent_username:'agent_a',is_direct:true}
  const snapshot=configuredDemo(JSON.parse(readFileSync('backend/internal/rules/config/canada_niuniu.json','utf8')))
  const standard={kind:'standard',id:gameId,game_name:'Game A',period_no:'123',placed_at:at,order:{id:gameId,status:'won',multiplier:'1',settled_at:at,payout_entry_id:gameId,total_points:'10',prize_points:'20',selection_normalized:{digits:[[1],[2],[3]],attributes:{color:['red']},features:{sum:['big']}}}}
  const configured={kind:'configured',id:'configured-history',game_name:'Configured game',period_no:'124',placed_at:at,order:{id:'configured-history',game_name:'Configured game',period_no:'124',status:'settled',snapshot,quote:{bet_points:'10',reserved_points:'200',lines:[{play_id:'aa',options:['aa'],plate:'A',stake:'10',prices:{aa:'2'}}]},result:{prize_points:'0',refund_points:'10',consumed_points:'190',lines:[{outcome:'lost',prize_points:'0'}]}}}
  const ticket={period_no:'125',status:'placed',total_points:'10',prize_points:'0',refund_points:'0',effective_points:'0',reserved_points:'0',lost_points:'0',items:[],attributes:{}}
  const betItems=[standard,configured,{kind:'mark-six',id:'mark-history',game_name:'Mark game',period_no:'125',placed_at:at,order:{...ticket,id:'mark-history',plate:'A'}},{kind:'canada',id:'canada-history',game_name:'Canada game',period_no:'125',placed_at:at,order:{...ticket,id:'canada-history',profile:'canada_niuniu'}}]
  let lose=true,release=()=>{throw Error('No request pending')}
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/v1/agent/**',async route=>{
   const req=route.request(),url=new URL(req.url()),path=url.pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(req.method()==='POST'){
    expect(path).toBe('/api/v1/agent/password');writes.push({key:req.headers()['idempotency-key'],body:req.postData()});expect(writes.at(-1)?.key).toBeTruthy()
    if(lose){lose=false;return route.abort('failed')}
    await new Promise<void>(resolve=>{release=resolve});return reply({audit_log_id:gameId})
   }
   reads.push(path)
   if(path.endsWith('/me'))return reply({account:{agent_account_id:gameId,username:'agent_a'},agent:root,code:'A1B2C3D4E5F607182930ABCD'})
   if(path.endsWith('/wallet'))return reply({commission_points:'5',available_points:'5'})
   if(path.endsWith('/commission'))return reply({items:[{id:gameId,created_at:at,entry_type:'commission',points:'7'},{id:memberId,created_at:at,entry_type:'commission_correction',points:'-2'}],total_count:'2',limit:20,offset:0,credited_points:'7',debited_points:'2'})
   if(path.endsWith('/children'))return reply({items:[],total_count:'0',limit:20,offset:0})
   if(path.endsWith('/summary'))return reply({member,wallet:{available_points:'100'},net_points:'10',unsettled_points:'0'})
   if(path.endsWith('/bets'))return reply(url.searchParams.get('cursor')==='page-2'?{items:[{...standard,id:'second-page',game_name:'Second page'}],next_cursor:null}:{items:betItems,next_cursor:'page-2'})
   if(path.endsWith('/members'))return reply({items:[member],total_count:'1',limit:20,offset:0,summary:{member_count:'1',available_points:'100',bet_count:'1',stake_points:'10',unsettled_points:'0',net_points:'10'}})
   throw Error('Unexpected agent endpoint '+path)
  })
  await page.goto(origins.agent)
  const shell=page.locator('.management-console');await expect(shell).toBeVisible()
  await expect(page).toHaveTitle(zh?'代理管理':'Agent management');await expect(page.locator('.brand-lockup b')).toHaveText('Partner Brand');await expect(page.locator('.brand-lockup img')).toHaveAttribute('src','/icons/agent-brand.png');await expect.poll(()=>page.locator('.brand-lockup img').evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBeGreaterThan(0)
  await expect(page.locator('link[rel=icon]')).toHaveCount(1);await expect(page.locator('link[data-brand-favicon]')).toHaveAttribute('href','/icons/agent-brand.png')
  const preset=page.viewportSize()!
  const navigate=async(id:string)=>{const nav=page.locator(preset.width<=700?'.mobile-nav':'.side-nav');await nav.locator(`a[href="#/${id}"]`).click();await expect(page).toHaveURL(new RegExp('#/'+id+'$'))}
  const labels=zh?['会员管理','下级代理','会员注单','佣金']:['Member management','Subagents','Member bets','Commission']
  const visibleNav=page.locator(preset.width<=700?'.mobile-nav':'.side-nav')
  for(const label of labels)await expect(visibleNav.getByRole('link',{name:label,exact:true})).toBeVisible()
  if(preset.width>700)await expect(page.locator('.sidebar')).toBeVisible();else await expect(page.locator('.sidebar')).toBeHidden()
  await expect(page.locator('.member-directory')).toContainText('member_a');await expect(page.locator('.member-directory table.console-table')).toHaveCount(1);expect(reads.some(p=>p.endsWith('/commission')||p.endsWith('/wallet')||p.endsWith('/children')||p.endsWith('/bets'))).toBe(false)
  await page.screenshot({path:info.outputPath(`agent-members-${locale}.png`)})
  await navigate('children');await expect(page.locator('.children-directory')).toBeVisible();await expect(page.locator('.children-directory td.empty-state')).toHaveAttribute('colspan','3');await expect(page.locator('.children-directory td.empty-state')).toHaveCSS('display','table-cell');await expect(page.locator('.member-directory')).toHaveCount(0)
  await navigate('bets');await page.getByRole('button',{name:zh?'查看注单':'View bets',exact:true}).click();await expect(page).toHaveURL(new RegExp('member='+memberId));await expect(page.locator('.member-bets')).toContainText('Game A');await expect(page.locator('.member-bets .standard-ticket')).toContainText(zh?'已中奖':'Won');await expect(page.locator('.member-bets [data-testid=order-prize]')).toContainText(zh?'已入账':'Applied');await expect(page.locator('.member-bets [data-testid=selection-summary]')).toContainText(zh?'位置 1':'Position 1');await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`agent-bets-${locale}.png`)});await expect(page.locator('.children-directory')).toHaveCount(0)
  await expect(page.locator('.member-bets .history-record')).toHaveCount(4);await expect(page.locator('.member-bets .configured-order-card')).toContainText(zh?'风险冻结积分: 200':'Reserved risk points: 200');await expect(page.locator('.member-bets .mark-six-orders')).toHaveCount(1);await expect(page.locator('.member-bets .canada-orders')).toHaveCount(1)
  const betsPanel=page.locator('.member-bets');await betsPanel.getByRole('button',{name:zh?'下一页':'Next',exact:true}).click();await expect(betsPanel).toContainText('Second page');await expect(betsPanel.locator('.history-record')).toHaveCount(1);await expect(betsPanel.getByRole('button',{name:zh?'下一页':'Next',exact:true})).toBeDisabled();await betsPanel.getByRole('button',{name:zh?'上一页':'Previous',exact:true}).click();await expect(betsPanel.locator('.history-record')).toHaveCount(4)
  for(const width of reflow?[320,359,700,701,900,1440]:[preset.width]){if(reflow)await page.setViewportSize({width,height:preset.height});await expectNoPageOverflow(page)}if(reflow)await page.setViewportSize(preset)
  await page.reload();await expect(page.locator('.member-bets')).toContainText('member_a')
  await navigate('commission');await expect(page.locator('.commission-page thead')).toContainText(zh?'入账时间':'Posted at');await expect(page.locator('.commission-page')).toContainText('+7');await expect(page.locator('.commission-page')).toContainText('-2');await expect(page.locator('.member-bets')).toHaveCount(0)
  for(const width of reflow?[320,359,700,701,900,1440]:[preset.width]){if(reflow)await page.setViewportSize({width,height:preset.height});await expectNoPageOverflow(page)}
  if(reflow)await page.setViewportSize(preset)
  await page.screenshot({path:info.outputPath(`agent-commission-${locale}.png`)})
  await page.goBack();await expect(page.locator('.member-bets')).toContainText('Game A');await page.goForward();await expect(page.locator('.commission-page')).toContainText('+7')
  await page.locator('.account-trigger').click();await page.getByRole('menuitem',{name:zh?'个人信息':'Profile',exact:true}).click()
  const profile=page.getByRole('dialog',{name:zh?'个人信息':'Profile',exact:true});await expect(profile).toContainText('agent_a');await expectInsideViewport(profile)
  const registrationUrl=`${origins.user}/register?agent_code=A1B2C3D4E5F607182930ABCD`
  await expect(profile.locator('.registration-link code')).toHaveText(registrationUrl)
  await profile.getByRole('button',{name:zh?'复制链接':'Copy link',exact:true}).click();await expect(profile.getByRole('status')).toHaveText(zh?'链接已复制':'Link copied');expect(await page.evaluate(()=>(window as any).__copiedLinks)).toEqual([registrationUrl])
  const qrImage=profile.getByRole('img',{name:zh?'注册链接二维码':'Registration link QR code',exact:true});await expect(qrImage).toBeVisible()
  const png=PNG.sync.read(Buffer.from((await qrImage.getAttribute('src'))!.split(',')[1]!, 'base64'));expect(png.width).toBe(768);expect(png.height).toBe(768);expect(jsQR(new Uint8ClampedArray(png.data),png.width,png.height)?.data).toBe(registrationUrl)
  const saveQr=profile.getByRole('link',{name:zh?'保存二维码':'Save QR code',exact:true}),hint=profile.locator('.registration-qr-hint')
  await expect(profile.locator('.registration-link-address button svg')).toBeVisible();await expect(profile.locator('.registration-link-address button')).toHaveText('');const copyBox=await profile.locator('.registration-link-address button').boundingBox();expect(copyBox!.width).toBeGreaterThanOrEqual(44);expect(copyBox!.height).toBeGreaterThanOrEqual(44)
  if(await page.evaluate(()=>matchMedia('(max-width:700px), (pointer:coarse)').matches)){
   await expect(saveQr).toBeHidden();await expect(hint).toHaveText(zh?'长按二维码保存图片':'Press and hold the QR code to save the image.');await expect(hint).toBeVisible()
  }else{
   await expect(hint).toBeHidden();const downloadReady=page.waitForEvent('download');await saveQr.click();const download=await downloadReady;expect(download.suggestedFilename()).toBe('registration-qr.png');const saved=PNG.sync.read(readFileSync((await download.path())!));expect(jsQR(new Uint8ClampedArray(saved.data),saved.width,saved.height)?.data).toBe(registrationUrl)
  }
  for(const width of reflow?[320,359,700,701,900,1440]:[preset.width]){if(reflow)await page.setViewportSize({width,height:preset.height});await expectNoPageOverflow(page);await expectInsideViewport(profile);const box=await qrImage.boundingBox();expect(box!.width).toBeLessThanOrEqual(Math.min(240,width));expect(box!.width).toBeCloseTo(box!.height,1)}if(reflow)await page.setViewportSize(preset)
  await expectMobileLinkShare(page,profile,'Partner Brand',registrationUrl,zh)
  await page.screenshot({path:info.outputPath(`agent-registration-link-${locale}.png`)})
  await page.evaluate(()=>{(window as any).__copyFailure=true});await profile.getByRole('button',{name:zh?'复制链接':'Copy link',exact:true}).click();await expect(profile.getByRole('alert')).toHaveText(zh?'复制失败，请手动复制链接。':'Unable to copy. Copy the link manually.')
  await profile.locator('.detail-modal-close').click()
  await page.locator('.account-trigger').click();await page.getByRole('menuitem',{name:zh?'修改密码':'Change password',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:zh?'修改密码':'Change password',exact:true});await expectInsideViewport(dialog);await expect(dialog.locator('input[type=password]')).toHaveCount(3)
  const current=dialog.getByLabel(zh?'当前密码':'Current password',{exact:true}),next=dialog.getByLabel(zh?'新密码':'New password',{exact:true}),confirm=dialog.getByLabel(zh?'确认新密码':'Confirm new password',{exact:true})
  await current.fill('agent123');await next.fill('changed456');await confirm.fill('mismatch')
  await dialog.getByRole('button',{name:zh?'保存密码':'Save password',exact:true}).click();await expect(dialog.getByRole('alert')).toBeVisible();expect(writes).toHaveLength(0)
  await confirm.fill('changed456');await dialog.getByRole('button',{name:zh?'保存密码':'Save password',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
  await expect(current).toBeDisabled();await expect(next).toBeDisabled()
  const retry=dialog.getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true});await retry.click();await expect.poll(()=>writes.length).toBe(2)
  try{
   await dialog.locator('form').evaluate((form:HTMLFormElement)=>form.requestSubmit());expect(writes).toHaveLength(2)
   await expect(retry).toBeDisabled();expect(writes[0]).toEqual(writes[1]);expect(JSON.parse(writes[0].body!)).toEqual({current_password:'agent123',new_password:'changed456'})
   await page.screenshot({path:info.outputPath(`agent-password-${locale}.png`)})
  }finally{release()}
  await expect(dialog).toHaveCount(0);await expect(page.locator('.login-entry')).toBeVisible();await expect(page.getByRole('status').filter({hasText:zh?'密码已修改':'Password changed'})).toBeVisible();expect(errors).toEqual([])
 })
}

export function registerSharedAccountMenuTests(){
 for(const locale of ['en','zh-CN'] as const)test(`shared account menu and tables work across management apps (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN';await installLayoutApi(page,locale)
  await page.route('**/api/v1/admin/**',async route=>{
   expect(route.request().method()).toBe('GET');const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'shared_operator',super_admin:false,brand_ids:[gameId],permissions:[],platform_permissions:[],permissions_by_brand:{[gameId]:['user.view.brand']}}})
   if(path.endsWith('/brands'))return reply({items:[{id:gameId,name:'Aurora',code:'aurora',status:'active'}]})
   if(path.endsWith('/users'))return reply({items:[],total_count:'0',limit:100,offset:0})
   if(path.endsWith('/brand-presentation'))return reply({brand_id:gameId,version:1,status:'active',base_name:'Aurora',config:{},effective:{display_name:'Aurora',logo_text:'A',logo_url:null,favicon_url:null,primary_color:'#059669',accent_color:'#f59e0b',success_color:'#10b981',warning_color:'#f59e0b',danger_color:'#ef4444',font_family:'system',font_scale:'standard',radius:'round',shadow:'subtle',default_locale:'en',available_locales:['en','zh-CN'],content:{en:{tagline:'',announcement:''},'zh-CN':{tagline:'',announcement:''}}}})
   throw Error('Unexpected shared account read '+path)
  })
  await page.route('**/api/v1/platform/**',async route=>{
   expect(route.request().method()).toBe('GET');const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/presentation'))return reply({display_name:'Platform',logo_text:'P',logo_url:''})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'shared_platform',super_admin:true,platform_permissions:[]}})
   if(path.endsWith('/platform-settings'))return reply({version:1,display_name:'Platform',logo_text:'P',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',updated_at:'2026-10-21T00:00:00Z'})
   if(path.endsWith('/brands'))return reply({items:[]})
   throw Error('Unexpected shared platform read '+path)
  })
  await page.route('**/api/v1/agent/**',async route=>{
   expect(route.request().method()).toBe('GET');const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{agent_account_id:gameId,username:'shared_agent'},agent:{id:gameId,depth:1,config:{can_create_children:false}},code:'A1B2C3D4E5F607182930ABCD'})
   if(path.endsWith('/members'))return reply({items:[],total_count:'0',limit:20,offset:0,summary:{member_count:'0',available_points:'0',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}})
   throw Error('Unexpected shared agent read '+path)
  })
  const tableStyles:unknown[]=[],passwordStyles:unknown[]=[],profileStyles:unknown[]=[]
  const dialogStyle=async(dialog:import('@playwright/test').Locator,selectors:string[])=>dialog.evaluate((root,selectors)=>Object.fromEntries(selectors.map(selector=>{const css=getComputedStyle(root.querySelector(selector)!);return [selector,Object.fromEntries(['fontSize','fontWeight','color','backgroundColor','padding','gap','display','borderRadius','minHeight','gridTemplateColumns'].map(key=>[key,css[key as keyof CSSStyleDeclaration]]))]})),selectors)
  for(const url of [origins.brand,origins.platform,origins.agent]){
   await page.goto(url);const trigger=page.locator('.account-trigger');await expect(trigger).toBeVisible();await trigger.click()
   const menu=page.getByRole('menu');await expect(menu).toBeVisible();await expectInsideViewport(menu)
   await page.keyboard.press('Escape');await expect(menu).toHaveCount(0)
   await trigger.click();await page.getByRole('menuitem',{name:zh?'个人信息':'Profile',exact:true}).click();const profile=page.getByRole('dialog',{name:zh?'个人信息':'Profile',exact:true});await expect(profile).toBeVisible();await expectInsideViewport(profile);await expect(profile).toContainText(url===origins.brand?'shared_operator':url===origins.platform?'shared_platform':'shared_agent');const profileStyle=await dialogStyle(profile,['dl','dt','dd','.detail-modal-close']);profileStyles.push(profileStyle);expect(profileStyle).toEqual(profileStyles[0]);await page.screenshot({path:info.outputPath(`profile-${new URL(url).port}-${locale}.png`)});await profile.locator('.detail-modal-close').click()
   await trigger.click();await page.getByRole('menuitem',{name:zh?'修改密码':'Change password',exact:true}).click();const password=page.getByRole('dialog',{name:zh?'修改密码':'Change password',exact:true});await expect(password.locator('input[type=password]')).toHaveCount(3);await expectInsideViewport(password);await expectNoPageOverflow(page);const passwordStyle=await dialogStyle(password,['form','fieldset','label','input','.form-actions','.button-primary','.detail-modal-close']);passwordStyles.push(passwordStyle);expect(passwordStyle).toEqual(passwordStyles[0]);await page.screenshot({path:info.outputPath(`password-${new URL(url).port}-${locale}.png`)});await page.keyboard.press('Escape');await expect(password).toHaveCount(0)
   const tableStyle=await page.locator('.management-console').evaluate(root=>{
    const wrap=document.createElement('div');wrap.className='table-wrap';wrap.innerHTML='<table class="member-directory-table console-table console-responsive-table"><thead><tr><th>Account</th><th>Points</th><th>Details</th></tr></thead><tbody><tr><td data-label="Account">table_member</td><td data-label="Points">100</td><td data-label="Details">View</td></tr><tr><td data-label="Account">other_member</td><td data-label="Points">200</td><td data-label="Details">View</td></tr></tbody></table>';root.querySelector('.page-content,.content')!.append(wrap)
    const read=(selector:string)=>{const css=getComputedStyle(wrap.querySelector(selector)!);return Object.fromEntries(['fontSize','fontWeight','lineHeight','color','backgroundColor','paddingTop','paddingRight','paddingBottom','paddingLeft','borderBottomWidth','borderBottomColor','textTransform','letterSpacing','position','textAlign','height','whiteSpace'].map(key=>[key,css[key as keyof CSSStyleDeclaration]]))}
    const result={head:read('th'),cell:read('tbody td'),last:read('tbody td:last-child')};wrap.remove();return result
   })
   tableStyles.push(tableStyle);expect(tableStyle).toEqual(tableStyles[0])
   await page.screenshot({path:info.outputPath(`shared-account-${url===origins.brand?'brand':url===origins.platform?'platform':'agent'}-${locale}.png`)})
  }
 })
 for(const locale of ['en','zh-CN'] as const)for(const kind of ['members','children'] as const)test(`agent shared create fields retain frozen requests (${kind}, ${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',at='2026-10-21T00:00:00Z',writes:Array<{body:unknown;key:string|undefined}>=[],errors:string[]=[]
  await installLayoutApi(page,locale);page.on('pageerror',cause=>errors.push(cause.message))
  const own={account:{agent_account_id:gameId,username:'create_reviewer'},agent:{id:gameId,brand_id:gameId,depth:1,policy_version:7,version:3,effective_mode:'loss',config:{can_create_children:true,ratio:'0.2',mode:'loss',status:'active'}},code:'CREATE-CODE'}
  let release:()=>void=()=>{throw Error('No create request pending')}
  await page.route('**/api/v1/agent/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(req.method()==='POST'){
    expect(path).toBe(`/api/v1/agent/${kind}`);writes.push({body:req.postDataJSON(),key:req.headers()['idempotency-key']});expect(writes.at(-1)!.key).toBeTruthy()
    if(writes.length===1)return route.abort('failed')
    await new Promise<void>(resolve=>{release=resolve});return reply({id:gameId,created_at:at})
   }
   expect(req.method()).toBe('GET')
   if(path.endsWith('/me'))return reply(own)
   if(path.endsWith('/children'))return reply({items:[],limit:20,offset:0,total_count:'0'})
   if(path.endsWith('/members'))return reply({items:[],limit:20,offset:0,total_count:'0',summary:{member_count:'0',available_points:'0',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}})
   throw Error('Unexpected create test read '+path)
  })
  await page.goto(`${origins.agent}/#/${kind}`)
  await page.getByRole('button',{name:kind==='members'?(zh?'添加会员':'Add member'):(zh?'添加下级代理':'Add subagent'),exact:true}).click()
  const dialog=page.getByRole('dialog');await expectInsideViewport(dialog);await expect(dialog.locator('.detail-modal-close')).toHaveCSS('border-top-width','0px')
  await dialog.getByLabel(zh?'账号':'Account',{exact:true}).fill('created_account')
  await dialog.getByLabel(zh?'密码':'Password',{exact:true}).fill('changed456')
  if(kind==='members'){await dialog.getByLabel(zh?'昵称':'Display name',{exact:true}).fill('Created member');await dialog.getByLabel(zh?'手机号':'Phone',{exact:true}).fill('12345678')}
  else{await dialog.getByLabel(zh?'占成比例':'Profit share ratio',{exact:true}).fill('15.5');await dialog.locator('input[type=checkbox]').check()}
  await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`agent-create-${kind}-${locale}.png`)})
  await dialog.getByRole('button',{name:zh?'保存':'Save',exact:true}).click();await expect.poll(()=>writes.length).toBe(1)
  await expect(dialog.locator('input').first()).toBeDisabled();await expect(dialog.getByRole('button',{name:zh?'取消':'Cancel',exact:true})).toBeDisabled()
  await page.keyboard.press('Escape');await expect(dialog).toBeVisible()
  await dialog.getByRole('button',{name:zh?'重试原请求':'Retry original request',exact:true}).click();await expect.poll(()=>writes.length).toBe(2)
  try{
   expect(writes[1]).toEqual(writes[0])
   const base={username:'created_account',password:'changed456'}
   expect(writes[0]!.body).toEqual(kind==='members'?{...base,display_name:'Created member',phone:'12345678'}:{...base,policy_version:7,parent_id:gameId,parent_version:3,config:{ratio:'0.155',mode:null,status:'active',can_create_children:true}})
   await dialog.locator('form').evaluate((form:HTMLFormElement)=>{form.requestSubmit();form.requestSubmit()});expect(writes).toHaveLength(2)
   await expect(dialog.locator('input').first()).toBeDisabled()
  }finally{release()}
  await expect(dialog).toHaveCount(0);await expect(page.getByRole('status')).toContainText(zh?'已保存':'Saved');expect(errors).toEqual([])
 })

 for(const locale of ['en','zh-CN'] as const)test(`agent registration link reaches user registration attribution (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',code='A1B2C3D4E5F607182930ABCD',referral='B1B2C3D4E5F607182930ABCD',at='2026-10-21T00:00:00Z',registerBodies:any[]=[],errors:string[]=[]
  await installLayoutApi(page,locale);page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/v1/agent/**',route=>{expect(route.request().method()).toBe('GET');const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}});if(path.endsWith('/me'))return reply({account:{agent_account_id:gameId,username:'link_agent'},agent:{id:gameId,depth:1,config:{can_create_children:false}},code});if(path.endsWith('/members'))return reply({items:[],total_count:'0',limit:20,offset:0,summary:{}});throw Error('Unexpected invitation test read '+path)})
  await page.goto(origins.agent);await page.locator('.account-trigger').click();await page.getByRole('menuitem',{name:zh?'个人信息':'Profile',exact:true}).click();const link=await page.locator('.registration-link code').textContent();expect(link).toBe(`${origins.user}/register?agent_code=${code}`)
  let validations=0
  await page.route('**/api/v1/join-codes/validate?*',route=>{expect(route.request().method()).toBe('GET');expect(new URL(route.request().url()).searchParams.get('agent_code')).toBe(code);validations++;return route.fulfill({json:{success:true,data:{valid:true}}})})
  await page.route('**/api/v1/auth/register',route=>{expect(route.request().method()).toBe('POST');registerBodies.push(route.request().postDataJSON());return route.fulfill({status:400,json:{success:false,error:{code:'USERNAME_TAKEN',message:'Account exists'}}})})
  await page.goto(link!);await expect(page.locator('.auth-form')).toBeVisible();await expect.poll(()=>validations).toBeGreaterThan(0)
  await page.locator('.auth-form').getByLabel(zh?'账号':'Account',{exact:true}).fill('invited_member');await page.locator('.auth-form input[type=password]').fill('member123');await page.locator('.combined-terms input[type=checkbox]').check();await page.locator('.auth-form').getByRole('button',{name:zh?'注册':'Register',exact:true}).click();await expect.poll(()=>registerBodies.length).toBe(1);expect(registerBodies[0]).toMatchObject({username:'invited_member',agent_code:code});expect(registerBodies[0]).not.toHaveProperty('referral_code')
  await installOrdersLayoutApi(page)
  await page.route('**/api/v1/me/join-codes?*',route=>{expect(route.request().method()).toBe('GET');return route.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,items:[{id:gameId,brand_id:gameId,owner_member_id:gameId,kind:'referral',agent_id:null,code:referral,status:'active',starts_at:null,expires_at:null,version:1,usable:true,created_at:at,updated_at:at}],limit:20,offset:0,total_count:'1'}}})})
  await page.goto(origins.user+'/invites');const card=page.locator('.join-code-card');await expect(card.locator('code')).toHaveText(`${origins.user}/register?referral_code=${referral}`);const image=card.getByRole('img',{name:zh?'注册链接二维码':'Registration link QR code',exact:true});await expect(image).toBeVisible();const png=PNG.sync.read(Buffer.from((await image.getAttribute('src'))!.split(',')[1]!,'base64'));expect(jsQR(new Uint8ClampedArray(png.data),png.width,png.height)?.data).toBe(`${origins.user}/register?referral_code=${referral}`);await expect(card.locator('.registration-link-address button svg')).toBeVisible();await expect(card.locator('.registration-link-address button')).toHaveAttribute('aria-label',zh?'复制链接':'Copy link');const mobile=await page.evaluate(()=>matchMedia('(max-width:700px), (pointer:coarse)').matches);await expect(card.locator('.registration-qr-hint')).toBeVisible({visible:mobile});await expect(card.getByRole('link',{name:zh?'保存二维码':'Save QR code',exact:true})).toBeVisible({visible:!mobile});await expectMobileLinkShare(page,card,'Layout trial',`${origins.user}/register?referral_code=${referral}`,zh);await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`referral-registration-link-${locale}.png`)});expect(errors).toEqual([])
 })

}
