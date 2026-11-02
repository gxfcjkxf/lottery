import { selectControl } from '../select-control'
import {selectLanguage} from '../language-switcher';
import {expect,test,type APIResponse,type Page} from '@playwright/test';
import type {ArchiveSchedule,ArchiveNextRuns} from '../../shared/src/archive-schedule';
const origin=process.env.TEST_GLOBAL_ARCHIVE_ORIGIN,brandOrigin=process.env.TEST_BRAND_ARCHIVE_ORIGIN,brand='0199a000-0000-7000-8000-000000000002';
type Policy=ArchiveSchedule & {version:number;daily_enabled:boolean;monthly_enabled:boolean;timezone:string;next_runs:ArchiveNextRuns};
async function data<T>(response:Pick<APIResponse,'text'|'status'>,status=200):Promise<T>{const raw=await response.text();expect(response.status(),raw).toBe(status);return JSON.parse(raw).data;}
async function openOperations(page:Page,name:string){
 const menu=page.locator('button[aria-controls="platform-navigation"]');if(await menu.isVisible()&&await menu.getAttribute('aria-expanded')==='false')await menu.click();
 const group=page.locator('#platform-navigation').getByRole('button',{name:'Operations',exact:true});
 if(await group.getAttribute('aria-expanded')==='false')await group.click();
 await page.getByTestId('platform-operations-tabs').getByRole('button',{name,exact:true}).click();
}
test('global archive configuration belongs to the platform and brands only inherit execution rules',async({page,context},info)=>{
 expect(process.env.GLOBAL_ARCHIVE_FIXTURE_CONFIRM).toBe('owned_synthetic_database');expect(origin).toMatch(/^http:\/\/127\.0\.0\.1:\d+$/);expect(brandOrigin).toBeTruthy();
 const platform=`${origin}/api/v1/platform`,admin=`${brandOrigin}/api/v1/admin`;
 const login=await page.request.post(`${platform}/auth/login`,{headers:{Origin:origin!,'Idempotency-Key':crypto.randomUUID()},data:{identifier:process.env.TEST_GLOBAL_ARCHIVE_USERNAME,password:process.env.TEST_GLOBAL_ARCHIVE_PASSWORD}});await data(login);
 const me=await data<{account:{id:string}}>(await page.request.get(`${platform}/me`));
 const brandPage=await context.newPage();await data(await brandPage.request.post(`${admin}/auth/login`,{headers:{Origin:brandOrigin!,'Idempotency-Key':crypto.randomUUID()},data:{identifier:process.env.TEST_BRAND_ARCHIVE_USERNAME,password:process.env.TEST_GLOBAL_ARCHIVE_PASSWORD}}));
 const staff=await data<{account:{id:string}}>(await brandPage.request.get(`${admin}/me`,{headers:{'X-Brand-ID':brand}}));
 const captured=await brandPage.request.post(`${admin}/report-archives`,{headers:{Origin:brandOrigin!,'X-Brand-ID':brand,'Idempotency-Key':crypto.randomUUID(),'X-Report-Archive-Actor-ID':staff.account.id},data:{kind:'monthly',period_key:'2020-01',expected_revision:0}});await data(captured,201);
 await brandPage.route('**/api/v1/context',async route=>{const response=await route.fetch({headers:{...route.request().headers(),Host:'harbor.localhost'}});await route.fulfill({response});});
 await brandPage.goto(brandOrigin!);await selectLanguage(brandPage,'en');await expect(brandPage.getByRole('button',{name:'Archive configuration',exact:true})).toHaveCount(0);await expect(brandPage.getByRole('button',{name:'Archive records',exact:true})).toHaveCount(0);
 const economics=async()=>{
  const response=await brandPage.request.get(`${admin}/reports/ledger?from=2020-01-01T00%3A00%3A00Z&to=2020-02-01T00%3A00%3A00Z&group_by=day`,{headers:{'X-Brand-ID':brand}});
  const report=await data<{summary:unknown}>(response);return report.summary;
 };
 const before=await economics();
 const writes:Array<{body:string|null;key:string|undefined}>=[];let original:Policy|undefined;
 await page.route('**/api/v1/platform/report-archive-policy',async route=>{if(route.request().method()!=='PUT'){await route.continue();return;}expect(route.request().headers()['x-brand-id']).toBeUndefined();writes.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});const response=await route.fetch();const accepted=await data<Policy>(response);if(writes.length===1){original=accepted;await route.abort('failed');return;}expect(accepted).toEqual(original);await route.fulfill({response});});
 await page.goto(origin!);await selectLanguage(page,'en');await openOperations(page,'Archive configuration');const panel=page.getByTestId('platform-archive-policy');await expect(panel).toBeVisible();await expect(page.locator('.topbar-brand-picker')).toHaveCount(0);
 await panel.getByTestId('archive-policy-daily').check();await panel.getByTestId('archive-policy-monthly').check();await selectControl(panel.getByTestId('archive-policy-daily-time').locator(".select-control-trigger").nth(0), '07');await selectControl(panel.getByTestId('archive-policy-daily-time').locator(".select-control-trigger").nth(1), '30');await panel.getByTestId('archive-policy-daily-offset').fill('2');await panel.getByTestId('archive-policy-monthly-day').fill('5');await selectControl(panel.getByTestId('archive-policy-monthly-time').locator(".select-control-trigger").nth(0), '06');await selectControl(panel.getByTestId('archive-policy-monthly-time').locator(".select-control-trigger").nth(1), '00');await panel.getByTestId('archive-policy-monthly-offset').fill('2');
 await panel.getByRole('button',{name:'Save configuration',exact:true}).click();await expect.poll(()=>Boolean(original)).toBe(true);await expect(panel.getByRole('button',{name:'Retry original request',exact:true})).toBeVisible();
 const input=JSON.parse(writes[0]!.body!);await data(await page.request.put(`${platform}/report-archive-policy`,{headers:{Origin:origin!,'Idempotency-Key':crypto.randomUUID(),'X-Report-Archive-Actor-ID':me.account.id},data:{...input,version:original!.version,daily_enabled:false,monthly_enabled:false}}));
 await page.locator('.toast-error').getByRole('button',{name:'Close',exact:true}).click();await panel.getByRole('button',{name:'Retry original request',exact:true}).click();await expect(panel.getByTestId('archive-policy-monthly')).not.toBeChecked();expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);
 await page.unroute('**/api/v1/platform/report-archive-policy');await panel.getByTestId('archive-policy-monthly').check();await panel.getByTestId('archive-policy-daily').check();await panel.getByRole('button',{name:'Save configuration',exact:true}).click();await expect(panel.getByRole('button',{name:'Save configuration',exact:true})).toBeDisabled();
 const current=await data<Policy>(await page.request.get(`${platform}/report-archive-policy`));expect(current.monthly_period_offset).toBe(2);expect(current.monthly_execution_day).toBe(5);expect(current.next_runs.monthly).toBeTruthy();await expect(panel).toContainText(current.next_runs.monthly!.period_key);
 expect(await economics()).toEqual(before);await brandPage.close();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:`.local/archive-global/config-${info.project.name}.png`,fullPage:true});
 await openOperations(page,'Archive records');await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), brand);await page.getByRole('tab',{name:'Monthly archives',exact:true}).click();const records=page.getByTestId('platform-archive-list');await expect(records).toContainText('2020-01');await expect(records).toContainText('Archived');await records.getByRole('button',{name:'2020-01',exact:true}).click();const detail=page.getByRole('dialog',{name:'2020-01',exact:true});await expect(detail).toBeVisible();expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:`.local/archive-global/record-${info.project.name}.png`,fullPage:true});
});
