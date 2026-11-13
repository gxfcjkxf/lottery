import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
import {installLayoutApi,installWalletLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
import {newAnnouncement} from '../../shared/src/announcements'
const actor='bbbbbbbb-bbbb-4bbb-8bbb-000000000001'
for(const locale of ['en','zh-CN'] as const){
 test(`player session check updates profile and ignores pre-logout reads ${locale}`,async({page},info)=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  let name='Before refresh',signedIn=true,hold=false
  let release:(()=>Promise<void>)|undefined
  const profile=()=>({user:{id:gameId,username:'session_member',status:'active'},member:{id:gameId,brand_id:gameId,status:'normal',display_name:name,joined_at:'2026-10-17T00:00:00Z'}})
  await page.route('**/api/v1/me',async route=>{
   if(hold){const old=profile();await new Promise<void>(resolve=>{release=async()=>{await route.fulfill({json:{success:true,data:old}});resolve()}});return}
   if(!signedIn)return route.fulfill({status:401,json:{success:false,error:{code:'AUTH_SESSION_REVOKED',message:'signed out'}}})
   return route.fulfill({json:{success:true,data:profile()}})
  })
  await page.route('**/api/v1/auth/logout',route=>{expect(route.request().method()).toBe('POST');signedIn=false;return route.fulfill({json:{success:true,data:{}}})})
  await page.goto(origins.user+'/account')
  await expect(page.locator('.account-identity')).toContainText(name)
  name='After refresh';await page.evaluate(()=>window.dispatchEvent(new Event('focus')))
  await expect(page.locator('.account-identity')).toContainText(name)
  hold=true;await page.evaluate(()=>window.dispatchEvent(new Event('focus')))
  await expect.poll(()=>Boolean(release)).toBe(true)
  await page.locator('.workspace-menu-trigger').click()
  await page.locator('.workspace-menu').getByRole('button',{name:locale==='en'?'Sign out':'退出登录',exact:true}).click()
  await expect(page).toHaveURL(/\/login$/)
  await release!()
  await expect(page.locator('.account-identity')).toHaveCount(0)
  await expect(page).toHaveURL(/\/login$/)
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`player-session-${locale}.png`)})
 })
 test(`agent capability refresh rereads the current directory ${locale}`,async({page},info)=>{
  await installLayoutApi(page,locale)
  let allowed=true,reads=0
  await page.route('**/api/v1/agent/**',route=>{
   const path=new URL(route.request().url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{agent_account_id:actor,username:'agent_trial'},agent:{id:actor,brand_id:gameId,depth:1,version:1,policy_version:1,config:{can_create_children:allowed}},code:null,user_origin:null,brand_code:''})
   if(path.endsWith('/children')){reads++;return reply({items:[],total_count:'0',limit:20,offset:0})}
   if(path.endsWith('/auth/config'))return reply({captcha_enabled:false})
   throw Error('Unexpected agent request '+path)
  })
  await page.goto(origins.agent+'/#/children')
  const create=page.getByRole('button',{name:locale==='en'?'Add subagent':'添加下级代理',exact:true})
  await expect(create).toBeVisible();const before=reads
  allowed=false;await page.evaluate(()=>window.dispatchEvent(new Event('focus')))
  await expect(create).toHaveCount(0);await expect.poll(()=>reads).toBeGreaterThan(before)
  allowed=true;await page.evaluate(()=>window.dispatchEvent(new Event('focus')))
  await expect(create).toBeVisible()
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`agent-session-${locale}.png`)})
 })
 test(`obsolete announcement receipt cannot expire the new member ${locale}`,async({page})=>{
  let views=0,release:(()=>Promise<void>)|undefined
  const notice={id:actor,brand_id:gameId,version:1,generation:1,status:'published',state:'active',published_at:'2026-11-11T00:00:00Z',updated_at:'2026-11-11T00:00:00Z',eligible:true,config:{...newAnnouncement().config,content:{en:{title:'Session notice',html:'<p>Notice</p>',button:'',url:''},'zh-CN':{title:'会话公告',html:'<p>公告</p>',button:'',url:''}}}}
  await page.route('**/api/**',async route=>{
   if(route.request().method()==='GET')return route.fulfill({json:{success:true,data:{items:[notice]}}})
   const body=route.request().postDataJSON()
   if(body.action==='view'&&++views===1){await new Promise<void>(resolve=>{release=async()=>{await route.fulfill({status:401,json:{success:false,error:{code:'AUTH_SESSION_REVOKED',message:'old receipt'}}});resolve()}});return}
   return route.fulfill({json:{success:true,data:{recorded:true}}})
  })
  const path='/@fs'+resolve('user-web/src/Announcements.vue')
  await page.route('**/__announcement-session',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp,h,reactive}from'/node_modules/.vite/deps/vue.js';import Component from'${path}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('user-web/src/workspace.css')}';import'/@fs${resolve('shared/src/controls.css')}';const props=reactive({brandId:'${gameId}',memberId:'${gameId}',locale:'${locale}',path:'/'});window.noticeProps=props;window.expired=0;createApp({render:()=>h(Component,{...props,onAuthExpired:()=>window.expired++})}).mount('#app')</script>`}))
  await page.goto(origins.user+'/__announcement-session')
  await expect.poll(()=>Boolean(release)).toBe(true)
  await page.evaluate(id=>{(window as any).noticeProps.memberId=id},actor)
  await expect.poll(()=>views).toBe(2)
  await release!()
  expect(await page.evaluate(()=>(window as any).expired)).toBe(0)
  await expect(page.getByRole('dialog')).toBeVisible()
 })
 test(`player session revocation is applied by the background check ${locale}`,async({page})=>{
  await installLayoutApi(page,locale);await installWalletLayoutApi(page)
  let revoked=false
  await page.route('**/api/v1/me',route=>revoked?route.fulfill({status:401,json:{success:false,error:{code:'AUTH_SESSION_REVOKED',message:'revoked'}}}):route.fulfill({json:{success:true,data:{user:{id:gameId,username:'session_member',status:'active'},member:{id:gameId,brand_id:gameId,status:'normal',display_name:'Session trial',joined_at:'2026-10-17T00:00:00Z'}}}}))
  await page.goto(origins.user+'/account');await expect(page.locator('.account-identity')).toContainText('Session trial')
  revoked=true;await page.evaluate(()=>window.dispatchEvent(new Event('focus')))
  await expect(page).toHaveURL(/\/login$/);await expect(page.locator('.account-identity')).toHaveCount(0)
 })

}
