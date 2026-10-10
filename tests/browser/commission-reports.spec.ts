import {test,expect,type APIResponse,type Page} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
import {rememberAdminSession} from './support/admin-session';

const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const admin=`${origin}/api/v1/admin`,brand='0199a000-0000-7000-8000-000000000001';
const fixture=process.env.COMMISSION_FIXTURE_BIN;
const expected={entry_count:'4',paid_entry_count:'1',paid_points:'1',adjustment_entry_count:'3',adjustment_credit_points:'3',adjustment_debit_points:'4',correction_entry_count:'0',correction_credit_points:'0',correction_debit_points:'0',net_points:'0'};
async function data<T>(r:Pick<APIResponse,'text'|'status'>):Promise<T>{const text=await r.text();expect(r.status(),text).toBe(200);const e=JSON.parse(text);expect(e.success).toBe(true);return e.data as T;}
function verify(){return JSON.parse(execFileSync(fixture!,['verify'],{env:process.env,encoding:'utf8',timeout:30_000})) as {commission_ledger_entries:number;commission_wallet_points:number;economic_fingerprint:string};}
async function navigate(page:Page){
  if(await page.evaluate(()=>innerWidth<=700)){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:/报表和对账|Reports/}).click();}
  else await page.locator('.side-nav').getByRole('button',{name:/报表和对账|Reports/}).click();
}

test('actual commission posting report filters saved beneficiaries and validates complete CSV without financial writes',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD,'Provide completed owned commission workflow fixture');
  const before=verify();expect(before.commission_ledger_entries).toBe(1);expect(before.commission_wallet_points).toBe(0);
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_admin',password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  await rememberAdminSession(context,'commission_admin',origin);
  const get=async<T>(path:string)=>data<T>(await page.request.get(`${admin}${path}`,{headers:{'X-Brand-ID':brand}}));
  const jobs=await get<{items:Array<{id:string;cycle_id:string;payout_mode:string;paid_points:string}>}>('/commission-payments?limit=100&offset=0');
  const payment=jobs.items.find(p=>p.payout_mode==='automatic'&&p.paid_points==='1')!;expect(payment).toBeTruthy();
  const targets=await get<{items:Array<{id:string;agent_id:string;member_id:string}>}>(`/commission-payments/${payment.id}/targets?limit=20&offset=0`);const target=targets.items[0]!;
  const writes:string[]=[],errors:string[]=[];page.on('request',r=>{if(r.url().startsWith(admin)&&r.method()!=='GET')writes.push(r.url());});page.on('pageerror',e=>errors.push(e.message));
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);await navigate(page);
  const panel=page.locator('.commission-report');await expect(panel.getByRole('heading',{name:'佣金账本报表',exact:true})).toBeVisible();
  await panel.getByTestId('commission-report-group').selectOption('agent');
  const query=page.waitForResponse(r=>r.request().method()==='GET'&&r.url().startsWith(`${admin}/reports/commission?`));
  await panel.getByTestId('commission-report-query').click();const report=await data<{summary:typeof expected;total_groups:string;items:Array<{key:string}>;query:{from:string;to:string}}>(await query);
  expect(report.summary).toEqual(expected);expect(report.total_groups).toBe('1');expect(report.items[0]?.key).toBe(target.agent_id);
  await expect(panel.getByTestId('commission-report-summary').locator('strong')).toHaveText(['4','1','1','3','3','4','0','0','0','0']);
  await panel.getByLabel('代理 UUID',{exact:true}).fill(target.agent_id);await panel.getByLabel('会员 UUID',{exact:true}).fill(target.member_id);await panel.getByLabel('周期 UUID',{exact:true}).fill(payment.cycle_id);await panel.getByTestId('commission-report-group').selectOption('cycle');
  const filteredResponse=page.waitForResponse(r=>r.request().method()==='GET'&&r.url().startsWith(`${admin}/reports/commission?`));await panel.getByTestId('commission-report-query').click();
  const filtered=await data<{summary:typeof expected;items:Array<{key:string}>;query:{from:string;to:string;agent_id:string;member_id:string;cycle_id:string}}>(await filteredResponse);
  expect(filtered.summary).toEqual(expected);expect(filtered.items[0]?.key).toBe(payment.cycle_id);expect(filtered.query).toMatchObject({agent_id:target.agent_id,member_id:target.member_id,cycle_id:payment.cycle_id});
  // Typing another draft does not silently change the already committed
  // filter that the export captures. The pending draft is deliberately wrong.
  await panel.getByLabel('代理 UUID',{exact:true}).fill('ffffffff-ffff-4fff-8fff-ffffffffffff');
  const csvResponse=page.waitForResponse(r=>r.request().method()==='GET'&&r.url().startsWith(`${admin}/reports/commission.csv?`));
  const download=page.waitForEvent('download');await panel.getByTestId('commission-report-export').click();
  const csv=await csvResponse;expect(csv.status(),await csv.text()).toBe(200);
  expect(csv.headers()['x-report-format-version']).toBe('2');
  expect(new URL(csv.url()).searchParams.get('agent_id')).toBe(target.agent_id);expect(new URL(csv.url()).searchParams.has('limit')).toBe(false);
  const file=await download,filePath=await file.path();expect(filePath).toBeTruthy();const bytes=readFileSync(filePath!);
  expect(createHash('sha256').update(bytes).digest('hex')).toBe(csv.headers()['x-report-sha256']);expect(String(bytes.length)).toBe(csv.headers()['x-report-byte-count']);expect(csv.headers()['x-report-group-count']).toBe('1');expect(bytes.subarray(0,3)).toEqual(Buffer.from([0xef,0xbb,0xbf]));
  expect(bytes.toString('utf8')).toContain('record_type,brand_id,snapshot_at,timezone,from,to,group_by,agent_id,member_id,cycle_id,key,label,entry_count,paid_entry_count,paid_points,adjustment_entry_count,adjustment_credit_points,adjustment_debit_points,correction_entry_count,correction_credit_points,correction_debit_points,net_points');
  expect(bytes.toString('utf8').split('\n').filter(line=>line.startsWith('group,'))).toHaveLength(1);
  // A posting window containing only the last clawback has negative net
  // movement. This is not a negative commission calculation or wallet.
  const history=await get<{items:Array<{ledger_entry_id:string;delta_points:string}>}>(`/commission-payment-targets/${target.id}/adjustments?limit=100&offset=0`);const last=history.items[0]!;expect(last.delta_points).toBe('-4');
  // Business-record creation and ledger posting timestamps are distinct.
  // Use the actual immutable ledger time, never the adjustment's created_at.
  const ledger=await get<{items:Array<{id:string;created_at:string}>}>(`/wallets/${target.member_id}/ledger?limit=100&offset=0`);
  const posting=ledger.items.find(row=>row.id===last.ledger_entry_id)!;expect(posting).toBeTruthy();
  const narrow=new URLSearchParams({from:posting.created_at,to:new Date(Date.now()+60_000).toISOString(),group_by:'cycle',cycle_id:payment.cycle_id});
  const negative=await get<{summary:typeof expected}>(`/reports/commission?${narrow}`);expect(negative.summary.net_points).toBe('-4');expect(negative.summary.entry_count).toBe('1');
  const negativeCSV=await page.request.get(`${admin}/reports/commission.csv?${narrow}`,{headers:{'X-Brand-ID':brand}});expect(negativeCSV.status()).toBe(200);expect(await negativeCSV.text()).toContain(",'\u002d4");
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await panel.screenshot({path:info.outputPath('commission-report-panel.png')});
  await page.getByTestId('admin-language').selectOption('en');await expect(panel.getByRole('heading',{name:'Commission ledger report',exact:true})).toBeVisible();
  expect(writes).toEqual([]);expect(errors).toEqual([]);expect(verify().economic_fingerprint).toBe(before.economic_fingerprint);
});
