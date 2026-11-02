import { selectLanguage } from '../language-switcher'
import { spawnSync } from 'node:child_process';
import { expect, test, type APIResponse, type Page } from '@playwright/test';

const origin = process.env.TEST_ARCHIVE_TASKS_ORIGIN;
const apiOrigin = process.env.TEST_ARCHIVE_TASKS_API_ORIGIN;
const brand = '0199a000-0000-7000-8000-000000000002';
const fixtureBin = process.env.ARCHIVE_TASKS_FIXTURE_BIN;
const viewport = process.env.REPORT_ARCHIVE_TASKS_VIEWPORT;
const username = process.env.TEST_ARCHIVE_TASKS_ADMIN_USERNAME;
const password = process.env.TEST_ARCHIVE_TASKS_ADMIN_PASSWORD;
const ack = 'owned_synthetic_database';
type Task = { id: string; brand_id: string; state: string; version: number; attempt_count: number; archive_id: string | null; policy_version: number };

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body);
  expect(envelope.success).toBe(true);
  return envelope.data as T;
}
async function navigate(page: Page, _project: string, name: string) {
  const mobileNavigation = page.viewportSize()!.width <= 700;
  if (mobileNavigation && name === '工作台|Dashboard') {
    await page.locator('.mobile-nav').getByRole('button', { name: new RegExp(name) }).click();
  } else if (mobileNavigation) {
    await page.locator('.mobile-nav button').nth(4).click();
    await page.locator('.mobile-more-menu').getByRole('button', { name: new RegExp(name) }).click();
  } else {
    await page.locator('.side-nav').getByRole('button', { name: new RegExp(name) }).click();
  }
}
function fixture(command: 'advance' | 'verify') {
  expect(fixtureBin).toBeTruthy();
  const env = { ...process.env };
  delete env.TEST_ARCHIVE_TASKS_ADMIN_PASSWORD;
  delete env.BOOTSTRAP_ADMIN_PASSWORD;
  const result = spawnSync(fixtureBin!, [command], { env, encoding: 'utf8', timeout: 30_000, maxBuffer: 64_000 });
  expect(result.error, 'Owned fixture must execute without exposing child diagnostics').toBeUndefined();
  expect(result.status, 'Owned fixture command must succeed').toBe(0);
  return JSON.parse(result.stdout) as { task_id: string; state: string; archive_id: string | null; archive_count: string; ledger_count: string; total_points: string };
}

test('archive configuration hides task lists while explicit API retry keeps its original receipt', async ({page},info)=>{
 expect(process.env.REPORT_ARCHIVE_TASKS_FIXTURE_CONFIRM).toBe(ack);expect(viewport).toBe(info.project.name);expect(username).toBe(`archive_tasks_browser_${info.project.name}`);
 const api=`${origin}/api/v1/admin`,headers={'X-Brand-ID':brand};
 await data(await page.request.post(`${api}/auth/login`,{headers:{Origin:origin!,'Idempotency-Key':crypto.randomUUID()},data:{identifier:username,password}}));
 const me=await data<{account:{id:string}}>(await page.request.get(`${api}/me`,{headers}));
 const tasks=await data<{items:Task[]}>(await page.request.get(`${api}/report-archive-tasks?limit=100&offset=0`,{headers}));
 const task=tasks.items[0]!;expect(task.state).toBe('failed');
 const economics=()=>{const result=fixture('verify');return {ledger_count:result.ledger_count,total_points:result.total_points}};
 const before=economics();
 await page.route('**/api/v1/context',async route=>{const response=await route.fetch({headers:{...route.request().headers(),Host:'harbor.localhost'}});await route.fulfill({response});});
 await page.goto(origin!);
 const key=crypto.randomUUID(),body={version:task.version};
 const path=`${api}/report-archive-tasks/${task.id}/retry`;
 const requests:Array<{body:string|null;key:string|undefined}>=[];
 let accepted:Task|undefined;
 await page.route(path,async route=>{
  requests.push({body:route.request().postData(),key:route.request().headers()['idempotency-key']});
  const response=await route.fetch();
  if(requests.length===1){accepted=await data<Task>(response);await route.abort('failed');return;}
  await route.fulfill({response});
 });
 const retry=()=>page.evaluate(async({path,headers,body})=>{
  try{const response=await fetch(path,{method:'POST',credentials:'same-origin',headers,body:JSON.stringify(body)});if(!response.ok)throw new Error('Retry failed');return {unknown:false,data:(await response.json()).data};}
  catch{return {unknown:true,data:null};}
 },{path,headers:{...headers,'Content-Type':'application/json','Idempotency-Key':key,'X-Report-Archive-Actor-ID':me.account.id},body});
 expect((await retry()).unknown).toBe(true);expect(accepted!.state).toBe('pending');
 expect(fixture('advance').state).toBe('completed');expect((await retry()).data).toEqual(accepted);
 expect(requests).toHaveLength(2);expect(requests[1]).toEqual(requests[0]);expect(economics()).toEqual(before);
 const complete=await data<Task>(await page.request.get(`${api}/report-archive-tasks/${task.id}`,{headers}));expect(complete.state).toBe('completed');
 let taskReads=0;page.on('request',r=>{if(r.url().includes('/api/v1/admin/report-archive-tasks'))taskReads++;});

 await page.goto(origin!);await selectLanguage(page,'en');await expect(page.getByRole('button',{name:'Archive configuration',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'Archive records',exact:true})).toHaveCount(0);
 await expect(page.getByTestId('archive-policy-editor')).toHaveCount(0);await expect(page.getByTestId('archive-tasks-list')).toHaveCount(0);expect(taskReads).toBe(0);
 const observed=fixture('verify');expect(observed.archive_count).toBe('1');expect(observed.ledger_count).toBe('0');expect(observed.total_points).toBe('0');
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
});
