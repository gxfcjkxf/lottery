import { selectControl } from '../select-control'
import { selectLanguage } from '../language-switcher'
import { test, expect } from './platform-fixture';

const origin = 'http://127.0.0.1:5185';
const brand = '0199a000-0000-7000-8000-000000000001';
const headers = { 'X-Brand-ID': brand };
const kinds = [['points', 'point-policy'], ['withdrawal', 'withdrawal-policy'], ['commission', 'commission-policy']] as const;

async function operations(page: any, tab: string) {
  await page.goto(origin);
  await page.getByRole('button', { name: 'Operations', exact: true }).click();
  await page.getByTestId('platform-operations-tabs').getByRole('button', { name: tab, exact: true }).click();
  if(tab!=='Archive configuration')await selectControl(page.locator(".topbar-brand-picker .select-control-trigger"), brand);
}

test('archive configuration shows saved schedules without requesting task lists', async ({page},info)=>{
 const policy={version:3,daily_enabled:true,monthly_enabled:true,timezone:'Asia/Singapore',updated_at:'2026-10-10T00:00:00Z',daily_execution_time:'07:30',daily_period_offset:2,monthly_execution_day:5,monthly_execution_time:'06:00',monthly_period_offset:2,next_runs:{daily:{execute_at:'2026-10-09T23:30:00Z',period_key:'2026-10-08'},monthly:{execute_at:'2026-11-04T22:00:00Z',period_key:'2026-09'}}};
 let tasks=0;page.on('request',request=>{if(request.url().includes('/api/v1/platform/report-archive-tasks'))tasks++;});
 await page.route('**/api/v1/platform/report-archive-policy',route=>route.fulfill({json:{success:true,data:policy}}));
 await operations(page,'Archive configuration');
 const panel=page.getByTestId('platform-archive-policy');
 await expect(panel.getByTestId('archive-policy-monthly-day')).toHaveValue('5');await expect(panel.getByTestId('archive-policy-monthly-time')).toHaveValue('06:00');await expect(panel.getByTestId('archive-policy-monthly-offset')).toHaveValue('2');
 await expect(panel.getByTestId('archive-policy-monthly-day')).toBeEnabled();await expect(panel).toContainText('2026-09');await expect(page.getByTestId('platform-archive-task-list')).toHaveCount(0);expect(tasks).toBe(0);
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);await page.screenshot({path:info.outputPath('platform-archive-config.png'),fullPage:true});
});
