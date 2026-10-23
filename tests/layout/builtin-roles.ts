import { expect, test } from '@playwright/test'
import { origins, gameId, installLayoutApi, expectNoPageOverflow } from './fixture'

export function registerBuiltinRoleTests() {
 for(const surface of ['brand','platform-brand','platform'] as const)for(const locale of ['en','zh-CN'] as const){
  test(`built-in roles are viewable and read-only (${surface}, ${locale})`,async({page},info)=>{
   const zh=locale==='zh-CN',brandEntry=surface==='brand',platformRole=surface==='platform'
   const builtin='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',custom='cccccccc-cccc-4ccc-8ccc-000000000001'
   const permission=platformRole?'role.view.platform':'role.view.brand'
   const grants=brandEntry?['role.view.brand','role.write.brand','role.delete.brand']:['admin.view.platform','role.view.platform','role.write.platform','role.delete.platform']
   const roles=[{id:builtin,brand_id:platformRole?'':gameId,code:`bootstrap_${builtin}`,name:'Bootstrap administrator',status:'active',version:1,is_bootstrap:true,permissions:[permission,'user.kick.brand'].filter(value=>!platformRole||value.endsWith('.platform'))},{id:custom,brand_id:platformRole?'':gameId,code:'operations',name:'Operations',status:'active',version:1,is_bootstrap:false,permissions:[permission]}]
   const errors:string[]=[],writes:string[]=[]
   page.on('pageerror',cause=>errors.push(cause.message))
   await installLayoutApi(page,locale)
   await page.route('**/api/**',async route=>{
    const request=route.request(),path=new URL(request.url()).pathname
    if(request.method()!=='GET'){writes.push(path);throw Error(`Read-only role test attempted mutation: ${path}`)}
    const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{[`role:${builtin}`]:{name:'Bootstrap administrator',built_in:true},[`brand:${gameId}`]:{name:'Aurora'}}}})
    if(path==='/api/v1/admin/me')return reply({account:{id:gameId,username:'brand_admin',super_admin:false,brand_ids:[gameId],permissions:grants,permissions_by_brand:{[gameId]:grants}}})
    if(path==='/api/v1/platform/presentation')return reply({display_name:'Layout',logo_text:'L',logo_url:''})
    if(path==='/api/v1/platform/me')return reply({account:{id:gameId,username:'platform_admin',super_admin:true,platform_permissions:grants}})
    if(path==='/api/v1/platform/platform-settings')return reply({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',display_name:'Layout',logo_text:'L',logo_url:'',version:1,updated_at:'2026-10-18T00:00:00Z'})
    if(path==='/api/v1/admin/brands'||path==='/api/v1/platform/brands')return reply({items:[{id:gameId,code:'aurora',name:'Aurora',status:'active'}]})
    if(path==='/api/v1/admin/users')return reply({items:[],limit:100,offset:0,total_count:'0'})
    if(path==='/api/v1/platform/accounts'||path==='/api/v1/platform/platform-accounts')return reply({items:[]})
    if(path==='/api/v1/admin/workbench')return route.fulfill({status:403,json:{success:false,error:{code:'PERMISSION_DENIED',message:'No workbench grant'}}})
    if(path.endsWith('/roles')||path.endsWith('/platform-roles'))return reply({items:roles})
    if(path.endsWith('/permissions')||path.endsWith('/platform-permissions'))return reply({items:[permission]})
    return route.fallback()
   })
   await page.goto(brandEntry?origins.brand:origins.platform)
   await expect(page.locator('.management-console')).toBeVisible()
   await expect(page.locator('.account-popover > span')).toHaveCount(0)
   if (!await page.evaluate(() => matchMedia('(pointer: coarse)').matches)) {
    const viewport=page.viewportSize()!
    for(const width of [320,899,900,901,1080,1440]){
     await page.setViewportSize({width,height:viewport.height})
     await page.locator('.topbar .locale-picker__trigger').evaluate(async el=>{
      getComputedStyle(el).width
      await Promise.all(el.getAnimations().map(animation=>animation.finished))
     })
     await expectNoPageOverflow(page)
     await expect(page.locator('.topbar .locale-picker__trigger')).toHaveCSS('height',width>900?'36px':'44px')
    }
    await page.setViewportSize(viewport)
    await page.locator('.topbar .locale-picker__trigger').evaluate(async el=>{
     getComputedStyle(el).width
     await Promise.all(el.getAnimations().map(animation=>animation.finished))
    })
   }
   const label=zh?'系统管理员':'System administrator',note=zh?'系统内置角色，不可修改':'System built-in role. Changes are not allowed.'
   if(brandEntry){
    await expect(page.locator('.app-shell')).toBeVisible()
    const name=zh?'账号与权限':'Accounts and permissions'
    if(await page.locator('.mobile-nav').isVisible()){
     await page.locator('.mobile-nav button').last().click()
     await page.locator('.mobile-more-menu').getByRole('button',{name:new RegExp(name)}).click()
    }else {
     const navigation=page.locator('.side-nav').getByRole('button',{name:new RegExp(name)})
     await navigation.scrollIntoViewIfNeeded()
     await expect(navigation).toBeInViewport()
     await navigation.click()
    }
   }else{
    await expect(page.locator('.app-frame')).toBeVisible()
    const menu=page.locator('button[aria-controls="platform-navigation"]');if(await menu.isVisible())await menu.click()
    if (!platformRole && await page.locator('button[aria-controls="platform-brand-tabs"]').getAttribute('aria-expanded') === 'false') await page.locator('button[aria-controls="platform-brand-tabs"]').click()
    const name=platformRole?(zh?'管理账号':'Admin accounts'):(zh?'后台账号':'Back-office accounts')
    await page.getByRole('button',{name,exact:true}).filter({visible:true}).click()
    if(!platformRole)await page.locator('.topbar-brand-picker select').selectOption(gameId)
    await page.getByRole('tab',{name:zh?'角色':'Roles',exact:true}).click()
   }
   const list=brandEntry?page.locator('.access-page .listing'):page.getByTestId('platform-access-list')
   const row=list.locator(brandEntry?'article.record':'tbody tr').filter({has:page.getByRole('button',{name:label,exact:true})})
   await expect(row.getByRole('button')).toHaveCount(1)
   await row.getByRole('button',{name:label,exact:true}).click()
   const dialog=page.getByRole('dialog',{name:zh?'角色详情':'Role details',exact:true})
   await expect(list).not.toContainText(note)
   await expect(dialog).not.toContainText(note);await expect(dialog).toContainText(label)
   await expect(dialog).not.toContainText(zh?'引导角色':'Bootstrap role')
   await expect(dialog.locator('input,select,textarea')).toHaveCount(0)
   await expect(dialog.getByRole('button')).toHaveCount(1)
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`builtin-role-${surface}-${locale}.png`)})
   await dialog.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
   // Custom roles remain editable; read-only is not applied to every role.
   if(brandEntry){
    const ordinary=list.locator('article.record').filter({has:page.getByRole('button',{name:'Operations',exact:true})})
    await ordinary.getByRole('button',{name:zh?'编辑':'Edit',exact:true}).click()
    await expect(page.locator('.access-page .form input').filter({visible:true}).first()).toHaveValue('Operations')
   }else{
    await list.getByRole('button',{name:'Operations',exact:true}).click()
    await expect(page.getByRole('dialog').getByRole('button',{name:zh?'修改角色':'Edit role',exact:true})).toBeVisible()
   }
   expect(writes).toEqual([]);expect(errors).toEqual([])
  })
 }
}
