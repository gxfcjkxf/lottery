import { test, expect } from './platform-fixture';

test.use({ storageState: async ({ operatorSession }, use) => use(operatorSession) });

const origin = 'http://127.0.0.1:5184';
const brand = '0199a000-0000-7000-8000-000000000001';
const model = { model: 'DIGITS_0_9', length: 3, allow_repeat: true, ordered: true };
const definition = {
  schema_version: 1, model, selection: { mode: 'numbers' }, unit_points: '1',
  prize_tiers: [{ code: 'EXACT', condition: { op: 'equals', field: 'position_match', value: 3 }, odds: '10', exclusive: true }],
  mixed_tier_policy: 'max_all', rounding: 'half_up', rounding_scope: 'order', limits: { max_combinations: 100, max_multiplier: '1000' },
};

test('one brand administrator creates and reviews rules with immediate and next-period effects', async ({ page }, info) => {
  await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'zh-CN'));
  const me = await page.request.get(`${origin}/api/v1/admin/me`);
  expect(me.status()).toBe(200);
  const account = (await me.json()).data.account;
  const headers = { Origin: origin, 'X-Brand-ID': brand };
  async function post(path: string, body: unknown, status = 200) {
    const response = await page.request.post(`${origin}/api/v1/admin${path}`, {
      headers: { ...headers, 'Idempotency-Key': crypto.randomUUID() }, data: body,
    });
    expect(response.status(), await response.text()).toBe(status);
    return (await response.json()).data;
  }
  await page.goto(origin);
  await page.getByLabel('选择真实后台品牌', { exact: true }).selectOption(brand);
  if (info.project.name === 'mobile360') {
    await page.locator('.mobile-nav button').last().click();
    await page.locator('.mobile-more-menu').getByRole('button', { name: /规则配置/ }).click();
  } else await page.locator('.side-nav').getByRole('button', { name: /规则配置/ }).click();
  const panel = page.locator('.rule-versions');
  for (const effect_mode of ['immediate', 'next_period']) {
    const suffix = crypto.randomUUID().replaceAll('-', '').slice(0, 10);
    const game = await post('/games', { code: `self_${suffix}`, name: 'Owned self-review game', model, timezone: 'UTC', reason: 'Prepare owned self-review verification' }, 201);
    const play = await post(`/games/${game.id}/plays`, { code: 'exact', name: 'Exact digits', reason: 'Prepare owned play' }, 201);
    const draft = await post('/rule-versions', { play_id: play.id, definition, effect_mode, reason: 'Create own draft' }, 201);
    const validated = await post(`/rule-versions/${draft.id}/validate`, { version: draft.version, cases: [{
      name: '111 repeat digits', selection: { digits: [[1], [1], [1]] }, draw: { digits: [1, 1, 1] }, multiplier: '1', expected_bet_points: '1', expected_prize_points: '10', expected_won: true,
    }], reason: 'Validate own rule' });
    expect(validated.validation.passed).toBe(true);
    await post(`/rule-versions/${draft.id}/submit-review`, { version: validated.version, reason: 'Submit own rule' });
    await panel.getByLabel('直接指定玩法 ID', { exact: true }).fill(play.id);
    await panel.getByRole('button', { name: '选择玩法并读取历史', exact: true }).click();
    await panel.locator('.version-list').getByRole('button', { name: /第 1 版/ }).click();
    const card = panel.locator('.review');
    await card.getByLabel('审核原因（批准和拒绝均必填）', { exact: true }).fill('Own rule checked and warnings acknowledged');
    const approve = card.getByRole('button', { name: '批准该版本', exact: true });
    await expect(approve).toBeDisabled();
    await card.getByRole('checkbox').check();
    const attempts: Array<{ body: string | null; key: string | undefined }> = [];
    const approvalUrl = `${origin}/api/v1/admin/rule-versions/${draft.id}/approve`;
    if (effect_mode === 'immediate') {
      await page.route(approvalUrl, async route => {
        attempts.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'] });
        if (attempts.length === 1) {
          const committed = await route.fetch();
          expect(committed.status()).toBe(200);
          // The real approval committed, but a proxy returned an unsupported old envelope.
          await route.fulfill({ status: 400, json: { success: false, error: 'legacy proxy response' } });
        } else await route.continue();
      });
      const malformed = page.waitForResponse(response => response.url() === approvalUrl && response.request().method() === 'POST');
      await approve.click();
      expect((await malformed).status()).toBe(400);
      await expect(panel.getByRole('alert')).toContainText('操作结果未确认');
    }
    const reply = page.waitForResponse(response => response.url().endsWith(`/rule-versions/${draft.id}/approve`) && response.request().method() === 'POST');
    await approve.click();
    const response = await reply;
    expect(response.status(), await response.text()).toBe(200);
    const reviewed = (await response.json()).data;
    if (effect_mode === 'immediate') {
      expect(attempts).toHaveLength(2);
      expect(attempts[1]).toEqual(attempts[0]);
      await page.unroute(approvalUrl);
    }
    expect(reviewed.created_by).toBe(account.id);
    expect(reviewed.reviewed_by).toBe(account.id);
    expect(reviewed.review_comment).toBe('Own rule checked and warnings acknowledged');
    expect(reviewed.status).toBe(effect_mode === 'immediate' ? 'active' : 'approved');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  }
});
