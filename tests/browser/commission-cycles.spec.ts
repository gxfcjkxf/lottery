import {test,expect,type APIResponse,type Page} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {rememberAdminSession} from './support/admin-session';

const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const admin=`${origin}/api/v1/admin`;
const brand='0199a000-0000-7000-8000-000000000001';
const fixture=process.env.COMMISSION_FIXTURE_BIN;
test.describe.configure({mode:'serial'});
type Cycle={id:string;state:string;version:number;current_run_id:string|null;current_generation:string|null;total_points:string;reason:string};
type Discovery={id:string;state:string;version:number;cycle_id:string|null};
type Run={id:string;generation:string;state:string;calculated_count:string};
async function data<T>(response:Pick<APIResponse,'text'|'status'>,status=200):Promise<T>{const body=await response.text();expect(response.status(),body).toBe(status);const envelope=JSON.parse(body);expect(envelope.success).toBe(true);return envelope.data as T;}
function command(name:'advance'|'discover'|'verify'){
  const value=execFileSync(fixture!,[name],{env:process.env,encoding:'utf8',timeout:30_000});
  return JSON.parse(value) as Record<string,unknown>;
}
async function navigate(page:Page,project:string,destination:string){
  if(project==='mobile'){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:new RegExp(destination)}).click();}
  else await page.locator('.side-nav').getByRole('button',{name:new RegExp(destination)}).click();
  return page.locator('.commission-cycles');
}
async function confirm(page:Page){
  const dialog=page.getByRole('dialog',{name:/确认佣金周期操作|Confirm commission cycle operation/});
  await expect(dialog).toBeVisible();
  await dialog.getByRole('checkbox').check();
  await dialog.getByRole('button',{name:/^确认并提交$|^Confirm and submit$/}).click();
}

test('real commission cycles retain unknown requests, retry real failures and preserve financial evidence',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD,'Provide explicitly owned commission workflow fixture');
  test.setTimeout(90_000);page.setDefaultTimeout(10_000);
  const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
  const before=command('verify');
  expect(before.commission_ledger_entries).toBe(0);expect(before.commission_wallet_points).toBe(0);expect(before.economic_fingerprint).toMatch(/^[0-9a-f]{64}$/);
  const username='commission_admin';
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:username,password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  rememberAdminSession(username,await context.cookies(`${admin}/me`),origin);
  const get=async<T>(path:string)=>data<T>(await page.request.get(`${admin}${path}`,{headers:{'X-Brand-ID':brand}}));
  const all=await get<{items:Cycle[]}>('/commission-cycles?limit=100&offset=0');
  const ready=all.items.find(c=>c.state==='ready')!,failed=all.items.find(c=>c.state==='failed')!;
  const queued=await get<{items:Discovery[]}>('/commission-discovery?limit=100&offset=0');
  const discovery=queued.items.find(d=>d.state==='failed')!;
  expect(ready).toBeTruthy();expect(failed).toBeTruthy();expect(discovery).toBeTruthy();expect(ready.total_points).toBe('1');
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
  let panel=await navigate(page,info.project.name,'佣金和奖励|Commissions.*rewards');
  await expect(panel.getByRole('heading',{name:'佣金周期核算',exact:true})).toBeVisible();
  await panel.locator(`[data-cycle-id="${ready.id}"]`).click();
  await expect(panel.locator('.detail')).toContainText('核算就绪');
  await expect(panel.locator('.data-row')).toContainText('3/5');
  await expect(panel.locator('.calc-row')).toHaveCount(2);
  await expect(panel.getByRole('button',{name:/批准佣金|派发积分|Approve commission|Pay out points/})).toHaveCount(0);

  const reason=`Owned commission registration ${info.project.name}`;
  await panel.getByLabel('锚点注单 UUID',{exact:true}).fill(discovery.id);
  await panel.getByLabel('登记原因',{exact:true}).fill(reason);
  const writes:Array<{body:string|null;key:string|undefined}>=[];
  let createdId='';
  await page.route('**/api/v1/admin/commission-cycles',async route=>{
    if(route.request().method()!=='POST'){await route.continue();return;}
    writes.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
    const response=await route.fetch();expect(response.status(),await response.text()).toBe(201);
    const receipt=(await response.json()).data as Cycle;
    if(writes.length===1){createdId=receipt.id;command('advance');await route.abort('failed');}
    else{expect(writes[1]).toEqual(writes[0]);await route.fulfill({response});}
  });
  await panel.getByRole('button',{name:'检查并确认登记',exact:true}).click();await confirm(page);
  await expect(panel.getByRole('heading',{name:'待恢复的原请求',exact:true})).toBeVisible();
  expect(writes).toHaveLength(1);
  const current=await get<Cycle>(`/commission-cycles/${createdId}`);expect(current.state).toBe('ready');expect(current.version).toBeGreaterThan(1);
  await panel.getByRole('button',{name:'刷新',exact:true}).click();
  await expect(panel.getByRole('heading',{name:'待恢复的原请求',exact:true})).toBeVisible();
  await navigate(page,info.project.name,'代理树|Agent tree');
  panel=await navigate(page,info.project.name,'佣金和奖励|Commissions.*rewards');
  await expect(panel.getByRole('button',{name:'检查并恢复原请求',exact:true})).toBeVisible();
  await panel.getByRole('button',{name:'检查并恢复原请求',exact:true}).click();await confirm(page);
  await expect(panel.locator('.commission-receipt')).toContainText('登记中 · v1');
  await expect(panel.getByRole('heading',{name:'待恢复的原请求',exact:true})).toHaveCount(0);
  expect(writes).toHaveLength(2);
  const once=await get<{items:Cycle[]}>('/commission-cycles?limit=100&offset=0');expect(once.items.filter(c=>c.reason===reason)).toHaveLength(1);

  await panel.locator(`[data-cycle-id="${failed.id}"]`).click();
  const cycleRetry=panel.locator('.detail .retry-box');
  await cycleRetry.getByLabel('重试原因',{exact:true}).fill('Recover genuine calculation failure');
  await cycleRetry.getByRole('button',{name:'检查并确认重试',exact:true}).click();
  const retryCommitted=page.waitForResponse(r=>r.request().method()==='POST'&&r.url()===`${admin}/commission-cycles/${failed.id}/retry`);
  await confirm(page);await data<Cycle>(await retryCommitted);
  command('advance');await panel.getByRole('button',{name:'刷新',exact:true}).click();
  await expect(panel.locator('.detail')).toContainText('核算就绪');
  const corrected=await get<Cycle>(`/commission-cycles/${failed.id}`);expect(corrected.state).toBe('ready');expect(corrected.current_generation).toBe('2');
  const runs=await get<{items:Run[]}>(`/commission-cycles/${failed.id}/runs?limit=100&offset=0`);
  expect(runs.items).toHaveLength(2);const old=runs.items.find(r=>r.state==='abandoned')!;
  await panel.locator(`[data-run-id="${old.id}"]`).click();
  await expect(panel.locator('.calculations h4')).toContainText(old.id);await expect(panel.locator('.calc-row')).toHaveCount(0);

  await panel.locator(`[data-discovery-id="${discovery.id}"] .discovery-select`).click();
  const discoveryRetry=panel.locator('.discoveries .retry-box');
  await discoveryRetry.getByLabel('重试原因',{exact:true}).fill('Link real discovered order after storage recovery');
  await discoveryRetry.getByRole('button',{name:'检查并确认重试发现记录',exact:true}).click();
  // The fixture worker only processes committed pending records. A click does
  // not wait for the async HTTP transaction; running it immediately raced the
  // retry commit on CI and legitimately found no eligible work.
  const discoveryRetryCommitted=page.waitForResponse(r=>r.request().method()==='POST'&&r.url()===`${admin}/commission-discovery/${discovery.id}/retry`);
  await confirm(page);await data<Discovery>(await discoveryRetryCommitted);
  command('discover');await panel.getByRole('button',{name:'刷新',exact:true}).click();
  await expect(panel.locator(`[data-discovery-id="${discovery.id}"]`)).toContainText('已登记');
  const linked=(await get<{items:Discovery[]}>('/commission-discovery?limit=100&offset=0')).items.find(d=>d.id===discovery.id)!;
  expect(linked.cycle_id).toBe(createdId);
  expect(command('verify').economic_fingerprint).toBe(before.economic_fingerprint);
  expect(command('verify').commission_ledger_entries).toBe(0);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath('commission-cycles-panel.png')});
  await page.screenshot({path:info.outputPath('commission-cycles-viewport.png')});
  await page.getByTestId('admin-language').selectOption('en');
  await expect(panel.getByRole('heading',{name:'Commission cycle calculations',exact:true})).toBeVisible();
  await expect(panel).toContainText('Calculated');expect(writes).toHaveLength(2);
  expect(errors).toEqual([]);
});

test('current commission management renders exact completed evidence without financial writes',async({page,context},info)=>{
  test.skip(!fixture||!process.env.COMMISSION_FIXTURE_CONFIRM||!process.env.TEST_COMMISSION_ADMIN_PASSWORD,'Provide explicitly owned commission workflow fixture');
  const before=command('verify');
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'commission_admin',password:process.env.TEST_COMMISSION_ADMIN_PASSWORD}}));
  rememberAdminSession('commission_admin',await context.cookies(`${admin}/me`),origin);
  const writes:string[]=[];page.on('request',r=>{if(r.url().startsWith(admin)&&r.method()!=='GET')writes.push(r.url());});
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select an administrative brand/,{exact:true}).selectOption(brand);
  const panel=await navigate(page,info.project.name,'佣金和奖励|Commissions.*rewards');
  await expect(panel.getByRole('heading',{name:'佣金周期核算',exact:true})).toBeVisible();
  const cycles=await data<{items:Cycle[]}>(await page.request.get(`${admin}/commission-cycles?limit=100&offset=0`,{headers:{'X-Brand-ID':brand}}));
  const cycle=cycles.items.find(x=>x.state==='ready'&&x.total_points==='1')!;
  expect(cycle).toBeTruthy();await panel.locator(`[data-cycle-id="${cycle.id}"]`).click();
  await expect(panel.locator('.data-row')).toContainText('3/5');
  await expect(panel.locator('.calc-row')).toHaveCount(2);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath('commission-cycles-current-panel.png')});
  await page.screenshot({path:info.outputPath('commission-cycles-current-viewport.png')});
  await page.getByTestId('admin-language').selectOption('en');
  await expect(panel).toContainText('Calculated');
  expect(writes).toEqual([]);expect(command('verify').economic_fingerprint).toBe(before.economic_fingerprint);
});
