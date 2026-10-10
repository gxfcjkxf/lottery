import { test, expect, type APIResponse, type Page } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { rememberAdminSession } from './support/admin-session';

const origin = process.env.TEST_ADMIN_ORIGIN ?? 'http://localhost:5174';
const admin = `${origin}/api/v1/admin`;
const brand = '0199a000-0000-7000-8000-000000000001';
const fixture = process.env.COMMISSION_FIXTURE_BIN;
const fields = [
  'observed_calculated_points', 'calculated_points', 'paid_entry_count', 'paid_points',
  'adjustment_entry_count', 'adjustment_credit_points', 'adjustment_debit_points',
  'correction_entry_count', 'correction_credit_points', 'correction_debit_points',
  'posting_entry_count', 'actual_net_points', 'manual_adjustment_net_points',
  'effective_target_points', 'calculation_minus_actual_points', 'effective_minus_actual_points',
  'calculation_complete', 'effective_target_complete',
] as const;
const coverageFields = ['selected_cycle_count', 'ready_cycle_count', 'unready_cycle_count'] as const;
const csvColumns = [
  'record_type', 'brand_id', 'snapshot_at', 'timezone', 'from', 'to', 'group_by', 'agent_id', 'member_id', 'cycle_id', 'key', 'label',
  ...coverageFields, ...fields,
];
const expectedTotals = {
  observed_calculated_points: '1', calculated_points: '1', paid_entry_count: '1', paid_points: '1',
  adjustment_entry_count: '3', adjustment_credit_points: '3', adjustment_debit_points: '4',
  correction_entry_count: '0', correction_credit_points: '0', correction_debit_points: '0',
  posting_entry_count: '4', actual_net_points: '0', manual_adjustment_net_points: '-1',
  effective_target_points: '0', calculation_minus_actual_points: '1', effective_minus_actual_points: '0',
  calculation_complete: true, effective_target_complete: true,
};
type Totals = typeof expectedTotals;
type Coverage = Record<typeof coverageFields[number], string>;
type Analysis = {
  brand_id: string; snapshot_at: string; timezone: string;
  query: { from: string; to: string; group_by: 'cycle' | 'agent'; limit: number; offset: number; agent_id: string | null; member_id: string | null; cycle_id: string | null };
  coverage: Coverage; summary: Totals; items: Array<{ key: string; label: string; totals: Totals }>; total_groups: string;
};
type FixtureVerify = { commission_ledger_entries: number; commission_wallet_points: number; economic_fingerprint: string; commission_business_fingerprint: string };

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(200);
  const envelope = JSON.parse(body);
  expect(envelope.success).toBe(true);
  return envelope.data as T;
}
function verify(): FixtureVerify {
  return JSON.parse(execFileSync(fixture!, ['verify'], { env: process.env, encoding: 'utf8', timeout: 30_000 })) as FixtureVerify;
}
async function navigate(page: Page) {
  if (await page.evaluate(() => innerWidth <= 700)) {
    await page.locator('.mobile-nav button').nth(4).click();
    await page.locator('.mobile-more-menu').getByRole('button', { name: /报表和对账|Reports/ }).click();
  } else {
    await page.locator('.side-nav').getByRole('button', { name: /报表和对账|Reports/ }).click();
  }
}
function localInput(date: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}T${p(date.getHours())}:${p(date.getMinutes())}:${p(date.getSeconds())}`.replace(/:00$/, '');
}
function canonicalIso(date: Date): string { return date.toISOString().replace('.000Z', 'Z'); }
function parseCsvLine(line: string): string[] {
  const out: string[] = [];
  let value = '', quoted = false;
  for (let i = 0; i < line.length; i++) {
    const c = line[i]!;
    if (quoted) {
      if (c === '"' && line[i + 1] === '"') { value += '"'; i++; }
      else if (c === '"') quoted = false;
      else value += c;
    } else if (c === ',' && !quoted) { out.push(value); value = ''; }
    else if (c === '"' && value === '') quoted = true;
    else value += c;
  }
  out.push(value);
  return out;
}

test('commission cycle analysis shows saved-period economics and exports the committed readonly report', async ({ page, context }, info) => {
  test.skip(!fixture || !process.env.COMMISSION_FIXTURE_CONFIRM || !process.env.TEST_COMMISSION_ADMIN_PASSWORD, 'Requires the prepared owned commission fixture and admin credentials');
  test.setTimeout(90_000);
  page.setDefaultTimeout(10_000);
  const before = verify();
  expect(before.commission_ledger_entries).toBe(1);
  expect(before.commission_wallet_points).toBe(0);
  expect(before.economic_fingerprint).toMatch(/^[0-9a-f]{64}$/);
  expect(before.commission_business_fingerprint).toMatch(/^[0-9a-f]{64}$/);
  const errors: string[] = [];
  const writes: string[] = [];
  page.on('pageerror', error => errors.push(error.message));

  await data(await page.request.post(`${admin}/auth/login`, {
    headers: { Origin: origin, 'Idempotency-Key': crypto.randomUUID() },
    data: { identifier: 'commission_admin', password: process.env.TEST_COMMISSION_ADMIN_PASSWORD },
  }));
  const me = await data<{ account: { id: string } }>(await page.request.get(`${admin}/me`, { headers: { 'X-Brand-ID': brand } }));
  await rememberAdminSession(context, 'commission_admin', origin);

  const get = async <T>(path: string) => data<T>(await page.request.get(`${admin}${path}`, { headers: { 'X-Brand-ID': brand } }));
  const payments = await get<{ items: Array<{ id: string; cycle_id: string; state: string; payout_mode: string; paid_points: string }> }>('/commission-payments?limit=100&offset=0');
  const payment = payments.items.find(row => row.state === 'paid' && row.payout_mode === 'automatic' && row.paid_points === '1');
  expect(payment, 'automatic one-point payment must exist in the prepared fixture').toBeTruthy();
  const targets = await get<{ items: Array<{ id: string; agent_id: string; member_id: string }> }>(`/commission-payments/${payment!.id}/targets?limit=20&offset=0`);
  const target = targets.items[0];
  expect(target).toBeTruthy();
  const adjustmentHistory = await get<{ items: Array<{ delta_points: string }> }>(`/commission-payment-targets/${target!.id}/adjustments?limit=100&offset=0`);
  expect(adjustmentHistory.items.map(row => row.delta_points).sort()).toEqual(['-4', '1', '2']);
  expect(adjustmentHistory.items.reduce((sum, row) => sum + BigInt(row.delta_points), 0n).toString()).toBe('-1');

  const cycles = await get<{ items: Array<{ id: string; state: string; evidence_current: boolean; window_to: string }> }>('/commission-cycles?limit=100&offset=0');
  const savedCycle = cycles.items.find(cycle => cycle.id === payment!.cycle_id);
  expect(savedCycle).toBeTruthy();
  const cycleEnd = new Date(savedCycle!.window_to);
  // Select a one-second cycle-end window that excludes all later postings.
  // Unlike the old posting-time report, all four actual entries still belong
  // to this cycle and must remain in its analysis and complete CSV.
  const from = new Date(Math.floor(cycleEnd.getTime() / 1000) * 1000);
  const to = new Date(from.getTime() + 1000);
  const ledger = await get<{items: Array<{entry_type: string; created_at: string}>}>(`/wallets/${target!.member_id}/ledger?limit=100&offset=0`);
  const postings = ledger.items.filter(row => ['commission', 'commission_adjustment', 'commission_correction'].includes(row.entry_type));
  expect(postings).toHaveLength(4);
  for (const row of postings) expect(new Date(row.created_at).getTime()).toBeGreaterThanOrEqual(to.getTime());
  const fromLocal = localInput(from), toLocal = localInput(to);

  await page.goto(origin);
  await page.getByLabel(/选择真实后台品牌|Select an administrative brand/, { exact: true }).selectOption(brand);
  await navigate(page);
  const panel = page.locator('.commission-analysis');
  await expect(panel.getByRole('heading', { name: /佣金周期核算与实际入账分析|Commission cycle calculation and actual posting analysis/ })).toBeVisible();
  page.on('request', request => {
    if (request.url().startsWith(`${admin}/`) && request.method() !== 'GET') writes.push(`${request.method()} ${request.url()}`);
  });

  await panel.locator('input[type="datetime-local"]').nth(0).fill(fromLocal);
  await panel.locator('input[type="datetime-local"]').nth(1).fill(toLocal);
  await panel.getByLabel(/代理 UUID（可选）|Agent UUID \(optional\)/).fill(target!.agent_id);
  await panel.getByLabel(/会员 UUID（可选）|Member UUID \(optional\)/).fill(target!.member_id);
  await panel.getByLabel(/周期 UUID（可选）|Cycle UUID \(optional\)/).fill(payment!.cycle_id);

  const reportResponse = page.waitForResponse(response => response.request().method() === 'GET' && response.url().startsWith(`${admin}/reports/commission-analysis?`));
  await panel.getByTestId('commission-analysis-query').click();
  const report = await data<Analysis>(await reportResponse);
  expect(report.brand_id).toBe(brand);
  expect(report.query).toMatchObject({ from: canonicalIso(from), to: canonicalIso(to), group_by: 'cycle', agent_id: target!.agent_id, member_id: target!.member_id, cycle_id: payment!.cycle_id, limit: 20, offset: 0 });
  expect(report.total_groups).toBe('1');
  expect(report.items).toHaveLength(1);
  expect(report.items[0]).toMatchObject({ key: payment!.cycle_id, label: payment!.cycle_id });
  expect(report.summary).toEqual(expectedTotals);
  expect(report.items[0]!.totals).toEqual(expectedTotals);
  expect(report.coverage).toEqual({ selected_cycle_count: '1', ready_cycle_count: '1', unready_cycle_count: '0' });
  expect(cycleEnd.getTime()).toBeGreaterThanOrEqual(from.getTime());
  expect(cycleEnd.getTime()).toBeLessThan(to.getTime());

  const summary = panel.getByTestId('commission-analysis-summary');
  await expect(summary.locator('dd')).toHaveText(['1', '1', '1', '1', '3', '3', '4', '0', '0', '0', '4', '0', '−1', '0', '1', '0', '完整', '完整']);
  const usesMobileGrouping = await page.evaluate(() => innerWidth <= 700);
  if (usesMobileGrouping) {
    await panel.locator('.mobile-groups details summary').click();
    await expect(panel.locator('.mobile-groups')).toBeVisible();
    await expect(panel.locator('.mobile-groups dd')).toHaveCount(18);
  } else {
    await expect(panel.locator('.table-wrap')).toBeVisible();
  }
  const configuredWidth = await page.evaluate(() => innerWidth);
  await expect.poll(() => page.evaluate(width => document.documentElement.scrollWidth <= width + 1, configuredWidth)).toBe(true);
  await panel.screenshot({ path: info.outputPath('commission-analysis-panel.png') });
  await page.screenshot({ path: info.outputPath('commission-analysis-viewport.png') });

  const csvResponse = page.waitForResponse(response => response.request().method() === 'GET' && response.url().startsWith(`${admin}/reports/commission-analysis.csv?`));
  const downloadEvent = page.waitForEvent('download');
  await panel.getByTestId('commission-analysis-export').click();
  const csv = await csvResponse;
  expect(csv.status(), await csv.text()).toBe(200);
  const csvUrl = new URL(csv.url());
  expect(csvUrl.searchParams.get('agent_id')).toBe(target!.agent_id);
  expect(csvUrl.searchParams.get('member_id')).toBe(target!.member_id);
  expect(csvUrl.searchParams.get('cycle_id')).toBe(payment!.cycle_id);
  expect(csvUrl.searchParams.has('limit')).toBe(false);
  expect(csv.headers()['x-report-brand-id']).toBe(brand);
  expect(csv.headers()['x-report-kind']).toBe('commission_analysis');
  expect(csv.headers()['x-report-format-version']).toBe('1');
  expect(csv.headers()['x-report-timezone']).toBeTruthy();
  expect(csv.headers()['x-report-group-count']).toBe('1');
  expect(csv.headers()['x-report-audit-id']).toMatch(/^[0-9a-f-]{36}$/i);
  const download = await downloadEvent;
  const filePath = await download.path();
  expect(filePath).toBeTruthy();
  const bytes = readFileSync(filePath!);
  expect(bytes.subarray(0, 3)).toEqual(Buffer.from([0xef, 0xbb, 0xbf]));
  expect(createHash('sha256').update(bytes).digest('hex')).toBe(csv.headers()['x-report-sha256']);
  expect(String(bytes.length)).toBe(csv.headers()['x-report-byte-count']);
  const lines = bytes.subarray(3).toString('utf8').trimEnd().split(/\r?\n/);
  expect(parseCsvLine(lines[0]!)).toEqual(csvColumns);
  expect(lines).toHaveLength(3);
  const summaryRow = parseCsvLine(lines[1]!);
  const groupRow = parseCsvLine(lines[2]!);
  expect(summaryRow[0]).toBe('summary');
  expect(groupRow[0]).toBe('group');
  expect(summaryRow).toHaveLength(33);
  expect(groupRow).toHaveLength(33);
  expect(groupRow[10]).toBe(payment!.cycle_id);
  expect(summaryRow.slice(12)).toEqual(['1', '1', '0', '1', '1', '1', '1', '3', '3', '4', '0', '0', '0', '4', '0', "'-1", '0', '1', '0', 'true', 'true']);
  expect(groupRow.slice(12)).toEqual(summaryRow.slice(12));

  // Unfiltered cycle grouping spans every saved cycle, including any incomplete
  // cycle still present in this fixture after the preceding serial specs.
  const ends = cycles.items.map(cycle => new Date(cycle.window_to).getTime());
  const broadFrom = new Date(Math.min(...ends) - 60_000).toISOString();
  const broadTo = new Date(Math.max(...ends) + 60_000).toISOString();
  expect(new Date(broadTo).getTime() - new Date(broadFrom).getTime()).toBeLessThanOrEqual(93 * 24 * 60 * 60 * 1000);
  const broadParams = new URLSearchParams({ from: broadFrom, to: broadTo, group_by: 'cycle', limit: '100', offset: '0' });
  const broad = await get<Analysis>(`/reports/commission-analysis?${broadParams}`);
  expect(broad.coverage.selected_cycle_count).toBe(String(cycles.items.length));
  const ready = cycles.items.filter(cycle => cycle.state === 'ready' && cycle.evidence_current);
  expect(broad.coverage.ready_cycle_count).toBe(String(ready.length));
  expect(broad.coverage.unready_cycle_count).toBe(String(cycles.items.length - ready.length));
  expect(broad.coverage.selected_cycle_count).toBe(String(Number(broad.coverage.ready_cycle_count) + Number(broad.coverage.unready_cycle_count)));
  if (ready.length !== cycles.items.length) expect(broad.summary.calculated_points).toBeNull();

  await panel.getByLabel(/代理 UUID（可选）|Agent UUID \(optional\)/).fill('ffffffff-ffff-4fff-8fff-ffffffffffff');
  await expect(panel.getByTestId('commission-analysis-summary')).toHaveCount(0);
  await expect(panel.getByTestId('commission-analysis-export')).toHaveCount(0);
  await expect(panel.locator('.table-wrap, .mobile-groups')).toHaveCount(0);
  await expect.poll(() => page.evaluate(width => document.documentElement.scrollWidth <= width + 1, configuredWidth)).toBe(true);
  await panel.screenshot({ path: info.outputPath('commission-analysis-cleared-panel.png') });
  await page.getByTestId('admin-language').selectOption('en');
  await expect(panel.getByRole('heading', { name: 'Commission cycle calculation and actual posting analysis', exact: true })).toBeVisible();
  await panel.getByLabel('Agent UUID (optional)', { exact: true }).fill(target!.agent_id);
  await panel.getByLabel(/^Group by/).selectOption('agent');
  const agentResponse = page.waitForResponse(response => response.request().method() === 'GET' && response.url().startsWith(`${admin}/reports/commission-analysis?`));
  await panel.getByTestId('commission-analysis-query').click();
  const byAgent = await data<Analysis>(await agentResponse);
  expect(byAgent.query.group_by).toBe('agent');
  expect(byAgent.items).toEqual([{key: target!.agent_id, label: target!.agent_id, totals: expectedTotals}]);
  expect(byAgent.summary).toEqual(expectedTotals);
  await expect(panel.getByTestId('commission-analysis-summary').locator('dd')).toHaveText(['1', '1', '1', '1', '3', '3', '4', '0', '0', '0', '4', '0', '−1', '0', '1', '0', 'Complete', 'Complete']);
  if (usesMobileGrouping) await panel.locator('.mobile-groups details summary').click();
  await expect.poll(() => page.evaluate(width => document.documentElement.scrollWidth <= width + 1, configuredWidth)).toBe(true);
  await panel.screenshot({path: info.outputPath('commission-analysis-english-panel.png')});
  expect(writes).toEqual([]);
  expect(errors).toEqual([]);
  const after = verify();
  expect(after.commission_ledger_entries).toBe(1);
  expect(after.commission_wallet_points).toBe(0);
  expect(after.economic_fingerprint).toBe(before.economic_fingerprint);
  expect(after.commission_business_fingerprint).toMatch(/^[0-9a-f]{64}$/);
  expect(after.commission_business_fingerprint).toBe(before.commission_business_fingerprint);
});
