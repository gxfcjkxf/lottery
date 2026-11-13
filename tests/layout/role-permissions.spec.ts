import {test,expect} from '@playwright/test'
import {resolve} from 'node:path'
const origin=`http://127.0.0.1:${process.env.LOTTERY_LAYOUT_PORT_BASE ?? 15273}`,id='aaaaaaaa-aaaa-4aaa-8aaa-000000000001'
for(const locale of ['en','zh-CN'] as const)for(const surface of ['platform','platform-brand','admin'] as const){
 test(`grouped role permissions ${surface} ${locale}`,async({page},info)=>{
  const zh=locale==='zh-CN',admin=surface==='admin',scope=surface==='platform'?'platform':'brand'
  const permissions=['admin.view','admin.write','admin.delete','role.view','role.write','audit.view'].map(p=>`${p}.${scope}`)
  const errors:string[]=[],writes:any[]=[],roles:any[]=[]
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   const ok=(data:any,status=200)=>route.fulfill({status,json:{success:true,data}})
   if(req.method()==='GET'){
    if(path.endsWith('permissions'))return ok({items:permissions})
    if(path.endsWith('roles'))return ok({items:roles,total_count:String(roles.length)})
    if(path.endsWith('accounts'))return ok({items:[],total_count:'0'})
   }
   if(req.method()==='POST'&&path.endsWith('roles')){
    const body=req.postDataJSON();writes.push(body)
    expect(req.headers()['idempotency-key']).toBeTruthy()
    expect(req.headers()['x-brand-id']).toBe(scope==='brand'?id:undefined)
    const role={...body,id,brand_id:scope==='brand'?id:'',status:body.status??'active',version:1,is_bootstrap:false,audit_log_id:id}
    roles.push(role);return ok(role,201)
   }
   throw Error('Unexpected request '+req.method()+' '+path)
  })
  const props=admin?{section:'roles',brandId:id,account:{id,username:'operator',status:'active',super_admin:false,brand_ids:[id],permissions,permissions_by_brand:{[id]:permissions}}}:{brandId:scope==='brand'?id:'',locale,scope,accountId:id,grants:['admin.view.platform','role.view.platform','admin.write.platform','role.write.platform'],initialTab:'roles'}
  const app=admin?'admin-web':'platform-web',factory=admin?'createAdminI18n':'createPlatformI18n',key=admin?'adminI18nKey':'platformI18nKey'
  const componentPath='/@fs'+resolve(app+'/src/'+(admin?'AccessManagement.vue':'AccessPanel.vue'))
  const componentSource=await(await page.request.get(origin+componentPath)).text()
  const i18nPath=componentSource.match(/from "([^"\n]*\/i18n\.ts[^"\n]*)"/)![1]
  await page.route('**/__role-permissions',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'${componentPath}';import{${factory},${key}}from'${i18nPath}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(app+'/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/management.css')}';const i18n=${factory}();i18n.setLocale('${locale}');createApp(Component,${JSON.stringify(props)}).provide(${key},i18n).mount('#app')</script></html>`}))
  await page.goto(origin+'/__role-permissions')
  await page.getByRole('button',{name:admin ? (zh?'新建角色':'Create role') : (zh?'新建角色':'New role'),exact:true}).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  const picker=page.locator('.permission-picker'),accounts=picker.getByRole('checkbox',{name:zh?'账号':'Accounts',exact:true}),rolesGroup=picker.getByRole('checkbox',{name:zh?'角色':'Roles',exact:true})
  await expect(picker.getByRole('checkbox')).toHaveCount(9)
  await accounts.check()
  await expect(picker.getByRole('checkbox',{name:zh?'账号 · 编辑':'Accounts · Edit',exact:true})).toBeChecked()
  const roleView=picker.getByRole('checkbox',{name:zh?'角色 · 查看':'Roles · View',exact:true})
  await roleView.check();await expect(rolesGroup).toBeChecked({indeterminate:true})
  await picker.getByRole('checkbox',{name:zh?'账号 · 编辑':'Accounts · Edit',exact:true}).uncheck()
  await expect(accounts).toBeChecked({indeterminate:true});await accounts.check();await expect(accounts).toBeChecked()
  await accounts.uncheck();await expect(roleView).toBeChecked()
  await expect(picker.getByRole('checkbox',{name:zh?'账号 · 查看':'Accounts · View',exact:true})).not.toBeChecked()
  await picker.getByRole('button',{name:zh?'全选权限':'Select all permissions',exact:true}).click()
  await expect(picker.getByRole('checkbox',{checked:true})).toHaveCount(9)
  await picker.getByRole('button',{name:zh?'清空权限':'Clear permissions',exact:true}).click()
  await expect(picker.getByRole('checkbox',{checked:true})).toHaveCount(0)
  await accounts.check();await roleView.check()
  const form=admin?page.locator('.access-page .form').first():page.getByRole('dialog')
  await form.getByLabel(admin?(zh?'显示名称':'Display name'):(zh?'名称':'Name'),{exact:!admin}).fill('Grouped operator')
  await form.getByLabel(admin?(zh?'角色代码':'Role code'):(zh?'代码':'Code'),{exact:!admin}).fill('grouped_operator')
  if(admin){
   const dialog=page.getByRole('dialog'),save=dialog.locator('.detail-modal-footer button')
   await expect(save).toHaveAttribute('form','brand-role-editor')
   await expect(save).toBeInViewport()
   await expect(dialog.locator('.panel')).toHaveCount(0)
   const labels=dialog.locator('.role-identity > label')
   const boxes=await labels.evaluateAll(els=>els.map(el=>el.getBoundingClientRect().top))
   if(page.viewportSize()!.width>620)expect(Math.abs(boxes[0]-boxes[1])).toBeLessThan(2)
   else expect(boxes[1]).toBeGreaterThan(boxes[0])
  }
  if(!admin){
   const labels=page.locator('.role-identity > label');await expect(labels).toHaveCount(3)
   const boxes=await labels.evaluateAll(els=>els.map(e=>({top:e.getBoundingClientRect().top,text:e.textContent})))
   expect(boxes[0].text).toContain(zh?'名称':'Name');expect(boxes[1].text).toContain(zh?'代码':'Code');expect(boxes[2].text).toContain(zh?'状态':'Status')
   if(page.viewportSize()!.width>620)expect(Math.max(...boxes.map(b=>b.top))-Math.min(...boxes.map(b=>b.top))).toBeLessThan(2)
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`grouped-${surface}-${locale}.png`)})
  await page.getByRole('dialog').getByRole('button',{name:admin?(zh?'创建角色':'Create role'):(zh?'保存':'Save'),exact:true}).click()
  await expect.poll(()=>writes.length).toBe(1)
  expect(writes[0].permissions.slice().sort()).toEqual(permissions.slice(0,4).sort())
  expect(errors).toEqual([])
 })
}

for (const locale of ['en', 'zh-CN'] as const) test(`brand role catalog loading and permission changes ${locale}`, async ({page}, info) => {
 const permissions=['admin.view.brand','admin.write.brand','role.view.brand']
 let release: (()=>void)|undefined
 let failed=false, calls=0
 await page.route('**/api/**', async route=>{
  const path=new URL(route.request().url()).pathname
  if(path.endsWith('/accounts')) return route.fulfill({json:{success:true,data:{items:[],total_count:'0'}}})
  if(path.endsWith('/roles')) {
   calls++
   await new Promise<void>(resolve=>{release=resolve})
   if(failed) return route.fulfill({status:500,json:{success:false,error:{code:'INTERNAL_ERROR',message:'test failure'}}})
   return route.fulfill({json:{success:true,data:{items:[],total_count:'0'}}})
  }
  throw Error('Unexpected request '+path)
 })
 const componentPath='/@fs'+resolve('admin-web/src/AccessManagement.vue')
 const source=await(await page.request.get(origin+componentPath)).text()
 const i18nPath=source.match(/from "([^"\n]*\/i18n\.ts[^"\n]*)"/)![1]
 const account={id,super_admin:false,brand_ids:[id],permissions:[],permissions_by_brand:{[id]:permissions}}
 await page.route('**/__catalog-loading',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import{createApp,reactive}from'/node_modules/.vite/deps/vue.js';import Component from'${componentPath}';import{createAdminI18n,adminI18nKey}from'${i18nPath}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('admin-web/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/management.css')}';const props=reactive({section:'accounts',brandId:'${id}',account:${JSON.stringify(account)}});window.catalogProps=props;const i18n=createAdminI18n();i18n.setLocale('${locale}');createApp(Component,props).provide(adminI18nKey,i18n).mount('#app');</script></html>`}))
 await page.goto(origin+'/__catalog-loading')
 await expect.poll(()=>calls).toBe(1)
 const warning=page.locator('.access-page .callout')
 await expect(warning).toHaveCount(0)
 const create=page.getByRole('button',{name:locale==='en'?'Create administrator':'新建管理员',exact:true})
 // No write entry is offered before the catalog is available.
 await expect(create).toHaveCount(0)
 release!()
 await expect(create).toBeVisible()
 await expect(warning).toHaveCount(0)
 await page.evaluate(()=>{(window as any).catalogProps.account.permissions_by_brand={'aaaaaaaa-aaaa-4aaa-8aaa-000000000001':['admin.view.brand','admin.write.brand']}})
 await expect(warning).toContainText(locale==='en'?'Missing role.view':'缺少 role.view')
 expect(calls).toBe(1)
 failed=true
 await page.evaluate(()=>{(window as any).catalogProps.account.permissions_by_brand={'aaaaaaaa-aaaa-4aaa-8aaa-000000000001':['admin.view.brand','admin.write.brand','role.view.brand']}})
 await expect.poll(()=>calls).toBe(2)
 await expect(warning).toHaveCount(0)
 release!()
 await expect(warning).toContainText(locale==='en'?'Could not load the role catalog':'角色目录读取失败')
 await expect(create).toHaveCount(0)
 // A response from a previously permitted request cannot restore revoked access.
 failed=false
 const setGrants=async (grants:string[])=>page.evaluate(({id,grants})=>{(window as any).catalogProps.account.permissions_by_brand={[id]:grants}}, {id,grants})
 await setGrants(permissions)
 await expect.poll(()=>calls).toBe(3)
 await setGrants(['admin.view.brand','admin.write.brand'])
 await expect(warning).toContainText(locale==='en'?'Missing role.view':'缺少 role.view')
 const staleResponse=page.waitForResponse(r=>new URL(r.url()).pathname.endsWith('/roles'))
 release!()
 await staleResponse
 await expect(warning).toContainText(locale==='en'?'Missing role.view':'缺少 role.view')
 await expect(create).toHaveCount(0)
 await setGrants(permissions)
 await expect.poll(()=>calls).toBe(4)
 release!()
 await expect(create).toBeVisible()
 await expect(warning).toHaveCount(0)
 const viewport=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,640,1440]:[viewport.width]) {
  if(info.project.name.endsWith('reflow')) await page.setViewportSize({width,height:viewport.height})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`catalog-${locale}-${width}.png`)})
 }
})

for(const locale of ['en','zh-CN'] as const) for(const scope of ['platform','brand'] as const) test(`platform access permission and option lifecycle ${scope} ${locale}`,async({page},info)=>{
 const zh=locale==='zh-CN', grants=['admin.view.platform','admin.write.platform','role.view.platform','role.write.platform']
 let accountReads=0,roleReads=0
 const permissionReleases:Array<()=>Promise<void>>=[]
 const roleOffsets:number[]=[]
 const role=(n:number)=>({id:`aaaaaaaa-aaaa-4aaa-8aaa-${String(n).padStart(12,'0')}`,brand_id:scope==='brand'?id:'',code:`role_${n}`,name:`Role ${n}`,status:'active',version:1,is_bootstrap:false,permissions:[]})
 await page.route('**/api/**',async route=>{
  const url=new URL(route.request().url()),path=url.pathname
  const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  expect(route.request().method()).toBe('GET')
  if(path.endsWith('permissions')) {
   const index=permissionReleases.length
   await new Promise<void>(resolve=>permissionReleases.push(async()=>{await reply({items:[index===0?`admin.view.${scope}`:`role.view.${scope}`]});resolve()}))
   return
  }
  if(path.endsWith('accounts')) {accountReads++;return reply({items:[{id:'aaaaaaaa-aaaa-4aaa-8aaa-000000000002',username:'review_operator',status:'active',version:1,super_admin:scope==='platform',brand_ids:scope==='platform'?[]:[id],role_ids:[],role_codes:[]}],total_count:'1'})}
  if(path.endsWith('roles')) {
   roleReads++
   const limit=Number(url.searchParams.get('limit')),offset=Number(url.searchParams.get('offset'))
   if(limit===100){roleOffsets.push(offset);return reply({items:Array.from({length:offset===0?100:1},(_,i)=>role(offset+i+1)),total_count:'101'})}
   return reply({items:[],total_count:'0'})
  }
  throw Error('Unexpected request '+path)
 })
 const componentPath='/@fs'+resolve('platform-web/src/AccessPanel.vue')
 const source=await(await page.request.get(origin+componentPath)).text()
 const i18nPath=source.match(/from "([^"\n]*\/i18n\.ts[^"\n]*)"/)![1]
 const props={brandId:scope==='brand'?id:'',locale,scope,accountId:id,grants:['admin.write.platform'],initialTab:'accounts'}
 await page.route('**/__platform-access-lifecycle',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app" class="management-console"></div><script type="module">import{createApp,reactive,h}from'/node_modules/.vite/deps/vue.js';import Toast from'/@fs${resolve('shared/src/OperationToastHost.vue')}';import Component from'${componentPath}';import{createPlatformI18n,platformI18nKey}from'${i18nPath}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('platform-web/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/management.css')}';const props=reactive(${JSON.stringify(props)});window.accessProps=props;const i18n=createPlatformI18n();i18n.setLocale('${locale}');createApp({render:()=>[h(Component,props),h(Toast,{locale:props.locale})]}).provide(platformI18nKey,i18n).mount('#app')</script></html>`}))
 await page.goto(origin+'/__platform-access-lifecycle')
 const createAccount=page.getByRole('button',{name:zh?'新建账号':'New account',exact:true})
 await expect(createAccount).toBeDisabled()
 await expect(page.locator('body')).toContainText(zh?'无操作权限':'You do not have permission')
 expect(accountReads).toBe(0)
 const setGrants=async(value:string[])=>page.evaluate(value=>{(window as any).accessProps.grants=value},value)
 await setGrants(grants)
 await expect(createAccount).toBeEnabled()
 expect(accountReads).toBe(1)
 await setGrants(grants.filter(p=>p!=='role.view.platform'))
 await expect(createAccount).toBeDisabled()
 await expect(page.getByTestId('platform-access-list')).not.toHaveAttribute('aria-busy','true')
 await page.locator('.member-menu-trigger').click()
 await page.getByRole('menuitem',{name:zh?'重置密码':'Reset password',exact:true}).click()
 const passwordDialog=page.getByRole('dialog')
 await passwordDialog.locator('input[type=password]').fill('catalog-test-password')
 await expect(passwordDialog.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeEnabled()
 expect(roleReads).toBe(0)
 await passwordDialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
 await page.getByRole('tab',{name:zh?'角色':'Roles',exact:true}).click()
 await expect(page.locator('body')).toContainText(zh?'无操作权限':'You do not have permission')
 expect(roleReads).toBe(0)
 await setGrants(grants)
 const createRole=page.getByRole('button',{name:zh?'新建角色':'New role',exact:true})
 await expect(createRole).toBeEnabled()
 await createRole.click()
 await expect.poll(()=>permissionReleases.length).toBe(1)
 let dialog=page.getByRole('dialog')
 await expect(dialog.getByRole('button',{name:zh?'保存':'Save',exact:true})).toBeDisabled()
 await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
 await createRole.click()
 await expect.poll(()=>permissionReleases.length).toBe(2)
 await permissionReleases[1]()
 await expect(dialog.locator('.permission-picker')).toContainText(zh?'角色':'Roles')
 await permissionReleases[0]()
 await expect(dialog.locator('.permission-picker')).not.toContainText(zh?'账号':'Accounts')
 await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
 await page.getByRole('tab',{name:zh?'账号':'Accounts',exact:true}).click()
 await expect(createAccount).toBeEnabled()
 await createAccount.click()
 await expect(dialog.getByRole('checkbox')).toHaveCount(101)
 expect(roleOffsets).toEqual([0,100])
 await expect(dialog).toContainText('Role 101')
 const viewport=page.viewportSize()!
 for(const width of info.project.name.endsWith('reflow')?[320,640,1440]:[viewport.width]) {
  if(info.project.name.endsWith('reflow')) await page.setViewportSize({width,height:viewport.height})
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`platform-access-${scope}-${locale}-${width}.png`)})
 }
})
