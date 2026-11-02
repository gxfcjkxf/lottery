import { selectControl } from '../select-control'
import { selectLanguage } from '../language-switcher';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import { expect,test,type APIResponse,type Page } from '@playwright/test';
const origin=process.env.TEST_ARCHIVE_ORIGIN;
const brand='0199a000-0000-7000-8000-000000000002';
async function data<T>(response:Pick<APIResponse,'text'|'status'>,status=200):Promise<T>{const raw=await response.text();expect(response.status(),raw).toBe(status);const envelope=JSON.parse(raw);expect(envelope.success).toBe(true);return envelope.data;}
async function navigate(page:Page){const toggle=page.locator('button[aria-controls="platform-navigation"]');if(await toggle.isVisible()&&await toggle.getAttribute('aria-expanded')==='false')await toggle.click();await page.locator('.sidebar').getByRole('button',{name:'Operations',exact:true}).click();await page.getByTestId('platform-operations-tabs').getByRole('button',{name:'Archive records',exact:true}).click();await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), brand);}
test('archive records separate daily and monthly records and open immutable details',async({page},info)=>{
 expect(process.env.REPORT_ARCHIVE_FIXTURE_CONFIRM).toBe('owned_synthetic_database');expect(process.env.REPORT_ARCHIVE_VIEWPORT).toBe(info.project.name);expect(process.env.TEST_ARCHIVE_ADMIN_USERNAME).toBeTruthy();expect(origin).toMatch(/^http:\/\/(?:localhost|127\.0\.0\.1):\d+$/);
 const api=`${origin}/api/v1/admin`,headers={'X-Brand-ID':brand};
 await data(await page.request.post(`${api}/auth/login`,{headers:{Origin:origin!,'Idempotency-Key':crypto.randomUUID()},data:{identifier:process.env.TEST_ARCHIVE_ADMIN_USERNAME,password:process.env.TEST_ARCHIVE_ADMIN_PASSWORD}}));
 const me=await data<{account:{id:string}}>(await page.request.get(`${api}/me`,{headers}));
 type Record={id:string;revision:number;payload_sha256:string;snapshot:{ledger:{entry_count:string}}};
 const create=(kind:string,period:string,revision:number,key:string)=>page.request.post(`${api}/report-archives`,{headers:{...headers,Origin:origin!,'Idempotency-Key':key,'X-Report-Archive-Actor-ID':me.account.id},data:{kind,period_key:period,expected_revision:revision}}).then(r=>data<Record>(r,201));
 const key=crypto.randomUUID(),first=await create('daily','2020-01-01',0,key);
 const second=await create('daily','2020-01-01',1,crypto.randomUUID());await create('monthly','2020-01',0,crypto.randomUUID());
 expect(await create('daily','2020-01-01',0,key)).toEqual(first);expect(first.snapshot.ledger.entry_count).toBe('0');
 const platformOrigin=process.env.TEST_ARCHIVE_PLATFORM_ORIGIN!;expect(platformOrigin).toBeTruthy();
 await data(await page.request.post(`${platformOrigin}/api/v1/platform/auth/login`,{headers:{Origin:platformOrigin,'Idempotency-Key':crypto.randomUUID()},data:{identifier:process.env.TEST_ARCHIVE_PLATFORM_ADMIN_USERNAME,password:process.env.TEST_ARCHIVE_ADMIN_PASSWORD}}));
 let writes=0;const queries:string[]=[];page.on('request',request=>{if(request.url().includes('/api/v1/platform/report-archives')){if(request.method()!=='GET')writes++;else queries.push(request.url());}});
 await page.goto(platformOrigin);await selectLanguage(page,'en');await navigate(page);
 const list=page.getByTestId('platform-archive-list');await expect(list.getByRole('button',{name:'2020-01-01',exact:true})).toHaveCount(2);await expect(list).not.toContainText(first.id);
 await list.getByRole('button',{name:'2020-01-01',exact:true}).first().click();
 const detail=page.getByTestId('platform-archive-detail');await expect(detail).toContainText('Business overview');
 const saving=page.waitForEvent('download');await detail.getByRole('button',{name:/Download/}).click();const file=await saving;const bytes=await readFile((await file.path())!);expect(createHash('sha256').update(bytes).digest('hex')).toBe(second.payload_sha256);
 await page.getByRole('dialog').getByRole('button',{name:'Close',exact:true}).click();await expect(detail).toHaveCount(0);
 await page.getByRole('tab',{name:'Monthly archives',exact:true}).click();await expect(list.getByRole('button',{name:'2020-01',exact:true})).toHaveCount(1);expect(queries.some(url=>new URL(url).searchParams.get('kind')==='monthly')).toBe(true);
 await list.getByRole('button',{name:'2020-01',exact:true}).click();await expect(detail).toContainText('Business overview');await expect(page.getByRole('dialog',{name:'2020-01',exact:true})).toBeVisible();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
 await page.getByRole('dialog').getByRole('button',{name:'Close',exact:true}).click();await selectLanguage(page,'zh-CN');await expect(page.getByRole('tab',{name:'日归档',exact:true})).toBeVisible();await expect(page.getByRole('tab',{name:'月归档',exact:true})).toBeVisible();expect(writes).toBe(0);
});
