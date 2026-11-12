import {installConsoleSession,openBrandPage} from './real-console-session'
import {expectRealEntry,expectListLayout} from './real-page-review'
import {expect,test} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow} from './fixture'
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',cycleId='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',runId='cccccccc-cccc-4ccc-8ccc-000000000001',agent='dddddddd-dddd-4ddd-8ddd-000000000001',paymentId='eeeeeeee-eeee-4eee-8eee-000000000001',earningId='ffffffff-ffff-4fff-8fff-000000000001',order='11111111-1111-4111-8111-000000000001',audit='22222222-2222-4222-8222-000000000001',time='2026-10-20T00:00:00Z'
const cycle={id:cycleId,brand_id:brand,window_from:'2026-10-12T16:00:00Z',window_to:'2026-10-19T16:00:00Z',anchor_order_id:order,calendar:{timezone:'Asia/Singapore',cycle:'weekly',boundary_time:'00:00:00',weekday:1,month_day:null,short_month:''},state:'ready',version:2,target_count:'1',scan_complete:true,current_run_id:runId,current_generation:'1',evidence_epoch:'1',evidence_current:true,calculated_count:'1',earning_count:'1',total_points:'20',created_by:null,creation_actor_type:'system',reason:'',created_at:time,updated_at:time,last_error_code:null,creation_audit_log_id:audit}
const payment={id:paymentId,brand_id:brand,cycle_id:cycleId,run_id:runId,state:'awaiting_approval',payout_mode:'manual',version:1,evidence_epoch:'1',total_points:'20',paid_points:'0',target_count:'1',paid_count:'0',creation_audit_log_id:audit,last_error_code:null,created_at:time,updated_at:time}
const correctionPlanId='44444444-4444-4444-8444-444444444444',correctionExecutionId='55555555-5555-4555-8555-555555555555'
const plan={id:correctionPlanId,brand_id:brand,cycle_id:cycleId,payment_id:paymentId,run_id:runId,state:'ready',payout_mode:'manual',version:3,evidence_epoch:'8',before_points:'10',calculated_points:'8',credit_points:'0',debit_points:'2',net_points:'-2',target_count:'1',planned_count:'1',creation_audit_log_id:audit,last_audit_log_id:audit,last_error_code:null,created_at:time,updated_at:time}
const execution={id:correctionExecutionId,brand_id:brand,cycle_id:cycleId,plan_id:correctionPlanId,run_id:runId,payout_mode:'manual',state:'awaiting_approval',version:1,plan_version:3,evidence_epoch:'8',credit_points:'0',debit_points:'2',net_points:'-2',applied_credit_points:'0',applied_debit_points:'0',target_count:'1',applied_count:'0',paused_plan_target_id:null,last_error_code:null,creation_audit_log_id:audit,last_audit_log_id:audit,approved_by:null,approval_actor_type:null,approval_audit_log_id:null,cycle_hold_active:false,created_at:time,updated_at:time}
const target={id:'99999999-9999-4999-8999-999999999999',brand_id:brand,plan_id:correctionPlanId,agent_id:agent,member_id:agent,original_target_id:'88888888-8888-4888-8888-888888888888',earning_id:null,adjustment_version:2,previous_correction_target_id:'66666666-6666-4666-8666-666666666666',financial_version:4,points_before:'10',points_after:'8',delta_points:'-2',creation_audit_log_id:audit,created_at:time}
const exTarget={id:'77777777-7777-4777-8777-777777777777',brand_id:brand,execution_id:correctionExecutionId,plan_target_id:target.id,agent_id:agent,member_id:agent,points_before:'10',points_after:'8',delta_points:'-2',state:'pending',ledger_entry_id:null,audit_log_id:null,financial_version:null,created_at:time,applied_at:null}
for(const locale of ['zh-CN','en'] as const){
 test(`commission calculations and payments workflow (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',errors:string[]=[],requests:string[]=[]
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',async route=>{
   expect(route.request().method()).toBe('GET')
   const url=new URL(route.request().url()),path=url.pathname;requests.push(url.pathname+url.search)
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{[`brand:${brand}`]:{name:'Aurora'},[`member:${agent}`]:{name:'test_member_01'},[`cycle:${cycleId}`]:{name:'2026-10-13 → 2026-10-20',from:cycle.window_from,to:cycle.window_to},[`agent:${agent}`]:{name:'aurora_agent_01'}}}})
   const pageData=(items:unknown[],scope:Record<string,string>={})=>reply({brand_id:brand,...scope,items,total_count:String(items.length),limit:Number(url.searchParams.get('limit')??20),offset:Number(url.searchParams.get('offset')??0)})
   if(path.endsWith('/filter-options'))return reply({brand_id:brand,items:[{id:url.searchParams.get('kind')==='cycle'?cycleId:url.searchParams.get('kind')==='agent'?agent:order,name:url.searchParams.get('kind')==='cycle'?'2026-10-13 → 2026-10-20':'aurora_agent_01'}],limit:100,offset:0})
   if(path.endsWith('/commission-cycles/candidates'))return reply({brand_id:brand,items:[{anchor_order_id:order,window_from:'2026-10-05T16:00:00Z',window_to:'2026-10-12T16:00:00Z',cycle:'weekly'}],limit:100,offset:0})
   if(path.endsWith('/commission-cycles'))return pageData([cycle])
   if(path.endsWith(`/commission-cycles/${cycleId}`))return reply(cycle)
   if(path.endsWith('/earnings'))return pageData([{id:earningId,brand_id:brand,cycle_id:cycleId,run_id:runId,agent_id:agent,member_id:agent,exact_amount:{numerator:'20',denominator:'1'},points:'20',created_at:time}],{cycle_id:cycleId,...(path.includes('/runs/')?{run_id:runId}:{})})
   if(path.endsWith('/runs'))return pageData([{id:runId,brand_id:brand,cycle_id:cycleId,generation:'1',evidence_epoch:'1',state:'ready',calculated_count:'1',earning_count:'1',total_points:'20',created_at:time}],{cycle_id:cycleId})
   if(path.endsWith('/allocations'))return reply({brand_id:brand,cycle_id:cycleId,run_id:runId,agent_id:url.searchParams.get('agent_id'),order_id:url.searchParams.get('order_id'),items:[{brand_id:brand,cycle_id:cycleId,run_id:runId,calculation_id:audit,order_id:order,agent_id:agent,member_id:agent,bettor_member_id:agent,base_points:'100',mode:'turnover',agent_ratio:'0.3',downstream_ratio:'0.1',difference_ratio:'0.2',exact_amount:{numerator:'20',denominator:'1'},created_at:time}],total_count:'1',limit:20,offset:0})
   if(path.endsWith('/calculations'))return pageData([],{cycle_id:cycleId,run_id:runId})
   if(path.endsWith('/commission-discovery'))return pageData([{id:audit,brand_id:brand,state:'registered',version:2,cycle_id:cycleId,window_from:cycle.window_from,window_to:cycle.window_to,next_check_at:time,last_error_code:null,last_audit_log_id:audit,created_at:time,updated_at:time}])
   if(path.endsWith('/commission-correction-policy'))return reply({brand_id:brand,version:2,enabled:true,audit_log_id:audit,updated_at:time})
   if(path.endsWith('/commission-correction-cases'))return pageData([{id:correctionPlanId,plan,execution_id:correctionExecutionId,execution_state:execution.state,queue:'pending'}])
   if(path.endsWith(`/commission-correction-plans/${correctionPlanId}`))return reply(plan)
   if(path.endsWith(`/commission-correction-executions/${correctionExecutionId}`))return reply(execution)
   if(path.includes('/commission-correction-plans/')&&path.endsWith('/targets'))return pageData([target],{plan_id:correctionPlanId})
   if(path.includes('/commission-correction-executions/')&&path.endsWith('/targets'))return pageData([exTarget],{execution_id:correctionExecutionId})
   if(path.endsWith('/commission-payment-policy'))return reply({brand_id:brand,version:1,enabled:true,audit_log_id:'',updated_at:time})
   if(path.endsWith('/commission-payments'))return pageData([payment])
   if(path.endsWith(`/commission-payments/${paymentId}`))return reply(payment)
   if(path.endsWith('/history'))return pageData([{id:audit,actor_type:'system',actor_id:null,operation:'create',state:'awaiting_approval',created_at:time}],{payment_id:paymentId})
   if(path.endsWith('/beneficiaries'))return pageData([{id:earningId,brand_id:brand,payment_id:paymentId,agent_id:agent,member_id:agent,target_id:null,original_points:'20',adjusted_points:null,state:'pending',paid_at:null}],{payment_id:paymentId})
   throw Error('Unexpected commission review request '+path)
  })
  await installConsoleSession(page,'brand',locale,brand,['commission.view.brand','commission.run.brand','commission.retry.brand','commission_payment.approve.brand','commission_payment.retry.brand','commission_adjustment.write.brand','commission_correction.approve.brand','commission_correction.retry.brand','commission_correction.continue.brand','commission_correction.execute_retry.brand'],'aaaaaaaa-aaaa-4aaa-8aaa-000000000002')
  await page.goto(origins.brand);await expect(page.locator('.management-console')).toBeVisible();await openBrandPage(page,zh?'佣金管理':'Commission management');await expectRealEntry(page,'brand')
  await expect(page.getByRole('heading',{name:zh?'佣金管理':'Commission management',exact:true})).toBeVisible()
  await expect(page.locator('[data-cycle-id]')).toBeVisible()
  const preset=page.viewportSize()!
  const widths=info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]
  for(const width of widths){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`cycles-${locale}-${width}.png`),fullPage:true})
   // Both toolbar actions must open a centered native modal instead of appending a panel.
   await page.getByRole('button',{name:zh?'任务记录':'Task records',exact:true}).click()
   const tasks=page.getByRole('dialog',{name:zh?'任务记录':'Task records',exact:true})
   await expect(tasks).toBeVisible()
   await expect(tasks).toHaveJSProperty('open',true)
   await tasks.locator('[data-discovery-id] .discovery-select').click()
   await expect(tasks).not.toBeVisible()
   const taskDetail=page.getByRole('dialog').filter({visible:true})
   await expect(taskDetail).toHaveCount(1)
   await taskDetail.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
   await expect(tasks).toBeVisible()

   const box=await tasks.boundingBox();expect(box).not.toBeNull()
   expect(Math.abs(box!.y+box!.height/2-page.viewportSize()!.height/2)).toBeLessThan(2)
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`task-history-${locale}-${width}.png`),fullPage:true})
   await tasks.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()
   await expect(tasks).toHaveCount(0)
   await page.getByRole('button',{name:zh?'补建核算':'Add missing calculation',exact:true}).click()
   const creation=page.getByRole('dialog',{name:zh?'补建核算':'Add missing calculation',exact:true})
   await expect(creation).toBeVisible()
   const creationBox=await creation.boundingBox()
   expect(Math.abs(creationBox!.y+creationBox!.height/2-page.viewportSize()!.height/2)).toBeLessThan(2)
   await expectListLayout(page)
   await creation.getByRole('button',{name:zh?'关闭':'Close',exact:true}).click()


   await page.locator('[data-cycle-id]').click()
   await expect(page.getByRole('tab',{name:zh?'核算结果':'Calculation result',exact:true})).toHaveAttribute('aria-selected','true')
   await expect(page.getByText('aurora_agent_01',{exact:true})).toBeVisible()
   await expectListLayout(page)
   await page.getByRole('tab',{name:zh?'计算明细':'Calculation details',exact:true}).click()
   await expect(page.getByText('30%',{exact:true})).toBeVisible()
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`calculation-details-${locale}-${width}.png`),fullPage:true})
   await page.getByRole('button',{name:zh?'查看派发':'View payments',exact:true}).click()
   await expect(page.locator('[data-payment-id]')).toBeVisible()
   await expect.poll(()=>requests.filter(r=>r.includes('/commission-payments?')).at(-1)).toContain(`cycle_id=${cycleId}`)
   await expectListLayout(page)
   await page.locator('[data-payment-id]').click()
   await page.getByRole('tab',{name:zh?'代理明细':'Agent details',exact:true}).click()
   await expect(page.locator('[data-testid=commission-payment-targets]')).toContainText('aurora_agent_01')
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`payment-beneficiaries-${locale}-${width}.png`),fullPage:true})
   await page.getByRole('tab',{name:zh?'操作记录':'Operation history',exact:true}).click()
   await expect(page.getByText(zh?'创建派发':'Create payout',{exact:true})).toBeVisible()
   await expectListLayout(page)
   await page.getByTestId('commission-payment-approve').click()
   const dialog=page.getByRole('dialog').filter({visible:true}).last()
   await expect(dialog).toBeVisible();await expect(dialog).toContainText(zh?'计划派发积分':'Planned points');await expect(dialog).toContainText('20');await expect(dialog).toContainText('Aurora')
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`payment-review-${locale}-${width}.png`),fullPage:true})
   await dialog.locator('.detail-modal-close').click()
   await page.getByRole('button',{name:zh?'查看核算':'View calculation',exact:true}).click()
   await expect(page.getByRole('tab',{name:zh?'核算结果':'Calculation result',exact:true})).toBeVisible()
   await page.getByRole('dialog',{name:zh?'佣金核算':'Commission calculations',exact:true}).locator('.detail-modal-close').click()
   await expect(page.locator('[data-cycle-id]')).toBeVisible()
   await page.locator('[data-cycle-id]').locator('xpath=ancestor::tr').locator('.member-menu-trigger').click();await page.locator('[data-cycle-corrections]').click()
   const corrections=page.getByRole('dialog',{name:zh?'佣金更正':'Commission corrections',exact:true})
   await expect(corrections).toBeVisible()
   await expect.poll(()=>requests.filter(r=>r.includes('/commission-correction-cases?')).at(-1)).toContain(`cycle_id=${cycleId}`)
   await expect(corrections.locator('.association-filter').filter({has:page.locator('[data-kind="cycle"]')})).toHaveCount(0)
   await corrections.getByRole('button',{name:zh?'清空':'Clear',exact:true}).click()
   await expect.poll(()=>requests.filter(r=>r.includes('/commission-correction-cases?')).at(-1)).toContain(`cycle_id=${cycleId}`)
   await corrections.locator('[data-case-id]').click()
   await expect(corrections.getByTestId('cc-plan-detail')).toContainText('10')
   await corrections.getByRole('tab',{name:zh?'代理差额':'Agent differences',exact:true}).click()
   await expect(corrections).toContainText('aurora_agent_01')
   await corrections.getByTestId('cc-approve').click()
   const correctionReview=page.getByTestId('cc-review')
   await expect(correctionReview).toBeVisible();await expect(correctionReview).toContainText('2')
   await correctionReview.locator('.detail-modal-close').click()
   await expectListLayout(page)
   await page.screenshot({path:info.outputPath(`correction-dialog-${locale}-${width}.png`),fullPage:true})
   await corrections.locator(':scope > .detail-modal-heading .detail-modal-close').click()
   await expect(corrections).toHaveCount(0)
   await expect(page.locator('[data-cycle-id]')).toBeVisible()
   await page.locator('[data-cycle-id]').locator('xpath=ancestor::tr').locator('.member-menu-trigger').click();await page.locator('[data-cycle-payments]').click()
   const payments=page.getByRole('dialog',{name:zh?'佣金派发':'Commission payouts',exact:true})
   await expect(payments).toBeVisible()
   await payments.getByRole('button',{name:zh?'清空':'Clear',exact:true}).click()
   await expect.poll(()=>requests.filter(r=>r.includes('/commission-payments?')).at(-1)).toContain(`cycle_id=${cycleId}`)
   await payments.locator('[data-payment-id]').click()
   await payments.getByRole('button',{name:zh?'查看核算':'View calculation',exact:true}).click()
   const calculation=page.getByRole('dialog',{name:zh?'佣金核算':'Commission calculations',exact:true})
   await expect(calculation).toBeVisible()
   await calculation.locator('.detail-modal-close').click()
   await expect(page.locator('[data-cycle-id]')).toBeVisible()
  }
  expect(errors).toEqual([])
 })
}

for(const locale of ['zh-CN','en'] as const){
 test(`betting confirmation stays centered (${locale})`,async({page},info)=>{
  const title=locale==='zh-CN'?'投注确认':'Confirm bets',close=locale==='zh-CN'?'关闭':'Close',confirm=locale==='zh-CN'?'提交':'Submit'
  await page.route('**/__betting-dialog-review',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app"></div><script type="module">import{createApp,h,ref}from'/node_modules/.vite/deps/vue.js';import BettingSheet from'/@fs${resolve('user-web/src/BettingSheet.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('user-web/src/style.css')}';createApp({setup(){const open=ref(false);return()=>[h('button',{onClick:()=>open.value=true},'${title}'),open.value?h(BettingSheet,{title:'${title}',closeLabel:'${close}',onClose:()=>open.value=false},{default:()=>Array.from({length:16},(_,i)=>h('p','${title} · '+(i+1))),footer:()=>h('button',{class:'button button-primary'},'${confirm}') }):null]}}).mount('#app')</script></body></html>`}))
  await page.goto(origins.brand+'/__betting-dialog-review')
  const preset=page.viewportSize()!
  for(const width of info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]){
   if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height})
   const trigger=page.getByRole('button',{name:title,exact:true});await trigger.focus();await trigger.press('Enter')
   const dialog=page.getByRole('dialog',{name:title,exact:true});await expect(dialog).toBeVisible()
   const box=await dialog.boundingBox();expect(Math.abs(box!.y+box!.height/2-page.viewportSize()!.height/2)).toBeLessThan(2)
   expect(box!.x).toBeGreaterThanOrEqual(11);expect(box!.y).toBeGreaterThanOrEqual(11)
   await expectListLayout(page)
   await dialog.locator('.detail-modal-content').evaluate(node=>node.scrollTop=node.scrollHeight)
   await expect(dialog.locator('.detail-modal-content p').last()).toBeInViewport()
   await expect(dialog.locator('.detail-modal-footer button')).toBeInViewport()
   await page.screenshot({path:info.outputPath(`betting-dialog-${locale}-${width}.png`),fullPage:true})
   await dialog.locator('.detail-modal-close').click();await expect(dialog).toHaveCount(0);await expect(trigger).toBeFocused()
  }
 })
}
