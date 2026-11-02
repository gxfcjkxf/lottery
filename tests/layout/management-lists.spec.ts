import {registerMemberBatchDeleteTests} from './member-batch-delete'
import {test,expect,type Page,type TestInfo} from '@playwright/test'
import {resolve} from 'node:path'
import {readFileSync} from 'node:fs'
import {selectControl} from '../select-control'
import {gameId,origins,installLayoutApi,openPlatformBrandList,expectNoPageOverflow} from './fixture'
const at='2026-11-02T00:00:00Z'
const id=(n:number)=>`bbbbbbbb-bbbb-4bbb-8bbb-${String(n).padStart(12,'0')}`
async function openHarness(page:Page,kind:string,locale:string,readOnly=false){
 await page.route('**/__management-list',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import C from'/@fs${resolve('tests/layout/ManagementListsHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('admin-web/src/style.css')}';import'/@fs${resolve('platform-web/src/style.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/refresh-button.css')}';import'/@fs${resolve('shared/src/empty-state.css')}';import'/@fs${resolve('shared/src/key-value.css')}';createApp(C,${JSON.stringify({kind,locale,id:gameId,readOnly})}).mount('#app')</script></html>`}))
 await page.goto(origins.platform+'/__management-list')
}
async function review(page:Page,info:TestInfo,label:string){
 const preset=page.viewportSize()!
 for(const width of info.project.name.includes('reflow')?[320,640,900,1080,1440]:[preset.width]){
  if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
  await expectNoPageOverflow(page)
  const list=page.locator('.management-list').first();await expect(list).toBeVisible()
  await expect(list).toHaveCSS('padding','0px')
  const filters=list.locator('.management-list-filters'),actions=list.locator('.management-list-toolbar > .console-page-actions')
  if(await filters.count() && await actions.count()){
   const geometry=await list.locator('.management-list-toolbar').evaluate(el=>{
    const box=el.getBoundingClientRect(),style=getComputedStyle(el)
    const action=el.querySelector(':scope > .console-page-actions')!.getBoundingClientRect()
    const query=el.querySelector('.management-list-filters button[type=submit]')?.getBoundingClientRect()
    return {right:box.right-parseFloat(style.paddingRight),actionRight:action.right,actionBottom:action.bottom,queryBottom:query?.bottom}
   })
   expect(Math.abs(geometry.right-geometry.actionRight)).toBeLessThanOrEqual(2)
   if(width>=900 && geometry.queryBottom!==undefined)expect(Math.abs(geometry.actionBottom-geometry.queryBottom)).toBeLessThanOrEqual(2)
  }
  await expect(list.locator('.management-list')).toHaveCount(0)
  const jump=list.locator('.table-pagination-jump label')
  if(await jump.isVisible())await expect(jump).toHaveCSS('flex-direction','row')
  await page.screenshot({path:info.outputPath(`${label}-${width}.png`),fullPage:true})
 }
}
for(const locale of ['en','zh-CN'] as const){
 test(`brand directory shares full pagination, query and batch selection (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN';await installLayoutApi(page,locale)
  const brands=Array.from({length:51},(_,i)=>({id:id(i+1),name:`brand-${i+1}`,code:`b${i+1}`,status:'active',member_count:'0',agent_count:'0',recharge_quota_available:null,created_at:at}))
  await page.route('**/api/v1/platform/**',async route=>{
   expect(route.request().method()).toBe('GET');const path=new URL(route.request().url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'list_reviewer',super_admin:true,platform_permissions:['brand.view.platform','brand.delete.platform']}})
   if(path.endsWith('/brands'))return reply({items:brands})
   if(path.endsWith('/platform-settings'))return reply({display_name:'Layout',logo_text:'L',logo_url:'',timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',version:1,updated_at:at})
   return route.fallback()
  })
  await page.goto(origins.platform);await openPlatformBrandList(page,locale)
  const list=page.getByTestId('platform-brand-list'),nav=list.locator('.table-pagination'),all=list.getByRole('checkbox',{name:zh?'全选':'Select all',exact:true})
  await expect(list.locator('.management-list-toolbar')).toHaveCount(1)
  await expect(list.locator('tbody tr')).toHaveCount(50);await all.check();await expect(list.locator('.member-batch-toolbar [role=status]')).toContainText('50')
  await nav.getByRole('button',{name:zh?'下一页':'Next page',exact:true}).click();await expect(list.locator('tbody tr')).toHaveCount(1);await expect(all).not.toBeChecked()
  await list.locator('input[type=search]').fill('brand-51');await list.locator('button[type=submit]').click();await expect(list.locator('tbody tr')).toHaveCount(1)
  await expect(nav.locator('[aria-current=page]')).toHaveText('1');await expect(nav.getByRole('button',{name:zh?'下一页':'Next page',exact:true})).toBeDisabled()
  await review(page,info,'brands')
  await list.locator('input[type=search]').fill('missing');await list.locator('button[type=submit]').click();await expect(list.locator('.empty-state')).toBeVisible();await expect(nav).toContainText(zh?'共 0 条':'Total: 0')
 })
 test(`platform access excludes protected rows and retries frozen deletions (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',attempts:{key:string;body:unknown}[]=[]
  const accounts=[{id:gameId,username:'review_operator',status:'active',version:1,super_admin:true,brand_ids:[],role_ids:[],role_codes:[]},{id:id(1),username:'operator_one',status:'active',version:7,super_admin:true,brand_ids:[],role_ids:[],role_codes:[]}]
  const roles=[{id:id(2),brand_id:'',code:'builtin',name:'Built in',status:'active',version:1,is_bootstrap:true,permissions:['role.view.platform']},{id:id(3),brand_id:'',code:'custom',name:'Custom',status:'active',version:2,is_bootstrap:false,permissions:['role.view.platform']}]
  await page.route('**/api/**',async route=>{
   const req=route.request(),url=new URL(req.url()),reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(req.method()!=='GET'){
    expect(url.pathname).toContain(id(1));attempts.push({key:req.headers()['idempotency-key']!,body:req.postDataJSON()})
    if(attempts.length===1)return route.abort('failed')
    accounts.splice(1,1);return reply({audit_log_id:id(9)})
   }
   if((url.pathname.endsWith('/accounts') || url.pathname.endsWith('/platform-accounts')))return reply({items:accounts,total_count:String(accounts.length)})
   if((url.pathname.endsWith('/roles') || url.pathname.endsWith('/platform-roles')))return reply({items:roles,total_count:'2'})
   throw Error('Unexpected read '+url.pathname)
  })
  await openHarness(page,'platform-access',locale)
  const list=page.locator('.management-list'),toolbar=list.locator('.member-batch-toolbar')
  await expect(list.locator('tbody tr')).toHaveCount(2);await expect(list.locator('tbody input[type=checkbox]')).toHaveCount(1)
  await toolbar.getByRole('checkbox').check();await expect(toolbar.getByRole('status')).toHaveText(zh?'已选 1 个':'1 selected')
  await review(page,info,'access')
  await toolbar.getByRole('button').click();const dialog=page.getByRole('dialog')
  await dialog.getByRole('button',{name:zh?'删除 1 个账号':'Delete 1 accounts',exact:true}).click()
  await expect(dialog.locator('.detail-modal-close')).toBeDisabled();await dialog.getByRole('button',{name:zh?'重试未完成项':'Retry unfinished',exact:true}).click()
  await expect(dialog).toContainText(zh?'已删除 1 个':'1 deleted');expect(attempts).toHaveLength(2);expect(attempts[1]).toEqual(attempts[0]);expect(attempts[0].body).toEqual({version:7})
  await dialog.locator('.detail-modal-close').click();await expect(list.locator('tbody tr')).toHaveCount(1)
  await list.getByRole('tab',{name:zh?'角色':'Roles',exact:true}).click();await expect(list.locator('tbody tr')).toHaveCount(2);await expect(list.locator('tbody input[type=checkbox]')).toHaveCount(1)
  await expect(list.locator('[role=tablist] button')).toHaveCount(2)
 })
 for(const kind of ['brand-access','brand-roles','agent-members','agent-children','agent-commission'])test(`${kind} uses the common list structure (${locale})`,async({page},info)=>{
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',async route=>{
   const req=route.request(),url=new URL(req.url());expect(req.method()).toBe('GET')
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{[`role:${id(5)}`]:{name:'Operations'}}}})
   const role={id:id(5),brand_id:gameId,code:'ops',name:'Operations',status:'active',version:1,is_bootstrap:false,permissions:['admin.view.brand']}
   if((url.pathname.endsWith('/roles') || url.pathname.endsWith('/platform-roles')))return reply({items:[role],total_count:'1'})
   if(url.pathname.endsWith('/permissions'))return reply({items:['admin.view.brand','admin.write.brand','role.view.brand','role.write.brand']})
   if((url.pathname.endsWith('/accounts') || url.pathname.endsWith('/platform-accounts')))return reply({items:[{id:id(1),username:'operator_one',status:'active',version:1,super_admin:false,brand_ids:[gameId],role_ids:[role.id],role_codes:['ops']}],total_count:'1'})
   if(url.pathname.endsWith('/members'))return reply({items:[{id:id(1),username:'member_one',phone:'',display_name:'Member',status:'normal',joined_at:at,available_points:'100',agent_username:'agent_one',is_direct:true}],total_count:'1',limit:20,offset:0,summary:{member_count:'1',available_points:'100',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}})
   if(url.pathname.endsWith('/children'))return reply({items:[],total_count:'0',limit:20,offset:0})
   if(url.pathname.endsWith('/commission'))return reply({items:[{id:id(1),created_at:at,entry_type:'commission_credit',points:'10'}],total_count:'1',limit:20,offset:0,credited_points:'10',debited_points:'0'})
   if(url.pathname.endsWith('/wallet'))return reply({available_points:'100',commission_points:'10'})
   throw Error('Unexpected read '+url.pathname)
  })
  await openHarness(page,kind,locale)
  if(kind==='agent-children')await expect(page.locator('.management-list .table-pagination')).toHaveCount(0)
  else await expect(page.locator('.management-list .table-pagination')).toBeVisible()
  if(kind!=='agent-children')await expect(page.locator('.management-list tbody tr:has(td:not(.empty-state))')).toHaveCount(1)
  await expect(page.locator('.member-batch-toolbar')).toHaveCount(0)
  await review(page,info,kind);expect(errors).toEqual([])
 })
}

registerMemberBatchDeleteTests(false)

for(const locale of ['en','zh-CN'] as const){
 for(const kind of ['platform-access','brand-access','brand-roles'])test(`read-only list hides write and batch actions ${kind} (${locale})`,async({page},info)=>{
  await page.route('**/api/**',route=>{
   expect(route.request().method()).toBe('GET')
   const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/permissions'))return reply({items:['admin.view.brand','role.view.brand']})
   if(path.endsWith('/roles')||path.endsWith('/platform-roles'))return reply({items:[{id:id(2),brand_id:kind==='platform-access'?'':gameId,code:'reader',name:'Reader',status:'active',version:1,is_bootstrap:false,permissions:[]}],total_count:'1'})
   if(path.endsWith('/accounts')||path.endsWith('/platform-accounts'))return reply({items:[{id:id(1),username:'reader',status:'active',version:1,super_admin:kind==='platform-access',brand_ids:kind==='platform-access'?[]:[gameId],role_ids:[],role_codes:[]}],total_count:'1'})
   throw Error('Unexpected read '+path)
  })
  await openHarness(page,kind,locale,true)
  const list=page.locator('.management-list');await expect(list.locator('tbody tr')).toHaveCount(1)
  await expect(list.locator('input[type=checkbox],.member-batch-toolbar')).toHaveCount(0)
  await expect(list.locator('.management-list-toolbar > .console-page-actions .button-primary:not([role=tab])')).toHaveCount(0)
  await list.locator('[aria-haspopup=menu]').click()
  await expect(page.getByRole('menuitem')).toHaveCount(1)
  await page.keyboard.press('Escape');await review(page,info,'readonly-'+kind)
 })
 test(`agent member reads commit query, page and scope only on success (${locale})`,async({page},info)=>{
  page.setDefaultTimeout(8000)
  const zh=locale==='zh-CN',reads:URL[]=[];let fail=false
  const members=Array.from({length:41},(_,i)=>({id:id(i+1),username:`member_${i}`,phone:'',display_name:'',status:'normal',joined_at:at,available_points:'10',agent_username:'agent',is_direct:true}))
  await page.route('**/api/**',route=>{
   expect(route.request().method()).toBe('GET');const url=new URL(route.request().url());reads.push(url)
   if(fail){fail=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic read failure'}}})}
   const q=url.searchParams.get('q'),items=members.filter(m=>!q||m.username===q),offset=Number(url.searchParams.get('offset'))
   return route.fulfill({json:{success:true,data:{items:items.slice(offset,offset+20),total_count:String(items.length),limit:20,offset,summary:{member_count:String(items.length),available_points:'10',bet_count:'0',stake_points:'0',unsettled_points:'0',net_points:'0'}}}})
  })
  await openHarness(page,'agent-members',locale)
  const list=page.locator('.management-list'),pager=list.locator('.table-pagination'),next=pager.getByRole('button',{name:zh?'下一页':'Next',exact:true}),rows=list.locator('tbody tr'),search=list.locator('input:not([type=checkbox])').first(),query=list.locator('button[type=submit]')
  await expect(rows).toHaveCount(20)
  fail=true;await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(rows.first()).toContainText('member_0')
  await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('2')
  await search.fill('member_40');fail=true;await query.click();await expect(rows.first()).toContainText('member_20');await expect(pager.locator('[aria-current=page]')).toHaveText('2')
  await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('3');expect(reads.at(-1)!.searchParams.get('q')).toBe('')
  await query.click();await expect(rows).toHaveCount(1);await expect(pager.locator('[aria-current=page]')).toHaveText('1')
  fail=true;await list.getByRole('button',{name:zh?'清空':'Clear',exact:true}).click();await expect(rows).toHaveCount(1);await expect(list.getByRole('button',{name:zh?'清空':'Clear',exact:true})).toBeVisible()
  await list.locator('.button[data-refresh-button]').click();await expect(rows).toHaveCount(20);expect(reads.at(-1)!.searchParams.get('q')).toBe('')
  const scope=list.getByRole('button',{name:zh?'全代理线':'Full agent line',exact:true});fail=true;await scope.click();await expect(scope).toHaveAttribute('aria-pressed','false');await expect(rows).toHaveCount(20)
  await list.locator('.button[data-refresh-button]').click();await expect(scope).toHaveAttribute('aria-pressed','true');expect(reads.at(-1)!.searchParams.get('scope')).toBe('line')
  await search.fill('missing');await query.click();await expect(list.locator('td.empty-state')).toBeVisible();await expect(pager).toHaveCount(0);await review(page,info,'agent-empty')
 })
}

for(const locale of ['en','zh-CN'] as const)for(const readOnly of [false,true])test(`catalogue filters use complete data and retain applied pages (${locale}, ${readOnly?'reader':'writer'})`,async({page},info)=>{
 page.setDefaultTimeout(8000)
 const zh=locale==='zh-CN',model=JSON.parse(readFileSync('backend/internal/rules/config/examples/lucky_group_preview.json','utf8')).catalog.model,reads:number[]=[]
 const games=Array.from({length:55},(_,i)=>({id:id(i+1),name:`Game ${i+1}`,code:`game_${i+1}`,model,timezone:'UTC',version:1,connected:true,status:i<50?'active':'offline',deletable:true,brand_enabled:true,brand_version:1,rule_published:true,sales_status:'unconfigured'}))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET');const url=new URL(route.request().url());expect(url.pathname).toBe('/api/v1/platform/platform-games')
  const offset=Number(url.searchParams.get('offset'));reads.push(offset)
  return route.fulfill({json:{success:true,data:{games:games.slice(offset,offset+50),limit:50,offset,total_count:'55'}}})
 })
 await openHarness(page,'platform-catalog',locale,readOnly)
 const list=page.locator('.management-list'),pager=list.locator('.table-pagination'),rows=list.locator('tbody tr'),filter=list.getByRole('combobox'),query=list.getByRole('button',{name:zh?'查询':'Search',exact:true})
 await expect(rows).toHaveCount(50);expect(reads).toEqual([0,50])
 await expect(pager).toContainText(zh?'共 50 条':'Total: 50')
 await expect(list.locator('.management-list-toolbar > .console-page-actions .button-primary')).toHaveCount(readOnly?0:1)
 await expect(list.locator('[data-refresh-button],input[type=checkbox]')).toHaveCount(0)
 await selectControl(filter,'offline');await expect(rows).toHaveCount(50)
 await query.click();await expect(rows).toHaveCount(5);await expect(pager).toContainText(zh?'共 5 条':'Total: 5')
 await selectControl(filter,'all');await query.click();await expect(rows).toHaveCount(50)
 await pager.getByRole('button',{name:zh?'下一页':'Next page',exact:true}).click();await expect(rows).toHaveCount(5)
 await selectControl(filter,'active');await pager.getByRole('button',{name:zh?'上一页':'Previous page',exact:true}).click();await expect(rows).toHaveCount(50);await expect(pager).toContainText(zh?'共 55 条':'Total: 55')
 await list.getByRole('button',{name:zh?'清空':'Clear',exact:true}).click();await expect(pager).toContainText(zh?'共 50 条':'Total: 50')
 if(readOnly){await expect(list.locator('[aria-haspopup=menu]')).toHaveCount(0);await expect(list.getByRole('columnheader',{name:zh?'操作':'Actions',exact:true})).toHaveCount(0)}
 await review(page,info,'catalogue-'+readOnly)
})

for(const locale of ['en','zh-CN'] as const)test(`brand agent directory retains applied filters and page on failed reads (${locale})`,async({page},info)=>{
 page.setDefaultTimeout(8000)
 const zh=locale==='zh-CN',reads:URL[]=[];let fail=false
 const nodes=Array.from({length:41},(_,i)=>({id:id(i+1),brand_id:gameId,agent_account_id:id(i+1),parent_id:null,depth:1,path:[id(i+1)],version:1,config:{ratio:'0.08',mode:null,status:'active',can_create_children:true},effective_mode:'loss',mode_source_agent_id:null,policy_version:1,parent_version:null,created_by:gameId,created_at:at,updated_at:at,username:`agent_${i}`,online:false,display_points:null}))
 await page.route('**/api/**',route=>{
  expect(route.request().method()).toBe('GET');const url=new URL(route.request().url()),reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(url.pathname.endsWith('/agent-policy'))return reply({brand_id:gameId,version:1,config:{enabled:true,max_depth:5,loss_ratio_cap:'0.25',turnover_ratio_cap:'0.25',mode:'loss',cycle:'monthly'},updated_at:at})
  if(url.pathname===`/api/v1/admin/agents/${id(1)}`)return reply(nodes[0])
  expect(['/api/v1/admin/agents','/api/v1/admin/agents/tree']).toContain(url.pathname);reads.push(url)
  if(fail){fail=false;return route.fulfill({status:503,json:{success:false,error:{code:'SERVICE_UNAVAILABLE',message:'Synthetic failure'}}})}
  const q=url.searchParams.get('q'),items=nodes.filter(n=>!q||n.username===q),offset=Number(url.searchParams.get('offset'))
  if(url.pathname.endsWith('/tree')){const parent=url.searchParams.get('parent_id');return reply({brand_id:gameId,parent_id:parent,items:parent?[]:nodes.slice(offset,offset+20),limit:20,offset,total_count:parent?'0':'41'})}
  return reply({brand_id:gameId,items:items.slice(offset,offset+20),limit:20,offset,total_count:String(items.length)})
 })
 await openHarness(page,'brand-agents',locale,true)
 const list=page.locator('.management-list'),pager=list.locator('.table-pagination'),rows=list.locator('tbody tr'),next=pager.getByRole('button',{name:zh?'下一页':'Next',exact:true}),search=list.locator('.member-filters input').first(),query=list.locator('button[type=submit]')
 await expect(rows).toHaveCount(20);await expect(list.locator('.member-batch-toolbar,input[type=checkbox],.console-page-actions .button')).toHaveCount(0)
 fail=true;await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(rows.first()).toContainText('agent_0')
 await next.click();await expect(pager.locator('[aria-current=page]')).toHaveText('2')
 await search.fill('agent_40');await next.click();await expect(rows).toHaveCount(1);expect(reads.at(-1)!.searchParams.get('q')).toBe(null)
 fail=true;await query.click();await expect(pager.locator('[aria-current=page]')).toHaveText('3')
 await query.click();await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(rows).toHaveCount(1)
 await list.getByRole('button',{name:zh?'清空':'Clear',exact:true}).click();await expect(rows).toHaveCount(20)
 await search.fill('missing');await query.click();await expect(list.locator('td.empty-state')).toBeVisible();await expect(pager).toContainText(zh?'共 0 条':'Total: 0')
 await review(page,info,'brand-agents-empty')
 const hierarchy=list.getByRole('button',{name:zh?'按上下级查看':'Browse hierarchy',exact:true}),all=list.getByRole('button',{name:zh?'全部代理':'All agents',exact:true})
 fail=true;await hierarchy.click();await expect(all).toHaveAttribute('aria-pressed','true');await expect(pager).toContainText(zh?'共 0 条':'Total: 0')
 await hierarchy.click();await expect(hierarchy).toHaveAttribute('aria-pressed','true');await expect(list.locator('.am-node')).toHaveCount(20);await expectNoPageOverflow(page)
 const children=list.getByRole('button',{name:zh?'下级':'Children',exact:true}).first()
 fail=true;await children.click();await expect(list.locator('.am-node')).toHaveCount(20);await expect(list.locator('.management-list-summary')).toHaveCount(0)
 await children.click();await expect(list.locator('.am-node')).toHaveCount(0);await expect(list.locator('.management-list-summary')).toBeVisible();expect(reads.at(-1)!.searchParams.get('parent_id')).toBe(id(1))
 await review(page,info,'brand-agent-hierarchy')
})

for(const locale of ['en','zh-CN'] as const)test(`domain directory paginates a complete snapshot and shares row actions (${locale})`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[];page.on('pageerror',error=>errors.push(error.message))
 let reads=0
 await page.route('**/api/**',async route=>{
  expect(route.request().method()).toBe('GET');expect(new URL(route.request().url()).pathname).toBe('/api/v1/platform/brand-domains');reads++
  await route.fulfill({json:{success:true,data:{brand_id:gameId,status:'active',version:1,updated_at:at,domains:Array.from({length:51},(_,i)=>({id:id(i+1),domain:`entry-${i+1}.example.com`,enabled:true}))}}})
 })
 await openHarness(page,'domains',locale)
 const list=page.locator('.brand-domains'),nav=list.locator('.table-pagination')
 await expect(list.locator('tbody tr')).toHaveCount(50)
 await nav.getByRole('button',{name:zh?'下一页':'Next page',exact:true}).click()
 await expect(list.locator('tbody tr')).toHaveCount(1);await expect(list.locator('tbody')).toContainText('entry-51.example.com');expect(reads).toBe(1)
 await list.locator('.member-menu-trigger').click();await expect(page.getByRole('menuitem')).toHaveText(zh?['编辑','删除']:['Edit','Delete']);await page.keyboard.press('Escape')
 await review(page,info,'domains');expect(errors).toEqual([])
})

for(const locale of ['en','zh-CN'] as const)test(`brand member search and create share the toolbar (${locale})`,async({page},info)=>{
 const zh=locale==='zh-CN'
 await installLayoutApi(page,locale)
 await page.route('**/api/v1/admin/**',route=>{
  expect(route.request().method()).toBe('GET')
  const path=new URL(route.request().url()).pathname,reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'toolbar_reviewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['user.view.brand','user.create.brand']}}})
  if(path.endsWith('/brands'))return reply({items:[{id:gameId,code:'layout',name:'Aurora',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:at}],total_count:'1'})
  if(path.endsWith('/users'))return reply({items:[],total_count:'0',limit:100,offset:0})
  throw Error('Unexpected toolbar read '+path)
 })
 await page.goto(origins.brand)
 await expect(page.getByRole('button',{name:'toolbar_reviewer',exact:true})).toBeVisible()
 const mobile=page.locator('.mobile-nav')
 if(await mobile.isVisible())await mobile.locator('button').nth(1).click()
 else await page.locator('.side-nav').getByRole('button',{name:zh?'会员管理':'Member management',exact:true}).click()
 const list=page.locator('.directory-page .management-list')
 await expect(list.locator('.search-field')).toBeVisible()
 await review(page,info,'brand-member-toolbar')
 await list.locator('.console-page-actions button').click()
 await expect(page.getByRole('dialog')).toBeVisible()
 await page.getByRole('dialog').locator('.detail-modal-close').click()
 await expect(list.locator('.search-field')).toBeVisible()
})

for(const locale of ['en','zh-CN'] as const)test(`platform access returns to the populated page after deleting the last page (${locale})`,async({page})=>{
 const zh=locale==='zh-CN',accounts=Array.from({length:51},(_,i)=>({id:id(i+1),username:`operator_${i+1}`,status:'active',version:3,super_admin:true,brand_ids:[],role_ids:[],role_codes:[]}))
 await page.route('**/api/**',route=>{
  const req=route.request(),url=new URL(req.url()),reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(req.method()==='POST'){
   expect(url.pathname).toBe(`/api/v1/platform/platform-accounts/${id(51)}/delete`);expect(req.postDataJSON()).toEqual({version:3});expect(req.headers()['idempotency-key']).toBeTruthy()
   accounts.splice(50,1);return reply({audit_log_id:id(99)})
  }
  expect(req.method()).toBe('GET');expect(url.pathname).toMatch(/\/(platform-)?accounts$/)
  const offset=Number(url.searchParams.get('offset')),limit=Number(url.searchParams.get('limit'))
  return reply({items:accounts.slice(offset,offset+limit),total_count:String(accounts.length)})
 })
 await openHarness(page,'platform-access',locale)
 const list=page.locator('.management-list'),pager=list.locator('.table-pagination'),batch=list.locator('.member-batch-toolbar')
 await expect(list.locator('tbody tr')).toHaveCount(50)
 await pager.locator('.table-pagination-pages > button').last().click();await expect(list.locator('tbody tr')).toHaveCount(1)
 await batch.getByRole('checkbox').check();await batch.getByRole('button').click()
 const dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:zh?'删除 1 个账号':'Delete 1 accounts',exact:true}).click()
 await expect(dialog).toContainText(zh?'已删除 1 个':'1 deleted');await dialog.locator('.detail-modal-close').click()
 await expect(pager.locator('[aria-current=page]')).toHaveText('1');await expect(list.locator('tbody tr')).toHaveCount(50);await expect(batch.getByRole('checkbox')).not.toBeChecked()
})
