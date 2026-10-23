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
    if(path.endsWith('roles'))return ok({items:roles})
    if(path.endsWith('accounts'))return ok({items:[]})
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
  const props=admin?{brandId:id,account:{id,username:'operator',status:'active',super_admin:false,brand_ids:[id],permissions,permissions_by_brand:{[id]:permissions}}}:{brandId:scope==='brand'?id:'',locale,scope,accountId:id,grants:['admin.write.platform','role.write.platform'],initialTab:'roles'}
  const app=admin?'admin-web':'platform-web',factory=admin?'createAdminI18n':'createPlatformI18n',key=admin?'adminI18nKey':'platformI18nKey'
  await page.route('**/__role-permissions',r=>r.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Component from'/@fs${resolve(app+'/src/'+(admin?'AccessManagement.vue':'AccessPanel.vue'))}';import{${factory},${key}}from'/@fs${resolve(app+'/src/i18n.ts')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve(app+'/src/style.css')}';import'/@fs${resolve('shared/src/controls.css')}';const i18n=${factory}();i18n.setLocale('${locale}');createApp(Component,${JSON.stringify(props)}).provide(${key},i18n).mount('#app')</script></html>`}))
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
  if(!admin){
   const labels=page.locator('.role-identity > label');await expect(labels).toHaveCount(3)
   const boxes=await labels.evaluateAll(els=>els.map(e=>({top:e.getBoundingClientRect().top,text:e.textContent})))
   expect(boxes[0].text).toContain(zh?'名称':'Name');expect(boxes[1].text).toContain(zh?'代码':'Code');expect(boxes[2].text).toContain(zh?'状态':'Status')
   if(page.viewportSize()!.width>620)expect(Math.max(...boxes.map(b=>b.top))-Math.min(...boxes.map(b=>b.top))).toBeLessThan(2)
  }
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true)
  await page.screenshot({path:info.outputPath(`grouped-${surface}-${locale}.png`)})
  await form.getByRole('button',{name:admin?(zh?'创建角色':'Create role'):(zh?'保存':'Save'),exact:true}).click()
  await expect.poll(()=>writes.length).toBe(1)
  expect(writes[0].permissions.slice().sort()).toEqual(permissions.slice(0,4).sort())
  expect(errors).toEqual([])
 })
}
