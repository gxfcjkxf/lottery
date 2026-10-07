import {test,expect,type APIResponse,type Page} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {rememberAdminSession} from './support/admin-session';

const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const admin=`${origin}/api/v1/admin`,brand='0199a000-0000-7000-8000-000000000001';
const fixture=process.env.COMMISSION_FIXTURE_BIN;
type Payment={id:string;state:string;version:number;payout_mode:string;total_points:string;paid_points:string;paid_count:string;target_count:string};
async function data<T>(response:Pick<APIResponse,'text'|'status'>):Promise<T>{const body=await response.text();expect(response.status(),body).toBe(200);const e=JSON.parse(body);expect(e.success).toBe(true);return e.data as T;}
function command(name:'pay'|'verify'){return JSON.parse(execFileSync(fixture!,[name],{env:process.env,encoding:'utf8',timeout:30_000})) as Record<string,unknown>;}
async function navigate(page:Page,project:string,destination:string){
  if(project==='mobile'){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:new RegExp(destination)}).click();}
  else await page.locator('.side-nav').getByRole('button',{name:new RegExp(destination)}).click();
}
async function confirm(page:Page){const dialog=page.getByTestId('commission-payment-review');await expect(dialog).toBeVisible();await dialog.getByRole('checkbox').check();await dialog.getByRole('button',{name:'确认提交',exact:true}).click();}

test('real payout opt-in credits automatic C and preserves lost manual approval receipts after completion',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD,'Provide explicitly owned commission workflow fixture');
  test.setTimeout(90_000);
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
  const before=command('verify');expect(before.commission_ledger_entries).toBe(0);expect(before.commission_wallet_points).toBe(0);
  // This runs after the cycle-management spec in the same owned CI database.
  // No production scheduler is started; all advancement is explicit.
  expect(command('pay').processed).toBe(0);
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_admin',password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  rememberAdminSession('commission_admin',await context.cookies(`${admin}/me`),origin);
  const get=async<T>(path:string)=>data<T>(await page.request.get(`${admin}${path}`,{headers:{'X-Brand-ID':brand}}));
  expect((await get<{enabled:boolean;version:number;audit_log_id:string}>('/commission-payment-policy')).enabled).toBe(false);
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
  await navigate(page,info.project.name,'佣金派发|Commission payouts');
  let panel=page.locator('.commission-payments');
  await expect(panel.getByRole('heading',{name:'佣金派发管理',exact:true})).toBeVisible();
  await expect(panel.getByTestId('commission-payment-policy-enabled')).not.toBeChecked();
  await panel.getByTestId('commission-payment-policy-enabled').check();
  await panel.getByTestId('commission-payment-policy-reason').fill('Enable only this explicitly owned synthetic payout workflow');
  await panel.getByTestId('commission-payment-policy-review').click();
  await expect(page.getByTestId('commission-payment-enable-warning')).toContainText('历史');
  const enableCommitted=page.waitForResponse(r=>r.request().method()==='PUT'&&r.url()===`${admin}/commission-payment-policy`);
  await confirm(page);await data(await enableCommitted);
  await expect(panel.getByTestId('commission-payment-receipt')).toContainText('策略版本 2');
  command('pay');
  const jobs=await get<{items:Payment[]}>('/commission-payments?limit=100&offset=0');
  const auto=jobs.items.find(p=>p.payout_mode==='automatic')!,manual=jobs.items.find(p=>p.state==='awaiting_approval')!;
  expect(auto).toBeTruthy();expect(auto.state).toBe('paid');expect(auto.total_points).toBe('1');expect(auto.paid_points).toBe('1');
  expect(manual).toBeTruthy();expect(manual.paid_count).toBe('0');
  const afterAuto=command('verify');expect(afterAuto.commission_ledger_entries).toBe(1);expect(afterAuto.commission_wallet_points).toBe(1);expect(afterAuto.economic_fingerprint).not.toBe(before.economic_fingerprint);
  await panel.getByTestId('commission-payments-refresh').click();
  await panel.getByRole('button',{name:manual.id,exact:true}).click();
  await expect(panel.locator('.cp-detail')).toContainText('待审批');
  await panel.getByLabel('审批 / 重试原因',{exact:true}).fill('Approve immutable zero-rounding target without creating a zero ledger');
  const writes:Array<{body:string|null;key:string|undefined;actor:string|undefined}>=[];
  await page.route(`**/api/v1/admin/commission-payments/${manual.id}/approve`,async route=>{
    writes.push({body:route.request().postData(),key:route.request().headers()['idempotency-key'],actor:route.request().headers()['x-commission-payment-actor-id']});
    expect(writes.at(-1)?.actor).toMatch(/^[0-9a-f-]{36}$/);
    const response=await route.fetch();expect(response.status(),await response.text()).toBe(200);
    const receipt=(await response.json()).data as Payment;expect(receipt.state).toBe('paying');expect(receipt.version).toBe(manual.version+1);
    if(writes.length===1){command('pay');await route.abort('failed');}else{expect(writes[1]).toEqual(writes[0]);await route.fulfill({response});}
  });
  await panel.getByTestId('commission-payment-approve').click();await confirm(page);
  await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();
  expect((await get<Payment>(`/commission-payments/${manual.id}`)).state).toBe('paid');
  await panel.locator('.cp-heading').getByRole('button',{name:'刷新',exact:true}).click();
  await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();
  await navigate(page,info.project.name,'佣金和奖励|Commissions.*rewards');
  await navigate(page,info.project.name,'佣金派发|Commission payouts');panel=page.locator('.commission-payments');
  await panel.getByRole('button',{name:'使用原请求重试',exact:true}).click();await confirm(page);
  await expect(panel.getByTestId('commission-payment-receipt')).toContainText(`派发中 · v${manual.version+1}`);
  await expect(panel.locator('.cp-detail')).toContainText('已入账');
  expect(writes).toHaveLength(2);
  // The manually approved 0.3 -> 0 target must finish with no zero ledger;
  // positive manual credit is separately covered by real PostgreSQL tests.
  const current=await get<Payment>(`/commission-payments/${manual.id}`);expect(current.paid_count).toBe('1');expect(current.paid_points).toBe('0');
  command('pay');expect(command('verify').economic_fingerprint).toBe(afterAuto.economic_fingerprint);
  expect(command('verify').commission_ledger_entries).toBe(1);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath('commission-payments-panel.png')});
  await page.getByTestId('admin-language').selectOption('en');
  await expect(panel.getByRole('heading',{name:'Commission payouts',exact:true})).toBeVisible();
  await expect(panel.getByTestId('commission-payment-receipt')).toContainText(`Paying · v${manual.version+1}`);
  expect(errors).toEqual([]);
});

test('current payout status renders exact credited C without further financial writes',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD,'Provide explicitly owned commission workflow fixture');
  const before=command('verify');expect(before.commission_ledger_entries).toBe(1);expect(before.commission_wallet_points).toBe(1);
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_admin',password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  rememberAdminSession('commission_admin',await context.cookies(`${admin}/me`),origin);
  const writes:string[]=[];page.on('request',r=>{if(r.url().startsWith(admin)&&r.method()!=='GET')writes.push(r.url());});
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
  await navigate(page,info.project.name,'佣金派发|Commission payouts');const panel=page.locator('.commission-payments');
  await expect(panel.getByRole('heading',{name:'佣金派发管理',exact:true})).toBeVisible();
  const jobs=await data<{items:Payment[]}>(await page.request.get(`${admin}/commission-payments?limit=100&offset=0`,{headers:{'X-Brand-ID':brand}}));
  const credited=jobs.items.find(x=>x.payout_mode==='automatic'&&x.state==='paid'&&x.paid_points==='1')!;expect(credited).toBeTruthy();
  await panel.getByRole('button',{name:credited.id,exact:true}).click();
  await expect(panel.locator('.cp-detail')).toContainText('已入账');
  await expect(panel.locator('.cp-detail')).toContainText('1 / 1');
  await expect(panel.getByTestId('commission-payment-approve')).toHaveCount(0);
  await expect(panel.getByTestId('commission-payment-retry')).toHaveCount(0);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath('commission-payments-current-panel.png')});
  await page.getByTestId('admin-language').selectOption('en');await expect(panel).toContainText('Paid');
  expect(writes).toEqual([]);expect(command('verify').economic_fingerprint).toBe(before.economic_fingerprint);
});
