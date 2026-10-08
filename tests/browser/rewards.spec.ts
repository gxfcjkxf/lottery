import {test,expect,type APIResponse,type Page} from '@playwright/test';
import {rememberAdminSession} from './support/admin-session';

const origin=process.env.TEST_ADMIN_ORIGIN??'http://localhost:5174';
const admin=`${origin}/api/v1/admin`,brand='0199a000-0000-7000-8000-000000000001';
type Order={id:string;member_id:string;state:string;version:number;grant_ledger_entry_id:string;revoke_ledger_entry_id:string|null};
type Buckets=Record<string,Record<string,string>>;
type Wallet={by_source:Buckets};
type Entry={id:string;entry_type:string;reference_id:string;reversal_of?:string;before_snapshot:Buckets;delta_snapshot:Buckets;after_snapshot:Buckets};
async function data<T>(response:Pick<APIResponse,'text'|'status'>,status=200):Promise<T>{
  const body=await response.text();expect(response.status(),body).toBe(status);
  const envelope=JSON.parse(body);expect(envelope.success).toBe(true);return envelope.data as T;
}
async function navigate(page:Page,project:string,destination:string){
  if(project==='mobile'&&destination==='工作台|Dashboard')await page.locator('.mobile-nav').getByRole('button',{name:new RegExp(destination)}).click();
  else if(project==='mobile'){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:new RegExp(destination)}).click();}
  else await page.locator('.side-nav').getByRole('button',{name:new RegExp(destination)}).click();
}
async function confirm(page:Page){
  const dialog=page.getByTestId('rewards-review');await expect(dialog).toBeVisible();
  await expect(dialog.getByTestId('rewards-confirm-submit')).toBeDisabled();
  await dialog.getByTestId('rewards-review-confirmed').check();await dialog.getByTestId('rewards-confirm-submit').click();
}

test('real manual rewards recover original receipts without duplicate money and wait for explicit full revocation',async({page,context},info)=>{
  test.skip(process.env.REWARD_UI_FIXTURE_CONFIRM!=='owned_synthetic_database'||!process.env.TEST_REWARD_ADMIN_PASSWORD||!process.env.TEST_REWARD_SUPER_PASSWORD,'Requires a dedicated owned reward database and normal bootstrap accounts');
  test.setTimeout(90_000);
  expect(process.env.REWARD_UI_VIEWPORT).toBe(info.project.name);
  const errors:string[]=[];page.on('pageerror',error=>errors.push(error.message));
  const get=async<T>(path:string)=>data<T>(await page.request.get(admin+path,{headers:{'X-Brand-ID':brand}}));
  const post=async<T>(path:string,body:unknown,extra:Record<string,string>={})=>data<T>(await page.request.post(admin+path,{headers:{Origin:origin,'X-Brand-ID':brand,'Idempotency-Key':crypto.randomUUID(),...extra},data:body}));
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'reward_s22_operator',password:process.env.TEST_REWARD_ADMIN_PASSWORD}}));
  const {account}=await get<{account:{id:string;permissions_by_brand:Record<string,string[]>}}>('/me');
  expect(account.permissions_by_brand[brand]).toEqual(expect.arrayContaining(['reward.view.brand','reward.grant.brand','reward.revoke.brand','reward.retry.brand']));
  rememberAdminSession('reward_s22_operator',await context.cookies(`${admin}/me`),origin,account.id);
  expect((await get<{items:Order[]}>('/reward-orders?limit=100&offset=0')).items).toEqual([]);
  // The member and all balances are created by normal APIs, never by direct SQL.
  const member=await data<{member_id:string}>(await page.request.post(`${admin}/users`,{headers:{Origin:origin,'X-Brand-ID':brand,'Idempotency-Key':crypto.randomUUID()},data:{username:`rw_${info.project.name}_${crypto.randomUUID().replaceAll('-','').slice(0,8)}`,password:'owned-reward-member-password-2026',display_name:'Owned synthetic reward member',reason:'Create explicitly owned reward browser member'}}),201);
  const memberID=member.member_id;
  const wallet=()=>get<Wallet>(`/wallets/${memberID}`);
  const ledger=()=>get<{items:Entry[]}>(`/wallets/${memberID}/ledger?limit=100&offset=0`);
  const economic=async()=>JSON.stringify({wallet:await wallet(),ledger:await ledger()});
  await page.goto(origin);await page.getByLabel(/选择真实后台品牌|Select live admin brand/,{exact:true}).selectOption(brand);
  await navigate(page,info.project.name,'人工奖励|Manual rewards');let panel=page.locator('.rewards-management');
  await expect(panel.getByRole('heading',{name:'人工奖励管理',exact:true})).toBeVisible();
  const fillGrant=async(amount:string,reason:string)=>{
    await panel.getByTestId('rewards-grant-member').fill(memberID);await panel.getByTestId('rewards-grant-points').fill(amount);await panel.getByTestId('rewards-grant-reason').fill(reason);await panel.getByTestId('rewards-grant-review').click();
  };
  type Write={body:string|null;key:string|undefined;actor:string|undefined};
  const grantWrites:Write[]=[];let first!:Order;
  await page.route('**/api/v1/admin/reward-orders',async route=>{
    if(route.request().method()!=='POST')return route.continue();
    grantWrites.push({body:route.request().postData(),key:route.request().headers()['idempotency-key'],actor:route.request().headers()['x-reward-actor-id']});
    const response=await route.fetch();const result=await data<Order>(response,201);
    expect(grantWrites.at(-1)?.actor).toBe(account.id);
    if(grantWrites.length===1){first=result;await route.abort('failed');}
    else{expect(result).toEqual(first);expect(grantWrites[1]).toEqual(grantWrites[0]);await route.fulfill({response});}
  });
  await fillGrant('100','Lost response after a genuine committed manual reward');await confirm(page);
  await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();
  expect((await wallet()).by_source.gift.available).toBe('100');
  const corrected=await post<Order>(`/reward-orders/${first.id}/revoke`,{version:1,reason:'Explicit synthetic operator reversal while original grant response is unresolved'},{'X-Reward-Actor-ID':account.id});
  expect(corrected.state).toBe('revoked');expect(corrected.version).toBe(2);
  const afterFirst=await economic();
  await panel.getByTestId('rewards-list-refresh').click();await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();
  await navigate(page,info.project.name,'工作台|Dashboard');await navigate(page,info.project.name,'人工奖励|Manual rewards');panel=page.locator('.rewards-management');
  await panel.getByRole('button',{name:'使用原请求重试',exact:true}).click();await confirm(page);
  await expect(panel.getByTestId('rewards-receipt')).toContainText('已发放 · v1');await expect(panel.locator('.rw-detail')).toContainText('已撤销 · v2');
  expect(grantWrites).toHaveLength(2);expect(await economic()).toBe(afterFirst);
  await page.unroute('**/api/v1/admin/reward-orders');

  const created=page.waitForResponse(r=>r.request().method()==='POST'&&r.url()===`${admin}/reward-orders`);
  await fillGrant('40','Full reversal must never consume frozen or other-source points');await confirm(page);const second=await data<Order>(await created,201);
  await expect(panel.locator('.rw-detail')).toContainText(second.id);
  // Freeze while gift is the ONLY available source, then fund each other source
  // by explicit manual adjustment. No commission business event is fabricated.
  const frozen=await post<Entry>(`/wallets/${memberID}/freeze`,{points:'40',reason:'Owned synthetic manual gift hold'});
  for(const source of ['recharge','winning','commission'])await post(`/wallets/${memberID}/adjust`,{source,delta:'5',reason:`Owned synthetic ${source} safety balance`});
  const beforePending=await economic();const beforePendingLedger=await ledger();
  await panel.getByTestId('rewards-action-reason').fill('Request full reversal without consuming held gift or other sources');
  await panel.getByTestId('rewards-revoke').click();await confirm(page);
  await expect(panel.locator('.rw-detail')).toContainText('撤销待处理 · v2');
  await expect(panel.getByTestId('rewards-receipt')).toContainText('撤销待处理 · v2');
  expect(await economic()).toBe(beforePending);expect(await ledger()).toEqual(beforePendingLedger);
  await post(`/wallets/${memberID}/unfreeze`,{entry_id:frozen.id,reason:'Explicit owned operator releases original full hold'});
  expect((await wallet()).by_source.gift.available).toBe('40');
  const afterUnfreeze=await economic();await panel.getByTestId('rewards-detail-refresh').click();
  await expect(panel.locator('.rw-detail')).toContainText('撤销待处理 · v2');expect(await economic()).toBe(afterUnfreeze);
  const retryWrites:Write[]=[];
  await page.route(`**/api/v1/admin/reward-orders/${second.id}/retry-revocation`,async route=>{
    retryWrites.push({body:route.request().postData(),key:route.request().headers()['idempotency-key'],actor:route.request().headers()['x-reward-actor-id']});
    const response=await route.fetch();const result=await data<Order>(response);expect(result.state).toBe('revoked');expect(result.version).toBe(3);
    if(retryWrites.length===1)await route.abort('failed');else{expect(retryWrites[1]).toEqual(retryWrites[0]);await route.fulfill({response});}
  });
  await panel.getByTestId('rewards-action-reason').fill('Explicitly continue pending full reversal after operator released original hold');
  await panel.getByTestId('rewards-retry').click();await confirm(page);
  await expect(panel.getByRole('button',{name:'使用原请求重试',exact:true})).toBeVisible();const finalEconomic=await economic();
  await panel.getByTestId('rewards-detail-refresh').click();await expect(panel.locator('.rw-detail')).toContainText('已撤销 · v3');
  await navigate(page,info.project.name,'工作台|Dashboard');await navigate(page,info.project.name,'人工奖励|Manual rewards');panel=page.locator('.rewards-management');
  await panel.getByRole('button',{name:'使用原请求重试',exact:true}).click();await confirm(page);
  await expect(panel.getByTestId('rewards-receipt')).toContainText('已撤销 · v3');await expect(panel.locator('.rw-detail')).toContainText('已撤销 · v3');
  expect(retryWrites).toHaveLength(2);expect(await economic()).toBe(finalEconomic);
  const actions=await get<{items:Array<{version:number;ledger_entry_id:string|null}>}>(`/reward-orders/${second.id}/actions?limit=20&offset=0`);
  expect(actions.items.map(action=>action.version).sort()).toEqual([1,2,3]);expect(actions.items.find(action=>action.version===2)?.ledger_entry_id).toBeNull();
  const rewardEntries=(await ledger()).items.filter(entry=>entry.reference_id===second.id);
  expect(rewardEntries).toHaveLength(2);expect(rewardEntries.find(entry=>entry.entry_type==='reward_reversal')?.reversal_of).toBe(second.grant_ledger_entry_id);
  const balances=(await wallet()).by_source;
  expect(balances.gift).toEqual({available:'0',manual_frozen:'0',system_frozen:'0',withdrawal:'0'});
  for(const source of ['recharge','winning','commission'])expect(balances[source]).toEqual({available:'5',manual_frozen:'0',system_frozen:'0',withdrawal:'0'});
  for(const entry of rewardEntries)for(const source of ['recharge','winning','commission'])expect(entry.before_snapshot[source]).toEqual(entry.after_snapshot[source]);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath('manual-rewards-panel.png')});
  const writes:string[]=[];page.on('request',request=>{if(request.url().includes('/reward-orders')&&request.method()!=='GET')writes.push(request.url());});
  await page.getByTestId('admin-language').selectOption('en');await expect(panel.getByRole('heading',{name:'Manual rewards',exact:true})).toBeVisible();
  await expect(panel.getByTestId('rewards-receipt')).toContainText('Revoked · v3');expect(writes).toEqual([]);
  const loggedOut=page.waitForResponse(response=>response.request().method()==='POST'&&response.url()===`${admin}/auth/logout`);
  await page.getByRole('button',{name:'Sign out',exact:true}).first().click();await data(await loggedOut);
  await data(await page.request.post(`${admin}/auth/login`,{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:'reward_s22_reader',password:process.env.TEST_REWARD_SUPER_PASSWORD}}));
  await page.reload();await page.getByLabel(/选择真实后台品牌|Select live admin brand/,{exact:true}).selectOption(brand);
  await navigate(page,info.project.name,'人工奖励|Manual rewards');panel=page.locator('.rewards-management');
  await expect(panel.getByRole('heading',{name:'Manual rewards',exact:true})).toBeVisible();await expect(panel.getByTestId('rewards-receipt')).toHaveCount(0);
  await expect(panel.getByTestId('rewards-grant-review')).toBeDisabled();
  await panel.getByRole('button',{name:second.id,exact:true}).click();await expect(panel.locator('.rw-detail')).toContainText('Revoked · v3');
  expect(await economic()).toBe(finalEconomic);expect(writes).toEqual([]);expect(errors).toEqual([]);
  await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await panel.screenshot({path:info.outputPath('manual-rewards-readonly-panel.png')});
});
