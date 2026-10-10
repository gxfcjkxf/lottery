import {test,expect,type APIResponse,type Page} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {rememberAdminSession} from './support/admin-session';

const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const admin=`${origin}/api/v1/admin`,brand='0199a000-0000-7000-8000-000000000001';
const fixture=process.env.COMMISSION_FIXTURE_BIN;
type Target={id:string;member_id:string;original_points:string;adjusted_points:string;adjustment_version:number;ledger_entry_id:string};
type Receipt={id:string;version:number;points_before:string;points_after:string;delta_points:string;ledger_entry_id:string;created_by:string};
async function data<T>(r:Pick<APIResponse,'text'|'status'>,status=200):Promise<T>{const body=await r.text();expect(r.status(),body).toBe(status);const e=JSON.parse(body);expect(e.success).toBe(true);return e.data as T;}
function command(name:'verify'|'notify'){return JSON.parse(execFileSync(fixture!,[name],{env:process.env,encoding:'utf8',timeout:30_000})) as Record<string,unknown>;}
async function navigate(page:Page,destination:string){
  if(await page.evaluate(()=>innerWidth<=700)){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:new RegExp(destination)}).click();}
  else await page.locator('.side-nav').getByRole('button',{name:new RegExp(destination)}).click();
}
async function confirm(page:Page){const dialog=page.getByTestId('adjustment-review-dialog');await expect(dialog).toBeVisible();await dialog.getByTestId('adjustment-confirmed').check();await dialog.getByTestId('adjustment-confirm-submit').click();}

test('real commission differences retain lost receipts across newer corrections and notify only actual ledger postings',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD||!process.env.COMMISSION_FIXTURE_USER_PASSWORD,'Provide explicitly owned commission workflow fixture');
  test.setTimeout(90_000);
  const before=command('verify');expect(before.commission_ledger_entries).toBe(1);expect(before.commission_wallet_points).toBe(1);
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_admin',password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  const me=await data<{account:{id:string}}>(await page.request.get(`${admin}/me`));
  await rememberAdminSession(context,'commission_admin',origin);
  const get=async<T>(path:string)=>data<T>(await page.request.get(`${admin}${path}`,{headers:{'X-Brand-ID':brand}}));
  const jobs=await get<{items:Array<{id:string;state:string;paid_points:string;payout_mode:string}>}>('/commission-payments?limit=100&offset=0');
  const payment=jobs.items.find(p=>p.state==='paid'&&p.payout_mode==='automatic'&&p.paid_points==='1')!;expect(payment).toBeTruthy();
  const targets=()=>get<{items:Target[];total_count:string;payment_id:string}>(`/commission-payments/${payment.id}/targets?limit=20&offset=0`);
  const original=(await targets()).items[0]!;expect(original.original_points).toBe('1');expect(original.adjusted_points).toBe('1');expect(original.adjustment_version).toBe(1);
  const path=`/commission-payment-targets/${original.id}/adjustments`;
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
  await navigate(page,'佣金派发|Commission payouts');
  const payout=page.locator('.commission-payments');await payout.getByRole('button',{name:payment.id,exact:true}).click();
  let panel=page.getByTestId('commission-adjustments-management');
  await expect(panel.getByTestId('adjustment-points')).toBeEnabled();
  await panel.getByTestId('adjustment-points').fill('3');await panel.getByTestId('adjustment-reason').fill('Review actual synthetic commission difference after original payout');
  const writes:Array<{body:string|null;key:string|undefined;actor:string|undefined}>=[];
  await page.route(`**/api/v1/admin${path}`,async route=>{
    if(route.request().method()!=='POST')return route.continue();
    writes.push({body:route.request().postData(),key:route.request().headers()['idempotency-key'],actor:route.request().headers()['x-commission-payment-actor-id']});
    const response=await route.fetch();const receipt=await data<Receipt>(response,201);
    if(writes.length===1){expect(receipt.version).toBe(2);expect(receipt.delta_points).toBe('2');await route.abort('failed');}
    else {expect(writes[1]).toEqual(writes[0]);expect(receipt.version).toBe(2);expect(receipt.points_after).toBe('3');await route.fulfill({response});}
  });
  await panel.getByTestId('adjustment-review').click();await confirm(page);
  await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();
  expect((await targets()).items[0]?.adjusted_points).toBe('3');expect(command('verify').commission_wallet_points).toBe(3);
  // A different explicitly authorized request advances the head before the
  // lost v1 request is recovered. Old receipt replay must not credit twice.
  const later=await data<Receipt>(await page.request.post(`${admin}${path}`,{headers:{Origin:origin,'X-Brand-ID':brand,'X-Commission-Payment-Actor-ID':me.account.id,'Idempotency-Key':crypto.randomUUID()},data:{version:2,points:'4',reason:'Independent later synthetic correction before old receipt recovery'}}),201);
  expect(later.version).toBe(3);expect(later.delta_points).toBe('1');
  await navigate(page,'佣金和奖励|Commissions.*rewards');await navigate(page,'佣金派发|Commission payouts');
  await payout.getByRole('button',{name:payment.id,exact:true}).click();panel=page.getByTestId('commission-adjustments-management');
  await panel.getByRole('button',{name:'使用原请求重试',exact:true}).click();await confirm(page);
  await expect(panel.getByTestId('adjustment-receipt')).toContainText('v2');
  await expect.poll(async()=>(await targets()).items[0]?.adjustment_version).toBe(3);
  await expect(panel.locator('.target-detail')).toContainText('4');expect(writes).toHaveLength(2);expect(command('verify').commission_wallet_points).toBe(4);
  await page.unroute(`**/api/v1/admin${path}`);
  await expect(panel.getByTestId('adjustment-points')).toBeEnabled();await panel.getByTestId('adjustment-points').fill('0');await panel.getByTestId('adjustment-reason').fill('Reduce current synthetic net commission to zero using exact C-only difference');
  await panel.getByTestId('adjustment-review').click();await confirm(page);
  await expect(panel.getByTestId('adjustment-receipt')).toContainText('Δ -4');
  const current=(await targets()).items[0]!;expect(current.original_points).toBe('1');expect(current.ledger_entry_id).toBe(original.ledger_entry_id);expect(current.adjusted_points).toBe('0');expect(current.adjustment_version).toBe(4);
  const history=await get<{items:Receipt[];total_count:string}>(`${path}?limit=100&offset=0`);expect(history.total_count).toBe('3');expect(history.items.map(v=>v.delta_points)).toEqual(['-4','1','2']);
  const finished=command('verify');expect(finished.commission_ledger_entries).toBe(1);expect(finished.commission_wallet_points).toBe(0);
  // Materialize real immutable events and read only the beneficiary's inbox.
  command('notify');const userBase=`${origin}/api/v1/b/aurora`;
  const login=await data<{access_token:string}>(await page.request.post(`${userBase}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_agent_owner',password:process.env.COMMISSION_FIXTURE_USER_PASSWORD}}));
  const inbox=await data<{items:Array<{event_type:string;payload:{resource_id:string;points:string};content:unknown}>}>(await page.request.get(`${userBase}/notifications?limit=100&offset=0`,{headers:{Authorization:`Bearer ${login.access_token}`}}));
  const commissions=inbox.items.filter(n=>n.event_type.startsWith('commission.'));expect(commissions).toHaveLength(4);
  expect(commissions.find(n=>n.event_type==='commission.paid')?.payload.points).toBe('1');expect(commissions.filter(n=>n.event_type==='commission.adjusted').map(n=>n.payload.points).sort()).toEqual(['-4','1','2']);
  expect(commissions.every(n=>n.content!==null)).toBe(true);expect(commissions.every(n=>Object.keys(n.payload).sort().join(',')==='points,resource_id')).toBe(true);
  expect(command('verify').economic_fingerprint).toBe(finished.economic_fingerprint);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await panel.screenshot({path:info.outputPath('commission-adjustments-panel.png')});
  await page.getByTestId('admin-language').selectOption('en');await expect(panel).toContainText('Current net commission points');expect(errors).toEqual([]);
});

test('current adjustment status preserves original payout and immutable net history without financial writes',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD,'Provide completed explicitly owned commission workflow fixture');
  const before=command('verify');expect(before.commission_ledger_entries).toBe(1);expect(before.commission_wallet_points).toBe(0);
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_admin',password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  await rememberAdminSession(context,'commission_admin',origin);
  const jobs=await data<{items:Array<{id:string;state:string;paid_points:string;payout_mode:string}>}>(await page.request.get(`${admin}/commission-payments?limit=100&offset=0`,{headers:{'X-Brand-ID':brand}}));
  const payment=jobs.items.find(p=>p.state==='paid'&&p.payout_mode==='automatic'&&p.paid_points==='1')!;expect(payment).toBeTruthy();
  const writes:string[]=[];page.on('request',r=>{if(r.url().startsWith(admin)&&r.method()!=='GET')writes.push(r.url());});
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
  await navigate(page,'佣金派发|Commission payouts');await page.locator('.commission-payments').getByRole('button',{name:payment.id,exact:true}).click();
  const panel=page.getByTestId('commission-adjustments-management');await expect(panel.locator('.history-card')).toHaveCount(3);
  const facts=panel.locator('.target-detail');await expect(facts).toContainText('原始佣金积分');await expect(facts.locator('dd').nth(0)).toHaveText('1');await expect(facts.locator('dd').nth(1)).toHaveText('0');await expect(facts.locator('dd').nth(2)).toHaveText('4');
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await panel.screenshot({path:info.outputPath('commission-adjustments-current-panel.png')});
  await page.getByTestId('admin-language').selectOption('en');await expect(panel).toContainText('Current net commission points');
  expect(writes).toEqual([]);expect(command('verify').economic_fingerprint).toBe(before.economic_fingerprint);
});
