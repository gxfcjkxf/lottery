import {test,expect} from '@playwright/test'
import {gameId,origins,installLayoutApi,expectNoPageOverflow,expectInsideViewport} from './fixture'
export function registerAgentModeChangeTests(reflow=false){
 for(const locale of ['en','zh-CN'] as const)test(`agent line mode change requires all new ratios and retains retry key (${locale})`,async({page},info)=>{
  const zh=locale==='zh-CN',at='2026-10-21T00:00:00Z',effective='2026-11-01T00:00:00Z',childId='aaaaaaaa-aaaa-4aaa-8aaa-000000000002'
  await installLayoutApi(page,locale)
  const root={id:gameId,brand_id:gameId,agent_account_id:gameId,parent_id:null,depth:1,path:[gameId],version:1,config:{ratio:'0.2',mode:'loss',status:'active',can_create_children:true},effective_mode:'loss',mode_source_agent_id:gameId,policy_version:1,parent_version:null,created_by:gameId,created_at:at,updated_at:at}
  const child={...root,id:childId,agent_account_id:childId,parent_id:gameId,depth:2,path:[gameId,childId],config:{...root.config,ratio:'0.1',mode:null},mode_source_agent_id:gameId,parent_version:1}
  const grants=['agent.view','agent.write','agent_policy.view'].map(p=>p+'.brand'),keys:string[]=[],bodies:unknown[]=[];let scheduled=false
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/v1/admin/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname
   const reply=(data:unknown)=>route.fulfill({json:{success:true,data,associations:{[`member:${gameId}`]:{name:'level1_agent'},[`member:${childId}`]:{name:'child_agent'},[`agent:${gameId}`]:{name:'level1_agent'},[`agent:${childId}`]:{name:'child_agent'}}}})
   if(path.endsWith('/me'))return reply({account:{id:gameId,username:'mode_reviewer',super_admin:false,brand_ids:[gameId],permissions_by_brand:{[gameId]:grants}}})
   if(path.endsWith('/brands'))return reply({items:[{id:gameId,name:'Aurora',code:'aurora',status:'active'}]})
   expect(req.headers()['x-brand-id']).toBe(gameId)
   if(path.endsWith('/agent-policy'))return reply({brand_id:gameId,version:1,config:{enabled:true,max_depth:5,loss_ratio_cap: '0.3', turnover_ratio_cap: '0.3',mode:'loss',cycle:'monthly'},updated_at:at})
   if(path.endsWith('/mode-change')){
    if(req.method()==='GET')return reply({brand_id:gameId,root_id:gameId,policy_version:1,items:[root,child]})
    expect(req.method()).toBe('POST');keys.push(req.headers()['idempotency-key']!);bodies.push(req.postDataJSON())
    if(keys.length===1)return route.abort('failed')
    scheduled=true;return reply({id:childId,brand_id:gameId,root_id:gameId,mode:'turnover',effective_at:effective,items:[{agent_id:gameId,version:1,ratio:'0.2'},{agent_id:childId,version:1,ratio:'0.1'}]})
   }
   const pending=scheduled?{pending_change:{id:childId,root_id:gameId,mode:'turnover',ratio:'0.2',effective_at:effective}}:{}
   if(path.endsWith('/agents/'+gameId))return reply({...root,...pending})
   if(path.endsWith('/agents'))return reply({brand_id:gameId,items:[{...root,...pending,username:'level1_agent',online:false,display_points:null}],limit:20,offset:0,total_count:'1'})
   throw Error('Unexpected mode change read: '+path)
  })
  await page.goto(origins.brand);await expect(page.locator('.management-console')).toBeVisible()
  const mobile=page.locator('.mobile-nav');if(await mobile.isVisible())await mobile.locator('button').last().click()
  const nav=page.locator(await mobile.isVisible()?'.mobile-more-menu':'.side-nav'),item=nav.getByRole('button',{name:zh?'代理列表':'Agent list',exact:true})
  if(!await item.isVisible())await item.locator('xpath=ancestor::details').locator('summary').click()
  await item.click();await page.getByRole('button',{name:'level1_agent',exact:true}).click()
  const detail=page.getByRole('dialog'),mode=detail.getByRole('combobox',{name:zh?'代理模式':'Agent mode',exact:true})
  await expect(detail.getByRole('textbox',{name:zh?'占成比例':'Profit share ratio',exact:true})).toHaveValue('20')
  await expect(mode).toHaveValue('loss');await expect(mode.locator('option')).toHaveText(zh?['选择模式','输赢','流水']:['Select mode','Loss','Turnover'])
  await mode.selectOption('turnover')
  const rootRatio=detail.getByRole('textbox',{name:zh?'佣金比例':'Commission ratio',exact:true}),childRatio=detail.getByRole('textbox',{name:zh?'child_agent 的新比例':'New ratio for child_agent',exact:true})
  await expect(rootRatio).toHaveValue('');await rootRatio.fill('20');await expect(childRatio).toHaveValue('')
  const submit=detail.locator('button[type=submit]');await expect(submit).toBeDisabled();await childRatio.fill('10');await expect(submit).toBeEnabled()
  const preset=page.viewportSize()!
  for(const width of reflow?[320,640,900,1440]:[preset.width]){if(reflow)await page.setViewportSize({width,height:preset.height});await expectInsideViewport(detail);await expectNoPageOverflow(page)}
  await page.screenshot({path:info.outputPath(`mode-change-${locale}.png`)})
  await submit.click()
  await expect.poll(()=>keys.length).toBe(1)
  await expect(detail.getByRole('alert')).toBeVisible()
  await expect(rootRatio).toHaveValue('20');await expect(childRatio).toHaveValue('10')
  await expect(rootRatio).toBeDisabled();await expect(childRatio).toBeDisabled()
  await detail.getByRole('button',{name:zh?'重试此操作':'Retry this operation',exact:true}).click()
  await expect.poll(()=>keys.length).toBe(2);expect(keys[0]).toBe(keys[1]);expect(bodies[0]).toEqual(bodies[1]);expect(bodies[0]).toMatchObject({items:[{agent_id:gameId,ratio:'0.2'},{agent_id:childId,ratio:'0.1'}]})
  await expect(page.getByRole('dialog').getByText(zh?/待生效：流水/:/Scheduled: Turnover/)).toBeVisible()
  await expect(page.getByRole('dialog').locator('button[type=submit]')).toBeDisabled()
  await expect(page.getByRole('dialog').getByRole('combobox',{name:zh?'代理模式':'Agent mode',exact:true})).toHaveValue('loss')
  expect(errors).toEqual([])
 })
}
