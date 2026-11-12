import {selectControl} from '../select-control'
import {test,expect,type Page} from '@playwright/test'
import {installLayoutApi,installOrdersLayoutApi,origins,gameId,expectNoPageOverflow} from './fixture'
import { notificationCatalog } from '../../shared/src/notification-catalog'
import { newAnnouncement, type Announcement } from '../../shared/src/announcements'
const id='bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb'
const item=(title='Platform notice',brand:string|null=null):Announcement=>({id,brand_id:brand,version:1,generation:1,status:'published',state:'active',published_at:'2026-11-11T00:00:00Z',updated_at:'2026-11-11T00:00:00Z',eligible:true,config:{...newAnnouncement().config,content:{en:{title,html:'<h2>Service update</h2><p>English <strong>announcement</strong></p><ul><li>First item</li><li>Second item</li></ul>',button:'View announcements',url:'/announcements'},'zh-CN':{title:'平台公告',html:'<h2>服务更新</h2><p>中文<strong>公告</strong></p><ul><li>第一项</li><li>第二项</li></ul>',button:'查看公告',url:'/announcements'}}}})
async function cancelImagePicker(page:Page,locale:'en'|'zh-CN') {
 const zh=locale==='zh-CN'
 await page.getByTestId('announcements-management').getByRole('button',{name:zh?'新增公告':'New announcement',exact:true}).click()
 const draft=page.getByRole('dialog',{name:zh?'新增公告':'New announcement',exact:true})
 await draft.getByLabel(zh?'标题':'Title',{exact:true}).fill('Unsaved announcement')
 await draft.locator('textarea').fill('<p>Keep this draft</p>')
 const chooserPromise=page.waitForEvent('filechooser')
 await draft.getByRole('button',{name:zh?'上传图片':'Upload image',exact:true}).click()
 const chooser=await chooserPromise
 await chooser.setFiles([])
 await draft.locator('input[type=file]').dispatchEvent('cancel',{bubbles:true})
 await expect(draft).toBeVisible()
 await expect(draft.getByLabel(zh?'标题':'Title',{exact:true})).toHaveValue('Unsaved announcement')
 await expect(draft.locator('textarea')).toHaveValue('<p>Keep this draft</p>')
 // Escape on the announcement itself still closes it normally.
 await page.keyboard.press('Escape')
 await expect(draft).toHaveCount(0)
}
for(const locale of ['en','zh-CN'] as const){
 const zh=locale==='zh-CN'
 test(`guest popup and group dismissal ${locale}`,async({page},info)=>{
  await installLayoutApi(page,locale)
  const first=item(),second={...item('Brand notice',gameId),id:'cccccccc-cccc-4ccc-8ccc-cccccccccccc',config:{...item().config,content:{en:{title:'Brand notice',html:'<p>Brand content</p>',button:'',url:''},'zh-CN':{title:'品牌公告',html:'<p>品牌内容</p>',button:'',url:''}}}}
  const receipts:{action:string;items:{id:string}[]}[]=[]
  await page.route('**/api/v1/announcements',r=>r.fulfill({json:{success:true,data:{items:[first,second]}}}))
  await page.route('**/api/v1/announcements/receipts',r=>{receipts.push(r.request().postDataJSON());return r.fulfill({json:{success:true,data:{recorded:true}}})})
  await page.goto(`${origins.user}/login`)
  const modal=page.getByRole('dialog',{name:zh?'平台公告':'Platform notice',exact:true})
  await expect(modal).toBeVisible();await expect(modal.getByText('1 / 2')).toBeVisible()
  await expect.poll(()=>receipts.filter(r=>r.action==='view').length).toBe(1)
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`guest-${locale}.png`),fullPage:true})
  await modal.getByRole('button',{name:zh?'下一条':'Next',exact:true}).click()
  const next=page.getByRole('dialog',{name:zh?'品牌公告':'Brand notice',exact:true});await expect(next).toBeVisible()
  await next.getByRole('checkbox').check();await next.getByRole('button',{name:zh?'完成':'Done',exact:true}).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(receipts.filter(r=>r.action==='view').map(r=>r.items[0].id)).toEqual([first.id,second.id])
  expect(receipts.find(r=>r.action==='mute')?.items.map(v=>v.id)).toEqual([first.id,second.id])
  await page.goto(`${origins.user}/announcements`)
  await expect(page.locator('.announcement-row')).toHaveCount(2)
  await expect(page.getByRole('heading', { name: zh ? '通知' : 'Notifications', exact: true })).toBeVisible()
  await expect(page.getByRole('tab', { name: zh ? '公告' : 'Announcements', exact: true })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('tab', { name: zh ? '消息' : 'Messages', exact: true })).toBeVisible()
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`updates-${locale}.png`),fullPage:true})
  await page.locator('.announcement-row').first().click();await expect(page.getByRole('dialog')).toBeVisible()
  if(info.project.name.includes('reflow')){for(const width of [320,640,700,1000,1440]){await page.setViewportSize({width,height:600});await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`directory-${locale}-${width}.png`),fullPage:true})}}
 })
 test(`brand editor publish and frozen retry ${locale}`,async({page},info)=>{
  await installLayoutApi(page,locale)
  let current=item('Brand announcement',gameId),lost=true
  current.config.starts_at='2026-11-11T16:00:00Z'
  await page.route('**/api/v1/display-settings',route=>route.fulfill({json:{success:true,data:{theme:'jade',timezone:'Pacific/Kiritimati',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss'}}}))
  const writes:{body:string|null;key:string|undefined}[]=[]
  await page.route('**/api/v1/admin/**',async route=>{const req=route.request(),path=new URL(req.url()).pathname;const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'announcement_reviewer',super_admin:false,brand_ids:[gameId],permissions:[],permissions_by_brand:{[gameId]:['announcement.view.brand','announcement.write.brand','announcement.publish.brand']},platform_permissions:[]}})
   if(path.endsWith('/brands'))return reply({items:[{id:gameId,name:'Aurora',code:'aurora',status:'active',member_count:'0',agent_count:'0',recharge_quota_available:'0',created_at:'2026-10-24T00:00:00Z'}],total_count:'1'})
   if(path==='/api/v1/admin/announcements'&&req.method()==='GET')return reply({items:[current]})
   if(path==='/api/v1/admin/announcements/'+id){writes.push({body:req.postData(),key:req.headers()['idempotency-key']});const body=req.postDataJSON();current={...current,version:2,config:body.config};if(lost){lost=false;return route.abort('failed')};return reply(current)}
   return route.fallback()
  })
  await page.goto(origins.brand)
  await expect(page.locator('.mobile-nav:visible, .side-nav:visible').first()).toBeVisible()
  if(await page.locator('.mobile-nav').isVisible()){await page.locator('.mobile-nav button').last().click();await page.locator('.mobile-more-menu').getByRole('button',{name:zh?'品牌设置':'Brand settings',exact:true}).click()}else await page.locator('.side-nav').getByRole('button',{name:zh?'品牌设置':'Brand settings',exact:true}).click()
  await page.locator('.console-tabs').getByRole('button',{name:zh?'公告':'Announcements',exact:true}).click()
  const management=page.getByTestId('announcements-management');await expect(management).toBeVisible();await cancelImagePicker(page,locale)
  await management.getByRole('button',{name:zh?'平台公告':'Brand announcement',exact:true}).click()
  const dialog=page.getByRole('dialog',{name:zh?'编辑公告':'Edit announcement',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('combobox',{name:zh?'时区':'Time zone',exact:true})).toHaveCount(0);await expect(dialog.locator('.platform-temporal-value').first()).toHaveValue('2026-11-12T06:00:00')
  await dialog.getByRole('button',{name:'English',exact:true}).click()
  await dialog.getByLabel(zh?'标题':'Title',{exact:true}).fill('Updated announcement')
  await dialog.getByRole('button',{name:zh?'预览':'Preview',exact:true}).click();const preview=page.getByRole('dialog',{name:'Updated announcement',exact:true});await expect(preview).toBeVisible();await expectNoPageOverflow(page);await preview.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
  const preset=page.viewportSize()!;for(const width of info.project.name.includes('reflow')?[320,640,700,1000,1440]:[preset.width]){if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:800});await expectNoPageOverflow(page);await page.screenshot({path:info.outputPath(`editor-${locale}-${width}.png`),fullPage:true})}
  await dialog.getByRole('button',{name:zh?'保存并发布':'Save and publish',exact:true}).click()
  await expect(dialog.getByRole('button',{name:zh?'重试保存':'Retry save',exact:true})).toBeEnabled();await expect(dialog.getByLabel(zh?'标题':'Title',{exact:true})).toBeDisabled()
  await dialog.getByRole('button',{name:zh?'重试保存':'Retry save',exact:true}).click();await expect(dialog).toHaveCount(0);expect(writes).toHaveLength(2);expect(writes[0]).toEqual(writes[1]);expect(JSON.parse(writes[0].body!).config).not.toHaveProperty('timezone')
 })
}

test('platform targets and preview follow the shared console structure',async({page},info)=>{
 await installLayoutApi(page,'en')
 const record=item()
 await page.route('**/api/v1/platform/**',route=>{const path=new URL(route.request().url()).pathname;const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
  if(path.endsWith('/me'))return reply({account:{id:gameId,username:'announcement_platform_reviewer',super_admin:true,brand_ids:[],permissions:[],permissions_by_brand:{},platform_permissions:['announcement.view.platform','announcement.write.platform','announcement.publish.platform']}})
  if(path.endsWith('/platform-settings'))return reply({timezone:'Asia/Singapore',date_format:'YYYY-MM-DD',time_format:'HH:mm:ss',display_name:'Platform',logo_text:'P',logo_url:'',version:1,updated_at:'2026-11-11T00:00:00Z'})
  if(path==='/api/v1/platform/brands')return reply({items:[],total_count:'0'})
  if(path==='/api/v1/platform/announcements')return reply({items:[record]})
  if(path==='/api/v1/platform/announcements/brands')return reply({items:[{id:gameId,name:'Aurora'}]})
  return route.fallback()
 })
 await page.goto(origins.platform)
 await expect(page.locator('.management-console')).toBeVisible()
 if(await page.getByRole('button',{name:'Menu',exact:true}).isVisible())await page.getByRole('button',{name:'Menu',exact:true}).click()
 await expect(page.getByRole('button',{name:'Operations',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Operations',exact:true}).click()
 await page.getByTestId('platform-operations-tabs').getByRole('button',{name:'Announcements',exact:true}).click()
 const list=page.getByTestId('announcements-management');await expect(list).toBeVisible();await cancelImagePicker(page,'en');await list.getByRole('button',{name:'Platform notice',exact:true}).click()
 const dialog=page.getByRole('dialog',{name:'Edit announcement',exact:true});await expect(dialog).toBeVisible();await expect(dialog.getByRole('combobox',{name:'Time zone',exact:true})).toHaveCount(0);await expect(dialog.getByRole('combobox',{name:'Brands',exact:true})).toBeVisible();await selectControl(dialog.getByRole('combobox',{name:'Brands',exact:true}),'selected');await dialog.getByRole('checkbox',{name:'Aurora',exact:true}).check();await expectNoPageOverflow(page)
 await dialog.locator('.detail-modal-content').evaluate(el=>el.scrollTop=0)
 await page.screenshot({path:info.outputPath('platform-editor.png'),fullPage:true})
})

for (const locale of ['en', 'zh-CN'] as const) {
 test(`user announcement rows and message icon follow system ${locale}`, async ({page}, info) => {
  await installLayoutApi(page, locale)
  await page.route('**/api/v1/announcements/receipts',r=>r.fulfill({json:{success:true,data:{recorded:true}}}))
  const first=item('Platform service announcement with a long title that wraps naturally')
  first.config.pinned=true
  const second={...item('Brand notice',gameId),id:'cccccccc-cccc-4ccc-8ccc-cccccccccccc'}
  await page.route('**/api/v1/announcements',r=>r.fulfill({json:{success:true,data:{items:[first,second]}}}))
  await page.goto(`${origins.user}/announcements`)
  const directory=page.locator('.announcement-directory')
  await expect(directory).toHaveClass(/user-record-list/)
  await expect(directory.locator('.announcement-row')).toHaveCount(2)
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,640,700,1000,1440]:[preset.width]) {
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:700})
   await expectNoPageOverflow(page)
   expect(await page.locator('.workspace-page-title').evaluate(el=>el.getBoundingClientRect().height)).toBeLessThan(55)
   expect(await directory.evaluate(el=>Math.abs(el.getBoundingClientRect().width-el.parentElement!.getBoundingClientRect().width))).toBeLessThanOrEqual(1)
   await page.screenshot({path:info.outputPath(`announcement-list-${locale}-${width}.png`),fullPage:true})
  }
  await directory.locator('.announcement-row').first().focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await installOrdersLayoutApi(page)
  await page.route('**/api/v1/notifications?*',r=>r.fulfill({json:{success:true,data:{brand_id:gameId,member_id:gameId,unread_count:'1',total_count:'1',limit:20,offset:0,items:[{id,brand_id:gameId,member_id:gameId,event_type:'bet.order.placed',template_key:'bet.order.placed',template_version:1,content:notificationCatalog['bet.order.placed'],payload:{resource_id:id,points:'25'},created_at:'2026-11-11T00:00:00Z',read_at:null}]}}}))
  await page.goto(`${origins.user}/notifications`)
  const tool=page.locator('.notifications-toolbar button')
  await expect(tool).toBeVisible()
  for(const width of info.project.name.includes('reflow')?[320,640,1000,1440]:[preset.width]) {
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:700})
   expect(await tool.locator('svg').evaluate(el=>({width:el.getBoundingClientRect().width,height:el.getBoundingClientRect().height,stroke:el.getAttribute('stroke-width')}))).toEqual({width:20,height:20,stroke:'1.75'})
   const box=await tool.boundingBox()
   expect(box!.width).toEqual(box!.height)
   expect(box!.width).toBeGreaterThanOrEqual(width<=900||info.project.name.includes('android')||info.project.name.includes('ios')?44:32)
   await expectNoPageOverflow(page)
   await page.screenshot({path:info.outputPath(`message-icon-${locale}-${width}.png`),fullPage:true})
  }
 })
}
