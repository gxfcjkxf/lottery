import { expectRealEntry, expectListLayout } from './real-page-review'
import { selectControl } from '../select-control'
import {test,expect} from '@playwright/test'
import {gameId,origins,installLayoutApi,expectInsideViewport} from './fixture'
export function registerMemberManagementTests(reflow=false){
 for(const locale of ['en','zh-CN'] as const)test(`member directory searches all pages and manages online accounts (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN';await installLayoutApi(page,locale)
  const errors:string[]=[],writes:{path:string;body:Record<string,unknown>;key:string|undefined}[]=[],queries:URL[]=[]
  page.on('pageerror',e=>errors.push(e.message))
  const members=Array.from({length:101},(_,i)=>({id:`bbbbbbbb-bbbb-4bbb-8bbb-${String(i+1).padStart(12,'0')}`,global_user_id:`cccccccc-cccc-4ccc-8ccc-${String(i+1).padStart(12,'0')}`,brand_id:gameId,username:`member${String(i+1).padStart(3,'0')}`,display_name:`member${String(i+1).padStart(3,'0')}`,phone:'',notes:'',tags:[] as string[],status:'normal',joined_at:'2026-10-21T00:00:00Z',online:i%2===0,display_points:i===0?'9007199254740993':'80'}))
  const grants=['user.view.brand','user.write.brand','user.create.brand','user.kick.brand','user.password_reset.brand','wallet.view.brand','join_code.view.brand']
  await page.route('**/api/v1/admin/**',async route=>{
   const req=route.request(),url=new URL(req.url()),path=url.pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data}})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'member_reviewer',super_admin:false,brand_ids:[gameId],permissions_by_brand:{[gameId]:grants}}})
   if(path.endsWith('/brands'))return reply({items:[{id:gameId,name:'Aurora',code:'aurora',status:'active',member_count:'12',agent_count:'3',recharge_quota_available:'1000',created_at:'2026-10-24T00:00:00Z'}], total_count: '1'})
   expect(req.headers()['x-brand-id']).toBe(gameId)
   if(path.endsWith('/users')&&req.method()==='GET'){
    queries.push(url);const q=url.searchParams.get('q')?.toLowerCase()??'',online=url.searchParams.get('online'),limit=Number(url.searchParams.get('limit')),offset=Number(url.searchParams.get('offset'))
    const selected=members.filter(m=>(!q||`${m.username} ${m.display_name} ${m.phone}`.toLowerCase().includes(q))&&(online===null||m.online===(online==='true')))
    return reply({items:selected.slice(offset,offset+limit),limit,offset,total_count:String(selected.length)})
   }
   if(path.endsWith('/join-codes')) {
    const memberId=url.searchParams.get('owner_member_id')!
    return reply({brand_id:gameId,kind:'referral',owner_member_id:memberId,items:[{id:gameId,brand_id:gameId,kind:'referral',code:'0123456789ABCDEF01234567',owner_member_id:memberId,agent_id:null,status:'active',starts_at:null,expires_at:null,version:1,usable:true,created_at:'2026-10-24T00:00:00Z',updated_at:'2026-10-24T00:00:00Z'}],limit:20,offset:0,total_count:'1'})
   }
   if(path.includes('/wallets/')) {
    const member=members.find(m=>path.endsWith(m.id))!;expect(member).toBeTruthy()
    const current=member.display_points, frozen=member===members[0]?'10':'0', available=(BigInt(current)-BigInt(frozen)).toString()
    const empty={available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'}
    return reply({account_id:gameId,brand_id:gameId,member_id:member.id,version:1,display_points:current,available_points:available,frozen_points:frozen,withdrawal_points:'0',recharge_points:available,winning_points:'0',gift_points:'0',commission_points:'0',manual_frozen_points:frozen,system_frozen_points:'0',by_source:{recharge:{...empty,available,manual_frozen:frozen},winning:{...empty},gift:{...empty},commission:{...empty}}})
   }
   const target=members.find(m=>path.includes(m.id));expect(target).toBeTruthy()
   const body=req.postDataJSON();writes.push({path,body,key:req.headers()['idempotency-key']});expect(writes.at(-1)!.key).toBeTruthy()
   if(path.endsWith('/reset-password')){expect(req.method()).toBe('POST');expect(body.password).toBe('member-password-2026');target!.online=false}
   else if(path.endsWith('/kick')){expect(req.method()).toBe('POST');target!.online=false}
   else{expect(req.method()).toBe('PATCH');expect(body.notes).toBe('updated member notes');target!.notes=body.notes}
   return reply({audit_log_id:gameId})
  })
  await page.goto(origins.brand);await expect(page.locator('.management-console')).toBeVisible()
  if(await page.locator('.mobile-nav').isVisible())await page.locator('.mobile-nav button').nth(1).click()
  else await page.locator('.side-nav').getByRole('button',{name:zh?'会员管理':'Member management',exact:true}).click()
  await expect(page.getByRole('heading',{name:zh?'会员管理':'Member management',exact:true})).toBeVisible()
  await expect(page.locator('.directory-page [data-refresh-button]')).toHaveCount(0)
  const table=page.locator('.member-directory-table'),rows=table.locator('tbody tr'),pagination=page.locator('.table-pagination')
  await expect(rows).toHaveCount(100);await expect(pagination.locator('.table-pagination-summary .table-pagination-desktop')).toHaveText(zh ? '第 1 页 · 本页 100 条 · 共 101 条' : 'Page 1 · 100 records on this page · Total: 101')
  await expect(rows.first().getByRole('button',{name:'member001',exact:true})).toHaveCount(1)
  await expect(rows.first().locator('small')).toHaveCount(0)
  await expect(rows.first()).toContainText('9007199254740993')
  await expect(page.locator('#member-search-scope,.page-footer,.filter-tabs')).toHaveCount(0)
  await pagination.getByRole('button',{name:zh?'下一页':'Next page',exact:true}).click()
  await expect(rows).toHaveCount(1);await expect(rows).toContainText('member101')
  const search=page.getByRole('textbox',{name:zh?'搜索账号或手机号':'Search account or phone',exact:true})
  await search.fill('MEMBER001');await search.press('Enter')
  await expect(rows).toHaveCount(1);await expect(rows).toContainText('member001')
  expect(queries.at(-1)!.searchParams.get('q')).toBe('MEMBER001');expect(queries.at(-1)!.searchParams.get('offset')).toBe('0')
  await page.locator('.member-filters').getByRole('button',{name:zh?'清空':'Clear',exact:true}).click()
  const online=page.getByRole('combobox',{name:zh?'在线状态':'Online status',exact:true})
  await selectControl(online, 'false');await page.locator('.member-filters button[type=submit]').click()
  await expect(rows).toHaveCount(50);await expect(pagination).toContainText('50')
  expect(queries.at(-1)!.searchParams.get('online')).toBe('false')
  await page.locator('.member-filters').getByRole('button',{name:zh?'清空':'Clear',exact:true}).click()
  await expect(rows).toHaveCount(100)
  const detail=page.getByRole('dialog',{name:zh?'会员详情':'Member details',exact:true})
  await rows.first().getByRole('button',{name:'member001',exact:true}).click()
  await expect(detail).toContainText('9007199254740993 / 9007199254740983')
  await expect(detail.locator('[title]')).toHaveAttribute('title',zh?'当前积分：9007199254740993\n可用积分：9007199254740983':'Current points: 9007199254740993\nAvailable points: 9007199254740983')
  await expect(detail.getByRole('button',{name:zh?'查看积分':'View points',exact:true})).toHaveCount(0)
  await expect(detail.locator('.owner-join-code')).toContainText('0123456789ABCDEF01234567')
  await expect(detail.locator('.owner-join-code button')).toHaveCount(0)
  const detailViewport=page.viewportSize()!
  for(const width of reflow?[320,640,900,1440]:[detailViewport.width]){
   if(reflow)await page.setViewportSize({width,height:detailViewport.height})
   await expectInsideViewport(detail);await expectRealEntry(page,'brand');await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`member-details-${locale}-${width}.png`)})
  }
  if(reflow)await page.setViewportSize(detailViewport)
  await page.keyboard.press('Escape')
  await rows.nth(1).getByRole('button',{name:'member002',exact:true}).click()
  const points=detail.locator('.member-details > div').filter({has:page.locator('dt',{hasText:zh?'积分':'Points'})}).locator('dd')
  await expect(points).toHaveText('80');await expect(points.locator('[title]')).toHaveCount(0);await page.keyboard.press('Escape')
  const create=page.getByRole('button',{name:zh?'添加会员':'Add member',exact:true})
  await create.click()
  const creation=page.getByRole('dialog',{name:zh?'添加会员':'Add member',exact:true})
  await expect(creation).toBeVisible()
  const createSubmit=creation.locator('.ui-form-actions button[type=submit]')
  if(reflow){
   await page.setViewportSize({width:320,height:800})
   await createSubmit.scrollIntoViewIfNeeded()
   await expect(createSubmit).toBeVisible()
   await expectInsideViewport(creation);await expectRealEntry(page,'brand');await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`member-create-${locale}-320.png`)})
   await page.setViewportSize(detailViewport)
  }
  await expect(creation.getByLabel(zh?'昵称':'Display name',{exact:true})).toHaveCount(0)
  await createSubmit.scrollIntoViewIfNeeded()
  await expect(createSubmit).toBeVisible()
  await expectInsideViewport(creation);await expectRealEntry(page,'brand');await expectListLayout(page)
  await page.screenshot({path:info.outputPath(`member-create-${locale}.png`)})
  await page.keyboard.press('Escape')
  const menuButton=()=>rows.first().getByRole('button',{name:zh?'member001 的操作':'Actions for member001',exact:true})
  const menu=page.getByRole('menu',{name:zh?'member001 的操作':'Actions for member001',exact:true})
  await menuButton().click();await expect(menu).toBeVisible();await expectInsideViewport(menu)
  await expect(menu.getByRole('menuitem')).toHaveCount(4)
  await page.keyboard.press('Escape');await expect(menu).not.toBeVisible()
  await menuButton().click();await menu.getByRole('menuitem',{name:zh?'编辑':'Edit',exact:true}).click()
  const edit=page.getByRole('dialog',{name:zh?'编辑会员':'Edit member',exact:true})
  await expect(edit).toBeVisible();await edit.locator('textarea').fill('updated member notes')
  await edit.getByRole('button',{name:zh?'重置密码':'Reset password',exact:true}).click()
  const reset=page.getByRole('dialog',{name:zh?'重置密码':'Reset password',exact:true})
  await expect(reset).toBeVisible();await expect(edit).not.toBeVisible();await reset.locator('input[type=password]').fill('member-password-2026');await expect(reset.getByRole('checkbox')).toHaveCount(0);await expect(reset.locator('.danger-note')).toHaveCount(0);await expectInsideViewport(reset);await expectRealEntry(page,'brand');await expectListLayout(page);await page.screenshot({path:info.outputPath(`member-reset-${locale}.png`)})
  await reset.getByRole('button',{name:zh?'确认重置密码':'Confirm reset',exact:true}).click()
  await expect(reset).not.toBeVisible();await expect(edit).toBeVisible()
  await edit.getByRole('button',{name:zh?'保存':'Save',exact:true}).click()
  await expect(edit).not.toBeVisible();await expect(rows.first()).toContainText('updated member notes')
  await menuButton().click();await expect(menu.getByRole('menuitem')).toHaveCount(3);await page.keyboard.press('Escape')
  const third=rows.nth(2);await third.getByRole('button',{name:zh?'member003 的操作':'Actions for member003',exact:true}).click()
  await page.getByRole('menu',{name:zh?'member003 的操作':'Actions for member003',exact:true}).getByRole('menuitem',{name:zh?'踢出':'Remove',exact:true}).click()
  const kick=page.getByRole('dialog',{name:zh?'踢出品牌会话':'Revoke brand session',exact:true});await expect(kick).toBeVisible()
  await kick.getByRole('button',{name:zh?'确认踢出':'Confirm removal',exact:true}).click();await expect(kick).not.toBeVisible()
  await expect(third.locator('.member-presence')).toContainText(zh?'离线':'Offline')
  expect(writes).toHaveLength(3);expect(new Set(writes.map(w=>w.key)).size).toBe(3)
  const preset=page.viewportSize()!
  for(const width of reflow?[320,640,900,1440]:[preset.width]){if(reflow)await page.setViewportSize({width,height:preset.height});await expectRealEntry(page,'brand');await expectListLayout(page)}
  await page.screenshot({path:info.outputPath(`member-management-${locale}.png`)})
  expect(errors).toEqual([])
 })
}
