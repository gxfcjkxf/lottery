import { test, expect } from '@playwright/test';
import { rememberAdminSession } from './support/admin-session';

const origin=process.env.TEST_RECONCILIATION_ORIGIN??'http://localhost:5174';
const brand='0199a000-0000-7000-8000-000000000002';
const username=process.env.TEST_RECONCILIATION_ADMIN_USERNAME;
const password=process.env.TEST_RECONCILIATION_ADMIN_PASSWORD;
const endpoint='/api/v1/admin/reconciliations/business-inventory';

test('brand inventory finds a parentless child without changing funds or automatically checking on mount',async({page,context},info)=>{
 test.skip(!username||!password,'Provide owned reconciliation administrator credentials');
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 const login=await page.request.post(origin+'/api/v1/admin/auth/login',{headers:{Origin:origin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:username,password}});
 expect(login.status(),await login.text()).toBe(200);rememberAdminSession(username!,await context.cookies(origin+'/api/v1/admin/me'));
 let reads=0;await page.route('**'+endpoint,route=>{reads++;return route.continue()});
 let writes=0;page.on('request',request=>{if(request.method()!=='GET'&&request.url().includes('/api/v1/admin/'))writes++});
 await page.goto(origin);
 await page.getByTestId('admin-language').selectOption('zh-CN');
 await page.getByLabel('选择真实后台品牌',{exact:true}).selectOption(brand);
 if(info.project.name==='mobile'){await page.locator('.mobile-nav button').nth(4).click();await page.locator('.mobile-more-menu').getByRole('button',{name:/批量对账/}).click()}
 else await page.locator('.side-nav').getByRole('button',{name:/批量对账/}).click();
 const panel=page.locator('.inventory');
 await expect(panel.getByRole('heading',{name:'品牌业务引用检查',exact:true})).toBeVisible();
 await expect(panel).toContainText('本页面不会自动执行检查');expect(reads).toBe(0);
 const live=await page.request.get(origin+endpoint,{headers:{'X-Brand-ID':brand}});
 expect(live.status(),await live.text()).toBe(200);const {data}=await live.json();
 expect(data.schema_version).toBe(1);expect(data.coverage).toHaveLength(41);
 expect(BigInt(data.source_row_count)).toBeGreaterThan(0n);expect(data.consistent).toBe(false);
 expect(data.issues).toEqual(expect.arrayContaining([
  expect.objectContaining({source_table:'reward_order_actions',parent_table:'reward_orders',code:'MISSING_PARENT_REFERENCE'}),
  expect.objectContaining({source_table:'reward_order_actions',reference_key:'ledger_entry_id',code:'MISSING_REQUIRED_LEDGER_REFERENCE'}),
 ]));
 await panel.getByRole('button',{name:'加载观察',exact:true}).click();
 await expect(panel.locator('.issue-row')).toHaveCount(data.issues.length);
 expect(reads).toBe(1);
 await expect(panel).toContainText('不是完整钱包或财务证明');
 await expect(panel.locator('.inventory-stats b').nth(0)).toHaveText(data.source_row_count);
 await expect(panel.locator('.inventory-stats b').nth(1)).toHaveText(data.reference_count);
 await expect(panel.locator('.inventory-stats b').nth(2)).toHaveText(data.issue_count);
 for(let i=0;i<data.issues.length;i++){await expect(panel.locator('.issue-row').nth(i)).toContainText(data.issues[i].source_id);await expect(panel.locator('.issue-row').nth(i)).toContainText(data.issues[i].reference_key)}
 const after=await page.request.get(origin+endpoint,{headers:{'X-Brand-ID':brand}});expect(after.status()).toBe(200);
 expect((await after.json()).data.fingerprint).toBe(data.fingerprint);
 await page.getByTestId('admin-language').selectOption('en');
 await expect(panel.getByRole('heading',{name:'Brand business reference inventory',exact:true})).toBeVisible();
 await expect(panel).toContainText('not a complete wallet or financial proof');
 await panel.screenshot({path:info.outputPath('inventory-live.png')});
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
 await page.unroute('**'+endpoint);
 await page.route('**'+endpoint,route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({success:false,error:{code:'BUSINESS_INVENTORY_UNAVAILABLE',message:'owned failure'}})}));
 await panel.getByRole('button',{name:'Load a fresh observation',exact:true}).click();
 await expect(panel.getByRole('alert')).toContainText('Could not load');await expect(panel.locator('.issue-row')).toHaveCount(0);await expect(panel.locator('.inventory-stats')).toHaveCount(0);
 await page.unroute('**'+endpoint);
 await panel.getByRole('button',{name:'Load observation',exact:true}).click();await expect(panel.locator('.issue-row')).toHaveCount(data.issues.length);
 expect(writes).toBe(0);expect(errors).toEqual([]);
});
