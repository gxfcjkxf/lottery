import {test,expect} from './platform-fixture';
const origin='http://127.0.0.1:5185';
test('monthly retention policy and execution records use new endpoints',async({page},info)=>{
 let legacyRequests=0;page.on('request',request=>{if(request.url().includes('/report-archive'))legacyRequests++});
 const policy={version:3,enabled:true,execution_day:10,execution_time:'02:00',timezone:'Asia/Singapore',next_run:{execute_at:'2026-12-09T18:00:00Z',cutoff_at:'2026-10-31T16:00:00Z'},updated_at:'2026-11-12T00:00:00Z'};
 await page.route('**/api/v1/platform/data-retention-policy',route=>route.fulfill({json:{success:true,data:policy}}));
 await page.route('**/api/v1/platform/data-retention-tasks?**',route=>{const q=new URL(route.request().url()).searchParams;return route.fulfill({json:{success:true,data:{items:[],total_count:'0',limit:Number(q.get('limit')),offset:Number(q.get('offset'))}}})});
 await page.goto(origin);await page.getByRole('button',{name:'Operations',exact:true}).click();await page.getByTestId('platform-operations-tabs').getByRole('button',{name:'Data retention',exact:true}).click();
 const panel=page.getByTestId('data-retention');await expect(panel).toBeVisible();await panel.getByRole('tab',{name:'Retention policy',exact:true}).click();await expect(panel).toContainText('Asia/Singapore');
 expect(legacyRequests).toBe(0);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:info.outputPath('monthly-retention.png'),fullPage:true});
});
