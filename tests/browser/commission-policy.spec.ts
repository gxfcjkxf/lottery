import {test,expect,type APIResponse} from '@playwright/test';
import {rememberAdminSession,restoreAdminSession} from './support/admin-session';

const brand='0199a000-0000-7000-8000-000000000002';
const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const api=`${origin}/api/v1/admin`;
const uid=()=>crypto.randomUUID();
type Config={enabled:boolean;calendar:null|{timezone:string;cycle:'weekly'|'monthly';boundary_time:string;weekday:number|null;month_day:number|null;short_month:string};payout_mode:'manual'|'automatic'};
type Policy={brand_id:string;version:number;config:Config;revision_id:string;audit_log_id?:string};
async function data<T>(r:APIResponse,label:string):Promise<T>{expect(r.status(),label).toBe(200);const envelope=await r.json();expect(envelope.success,label).toBe(true);return envelope.data as T;}

test('real commission financial policy retains the original unknown receipt and immutable history without posting funds',async({page,context},info)=>{
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME||!process.env.TEST_HARBOR_ADMIN_PASSWORD,'Provide owned Harbor admin credentials');
  test.setTimeout(60_000);page.setDefaultTimeout(10_000);
  const h={Origin:origin,'X-Brand-ID':brand};
  const username=process.env.TEST_HARBOR_ADMIN_USERNAME!;
  if(!await restoreAdminSession(context,username,brand,origin))await data(await page.request.post(`${api}/auth/login`,{headers:{...h,'Idempotency-Key':uid()},data:{identifier:username,password:process.env.TEST_HARBOR_ADMIN_PASSWORD}}),'admin login');
  await rememberAdminSession(context,username,origin);
  const call=async<T>(path:string,method:'GET'|'PUT',body?:unknown)=>data<T>(await page.request.fetch(`${api}${path}`,{method,headers:{...h,...(body===undefined?{}:{'Content-Type':'application/json','Idempotency-Key':uid()})},...(body===undefined?{}:{data:body})}),`${method} ${path}`);
  const original=await call<Policy>('/commission-policy','GET');
  expect(original.config.enabled,'test must not take over a running financial policy').toBe(false);
  const postingWindow=new URLSearchParams({from:new Date(Date.now()-86_400_000).toISOString(),to:new Date(Date.now()+86_400_000).toISOString(),group_by:'entry_type',limit:'100',offset:'0'});
  const readFinancial=async()=>{
    const ledger=await call<{summary:unknown;balances:unknown;items:unknown;total_groups:string}>(`/reports/ledger?${postingWindow}`,'GET');
    return {summary:ledger.summary,balances:ledger.balances,items:ledger.items,total_groups:ledger.total_groups};
  };
  const originalFinancial=await readFinancial();
  const originalPayoutGate=await call<{enabled:boolean;version:number}>('/commission-payment-policy','GET');
  expect(originalPayoutGate.enabled,'policy configuration must not implicitly enable actual payouts').toBe(false);
  const agency=await call<{version:number;config:{enabled:boolean;max_depth:number;ratio_cap:string;mode:string;cycle:string}}>('/agent-policy','GET');
  const writes:Array<{body:string|null;key:string|undefined}>=[];
  let changedAgency=false;
  try{
    if(!agency.config.enabled||agency.config.cycle!=='weekly'){
      await call('/agent-policy','PUT',{version:agency.version,config:{...agency.config,enabled:true,cycle:'weekly'},reason:'owned browser commission policy test prerequisite'});changedAgency=true;
    }
    await page.goto(origin);
    await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
    if(info.project.name==='mobile'){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:/代理树|Agent tree/}).click();}
    else await page.locator('.side-nav').getByRole('button',{name:/代理树|Agent tree/}).click();
    const panel=page.locator('.commission-policy-panel');
    await expect(panel.getByRole('heading',{name:'佣金财务策略',exact:true})).toBeVisible();
    await expect(panel.locator('.commission-policy-notice')).toContainText('独立后台');
    await expect(panel.locator('.commission-policy-notice')).toContainText('默认关闭运行开关');
    await expect(panel.locator('.commission-policy-notice')).toContainText('可能处理现存的 ready 周期');
    await panel.getByLabel('启用佣金策略',{exact:true}).check();
    await panel.getByRole('button',{name:'明确配置日历',exact:true}).click();
    await panel.getByLabel('时区（IANA）',{exact:true}).fill('UTC');
    await panel.getByRole('combobox',{name:'周期',exact:true}).selectOption('weekly');
    await panel.getByLabel('周期边界时间',{exact:true}).fill('18:30:00');
    await panel.getByRole('combobox',{name:'周边界日',exact:true}).selectOption('1');
    await panel.getByRole('combobox',{name:'派发模式',exact:true}).selectOption('automatic');
    await panel.getByLabel('操作原因',{exact:true}).fill('browser policy configuration only; not automatic payout');
    await page.route('**/api/v1/admin/commission-policy',async route=>{
      if(route.request().method()!=='PUT'){await route.continue();return;}
      writes.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
      const response=await route.fetch();expect(response.status(),'server policy write').toBe(200);
      if(writes.length===1)await route.abort('failed');else await route.fulfill({response});
    });
    await panel.getByRole('button',{name:'核对并保存',exact:true}).click();
    await panel.getByRole('button',{name:'确认提交',exact:true}).click();
    await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();
    const afterLostReceipt=await call<Policy>('/commission-policy','GET');
    expect(afterLostReceipt.version).toBe(original.version+1);
    await panel.getByRole('button',{name:'使用原请求重试',exact:true}).click();
    await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toHaveCount(0);
    expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);
    const saved=await call<Policy>('/commission-policy','GET');
    expect(saved).toEqual(afterLostReceipt);
    expect(saved.config).toEqual({enabled:true,calendar:{timezone:'UTC',cycle:'weekly',boundary_time:'18:30:00',weekday:1,month_day:null,short_month:''},payout_mode:'automatic'});
    const history=await call<{items:Array<{id:string;version:number;audit_log_id:string|null}>}>('/commission-policy/history?limit=20&offset=0','GET');
    expect(history.items.filter(r=>r.version===saved.version)).toHaveLength(1);
    expect(history.items[0]).toMatchObject({id:saved.revision_id,audit_log_id:saved.audit_log_id});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    await panel.screenshot({path:info.outputPath('commission-policy-configured.png')});
    await page.getByTestId('admin-language').selectOption('en');
    await expect(panel.getByRole('heading',{name:'Commission financial policy',exact:true})).toBeVisible();
    await expect(panel.getByLabel('Timezone (IANA)',{exact:true})).toHaveValue('UTC');
    await expect(panel.getByRole('combobox',{name:'Payout mode',exact:true})).toHaveValue('automatic');
    expect(await call('/commission-payment-policy','GET')).toMatchObject(originalPayoutGate);
    expect(await readFinancial()).toEqual(originalFinancial);
    expect(writes).toHaveLength(2);
  }finally{
    await page.unrouteAll({behavior:'wait'});
    const current=await call<Policy>('/commission-policy','GET');
    if(current.version!==original.version)await call('/commission-policy','PUT',{version:current.version,config:original.config,reason:'restore original disabled policy after owned browser test'});
    if(changedAgency){const currentAgency=await call<{version:number}>('/agent-policy','GET');await call('/agent-policy','PUT',{version:currentAgency.version,config:agency.config,reason:'restore original agency policy after owned browser test'});}
  }
});
