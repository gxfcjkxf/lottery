import {expect,test} from '@playwright/test'
import {resolve} from 'node:path'
import {origins,expectNoPageOverflow} from './fixture'
const brand='aaaaaaaa-aaaa-4aaa-8aaa-000000000001',cycle='bbbbbbbb-bbbb-4bbb-8bbb-000000000001',run='cccccccc-cccc-4ccc-8ccc-000000000001',agent='dddddddd-dddd-4ddd-8ddd-000000000001',payment='eeeeeeee-eeee-4eee-8eee-000000000001',planId='44444444-4444-4444-8444-444444444444',executionId='55555555-5555-4555-8555-555555555555',audit='22222222-2222-4222-8222-000000000001',time='2026-10-20T00:00:00Z'
const plan={id:planId,brand_id:brand,cycle_id:cycle,payment_id:payment,run_id:run,state:'ready',payout_mode:'manual',version:3,evidence_epoch:'8',before_points:'10',calculated_points:'8',credit_points:'0',debit_points:'2',net_points:'-2',target_count:'1',planned_count:'1',creation_audit_log_id:audit,last_audit_log_id:audit,last_error_code:null,created_at:time,updated_at:time}
const execution={id:executionId,brand_id:brand,cycle_id:cycle,plan_id:planId,run_id:run,payout_mode:'manual',state:'awaiting_approval',version:1,plan_version:3,evidence_epoch:'8',credit_points:'0',debit_points:'2',net_points:'-2',applied_credit_points:'0',applied_debit_points:'0',target_count:'1',applied_count:'0',paused_plan_target_id:null,last_error_code:null,creation_audit_log_id:audit,last_audit_log_id:audit,approved_by:null,approval_actor_type:null,approval_audit_log_id:null,cycle_hold_active:false,created_at:time,updated_at:time}
const target={id:'99999999-9999-4999-8999-999999999999',brand_id:brand,plan_id:planId,agent_id:agent,member_id:agent,original_target_id:'88888888-8888-4888-8888-888888888888',earning_id:null,adjustment_version:2,previous_correction_target_id:'66666666-6666-4666-8666-666666666666',financial_version:4,points_before:'10',points_after:'8',delta_points:'-2',creation_audit_log_id:audit,created_at:time}
const exTarget={id:'77777777-7777-4777-8777-777777777777',brand_id:brand,execution_id:executionId,plan_target_id:target.id,agent_id:agent,member_id:agent,points_before:'10',points_after:'8',delta_points:'-2',state:'pending',ledger_entry_id:null,audit_log_id:null,financial_version:null,created_at:time,applied_at:null}
for(const locale of ['zh-CN','en'] as const){test(`correction and reports (${locale})`,async({page},info)=>{
 const zh=locale==='zh-CN',errors:string[]=[],requests:string[]=[];page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/**',async route=>{expect(route.request().method()).toBe('GET');const url=new URL(route.request().url()),p=url.pathname,q=url.searchParams;requests.push(p+url.search)
  const reply=(data:unknown)=>route.fulfill({json:{success:true,request_id:'read-1',data,associations:{[`brand:${brand}`]:{name:'Aurora'},[`cycle:${cycle}`]:{name:'2026-10-13 → 2026-10-20',from:'2026-10-12T16:00:00Z',to:'2026-10-19T16:00:00Z'},[`agent:${agent}`]:{name:'agent_01'},[`member:${agent}`]:{name:'agent_01'}}}})
  const pageData=(items:unknown[],scope:Record<string,string>={})=>reply({brand_id:brand,...scope,items,total_count:String(items.length),limit:Number(q.get('limit')??20),offset:Number(q.get('offset')??0)})
  if(p.endsWith('/filter-options'))return reply({brand_id:brand,items:[{id:q.get('kind')==='cycle'?cycle:agent,name:'agent_01'}],limit:100,offset:0})
  if(p.endsWith('/commission-payment-policy'))return reply({brand_id:brand,version:1,enabled:true,audit_log_id:'',updated_at:time})
  if(p.endsWith('/commission-correction-policy'))return reply({brand_id:brand,version:2,enabled:true,audit_log_id:audit,updated_at:time})
  if(p.endsWith('/commission-correction-cases'))return pageData([{id:planId,plan,execution_id:executionId,execution_state:execution.state,queue:'pending'}])
  if(p.endsWith(`/commission-payments/${payment}`))return reply({id:payment,brand_id:brand,cycle_id:cycle,run_id:run,state:'completed',payout_mode:'manual',version:1,evidence_epoch:'1',total_points:'10',paid_points:'10',target_count:'1',paid_count:'1',creation_audit_log_id:audit,last_error_code:null,created_at:time,updated_at:time})
  if(p.endsWith(`/commission-correction-plans/${planId}`))return reply(plan)
  if(p.endsWith(`/commission-correction-executions/${executionId}`))return reply(execution)
  if(p.includes('/commission-correction-plans/')&&p.endsWith('/targets'))return pageData([target],{plan_id:planId})
  if(p.includes('/commission-correction-executions/')&&p.endsWith('/targets'))return pageData([exTarget],{execution_id:executionId})
  const query={from:q.get('from'),to:q.get('to'),group_by:q.get('group_by'),limit:Number(q.get('limit')),offset:Number(q.get('offset')),agent_id:q.get('agent_id'),member_id:q.get('member_id'),cycle_id:q.get('cycle_id')}
  const common={paid_entry_count:'1',paid_points:'10',adjustment_entry_count:'0',adjustment_credit_points:'0',adjustment_debit_points:'0',correction_entry_count:'0',correction_credit_points:'0',correction_debit_points:'0'}
  if(p.endsWith('/reports/commission-analysis')){const totals={...common,observed_calculated_points:'10',calculated_points:'10',posting_entry_count:'1',actual_net_points:'10',manual_adjustment_net_points:'0',effective_target_points:'10',calculation_minus_actual_points:'0',effective_minus_actual_points:'0',calculation_complete:true,effective_target_complete:true};return reply({brand_id:brand,snapshot_at:time,timezone:'Asia/Singapore',query,coverage:{selected_cycle_count:'1',ready_cycle_count:'1',unready_cycle_count:'0'},summary:totals,items:[{key:cycle,label:cycle,totals}],total_groups:'1'})}
  if(p.endsWith('/reports/commission')){const totals={...common,entry_count:'1',net_points:'10'};return reply({brand_id:brand,snapshot_at:time,timezone:'Asia/Singapore',query,summary:totals,items:[{key:'2026-10-20',label:'2026-10-20',totals}],total_groups:'1'})}
  throw Error('Unexpected request '+p)
 })
 await page.route('**/__commission-followup',route=>route.fulfill({contentType:'text/html',body:`<!doctype html><html><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><body><div id="app"></div><script type="module">import{createApp}from'/node_modules/.vite/deps/vue.js';import Harness from'/@fs${resolve('tests/layout/CommissionFollowupHarness.vue')}';import'/@fs${resolve('shared/src/styles.css')}';import'/@fs${resolve('admin-web/src/style.css')}';import'/@fs${resolve('admin-web/src/directory.css')}';import'/@fs${resolve('shared/src/management.css')}';import'/@fs${resolve('shared/src/controls.css')}';import'/@fs${resolve('shared/src/empty-state.css')}';import'/@fs${resolve('shared/src/key-value.css')}';createApp(Harness,{locale:'${locale}'}).mount('#app')</script></body></html>`}))
 await page.goto(origins.brand+'/__commission-followup');await expect(page.locator('[data-case-id]')).toBeVisible()
 const preset=page.viewportSize()!,widths=info.project.name.includes('reflow')?[320,640,900,901,1440]:[preset.width]
 for(const width of widths){if(info.project.name.includes('reflow'))await page.setViewportSize({width,height:preset.height});await expectNoPageOverflow(page);await page.locator('[data-case-id]').click();await expect(page.getByTestId('cc-plan-detail')).toContainText('10');await expectNoPageOverflow(page);await page.getByRole('tab',{name:zh?'代理差额':'Agent differences'}).click();await expect(page.getByText('agent_01',{exact:true})).toBeVisible();await expectNoPageOverflow(page);await page.getByTestId('cc-approve').click();const dialog=page.getByRole('dialog');await expect(dialog).toContainText('2');await expectNoPageOverflow(page);await dialog.locator('.detail-modal-close').click();await page.getByRole('button',{name:zh?'返回列表':'Back to list'}).click();await page.getByRole('button',{name:'Reports',exact:true}).click();await page.getByRole('tab',{name:zh?'入账统计':'Posting activity'}).click();await expect(page.getByTestId('commission-report-summary')).toContainText('10');await expectNoPageOverflow(page);await page.getByRole('button',{name:'Corrections',exact:true}).click();await expect(page.locator('[data-case-id]')).toBeVisible()}
 await page.getByRole('button',{name:'Reports',exact:true}).click()
 for(const tab of ['posting'] as const){
  await page.getByRole('tab',{name:zh?'入账统计':'Posting activity',exact:true}).click()
  const section=page.locator('.commission-report')
  await expect(section).not.toContainText('Asia/Singapore')
  const accountSearch=section.getByRole('searchbox',{name:zh?'会员账号':'Member account',exact:true})
  await expect(accountSearch).toBeVisible()
  await accountSearch.fill('agent_01')
  const query=page.getByTestId('commission-report-query')
  await expect(query).toBeDisabled()
  await expect(section.locator('.association-search-controls').getByRole('button',{name:zh?'搜索':'Search',exact:true})).toHaveCount(0)
  await section.locator('.association-search-results').getByRole('button',{name:'agent_01',exact:true}).click()
  await expect(query).toBeEnabled();await query.click()
  await expect.poll(()=>requests.filter(x=>x.includes('/reports/commission?')).at(-1)).toContain(`member_id=${agent}`)
  const summary=section.getByTestId(tab==='cycle'?'commission-analysis-summary':'commission-report-summary')
  await expect(summary).toBeVisible()
  await expect(summary.locator('details')).toHaveCount(0)
  await expectNoPageOverflow(page)
  await page.screenshot({path:info.outputPath(`report-${tab}-${locale}.png`),fullPage:true})
 }
 expect(errors).toEqual([])
})}
