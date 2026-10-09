import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync } from 'node:fs';
import { expect, request, test, type APIRequestContext, type APIResponse } from '@playwright/test';

const brand = '0199a000-0000-7000-8000-000000000002';
const origin = process.env.TEST_ATTRIBUTION_ORIGIN;
const viewport = process.env.REPORT_ATTRIBUTION_VIEWPORT;
const username = process.env.TEST_ATTRIBUTION_ADMIN_USERNAME;
const password = process.env.TEST_ATTRIBUTION_ADMIN_PASSWORD;
const confirmation = 'owned_synthetic_database';
const id = () => crypto.randomUUID();
const suffix = () => id().replaceAll('-', '').slice(0, 12);

function assertAttributionFixtureEnvironment() {
  if (!['desktop', 'mobile'].includes(viewport ?? '') || process.env.APP_ENV !== 'test' || process.env.REPORT_ATTRIBUTION_FIXTURE_CONFIRM !== confirmation) {
    throw new Error('Attribution browser tests require the explicitly owned desktop/mobile test fixture');
  }
  const expectedDatabase = `lottery_attribution_browser_${viewport}`;
  if (!new RegExp(`^postgres(?:ql)?:\\/\\/lottery_test@127\\.0\\.0\\.1:(?:5432|55432)\\/${expectedDatabase}\\?sslmode=disable$`).test(process.env.DATABASE_URL ?? '')) {
    throw new Error('Attribution browser tests require their exact owned loopback database');
  }
  if (['DATABASE_READ_URL', 'DATABASE_READ_URLS'].some(name => process.env[name])) {
    throw new Error('Attribution browser tests forbid read database overrides');
  }
  if (!/^http:\/\/localhost:(?:5174|15292)$/.test(origin ?? '') || username !== `attribution_browser_${viewport}` || !password) {
    throw new Error('Attribution browser tests require the matching local UI origin and isolated administrator');
  }
}

const adminBase = `${origin}/api/v1/admin`;
const publicBase = `${origin}/api/v1/b/harbor`;
type Envelope<T> = { success: boolean; data: T; error?: unknown };

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body) as Envelope<T>;
  expect(envelope.success, body).toBe(true);
  return envelope.data;
}

async function loginAdmin(client: APIRequestContext, name: string, secret: string) {
  return data<{ access_token: string }>(await client.post(`${adminBase}/auth/login`, {
    headers: { Origin: origin!, 'X-Brand-ID': brand, 'Idempotency-Key': id() },
    data: { identifier: name, password: secret },
  }));
}

function adminAPI(client: APIRequestContext, token: string) {
  const headers = { Authorization: `Bearer ${token}`, Origin: origin!, 'X-Brand-ID': brand };
  return {
    get: async <T>(path: string) => data<T>(await client.get(`${adminBase}${path}`, { headers })),
    write: async <T>(path: string, body: unknown, status = 200) => data<T>(await client.post(`${adminBase}${path}`, {
      headers: { ...headers, 'Idempotency-Key': id() }, data: body,
    }), status),
    put: async <T>(path: string, body: unknown) => data<T>(await client.put(`${adminBase}${path}`, {
      headers: { ...headers, 'Idempotency-Key': id() }, data: body,
    })),
  };
}

function publicAPI(client: APIRequestContext, token: string) {
  const headers = { Authorization: `Bearer ${token}`, Origin: origin! };
  return {
    get: async <T>(path: string) => data<T>(await client.get(`${publicBase}${path}`, { headers })),
    post: async <T>(path: string, body: unknown, status = 200) => data<T>(await client.post(`${publicBase}${path}`, {
      headers: { ...headers, 'Idempotency-Key': id() }, data: body,
    }), status),
  };
}

type PublicAttribution = {
  brand_id: string;
  member_id: string;
  join_method: string;
  joined_at: string;
  code_id: string | null;
  source_code: string | null;
};

const publicAttributionFields = ['brand_id', 'member_id', 'join_method', 'joined_at', 'code_id', 'source_code'];

function expectPublicAttribution(value: PublicAttribution, memberId: string, expected: Partial<PublicAttribution>) {
  expect(Object.keys(value).sort()).toEqual([...publicAttributionFields].sort());
  expect(value).toMatchObject({ brand_id: brand, member_id: memberId, ...expected });
  expect(Number.isNaN(Date.parse(value.joined_at))).toBe(false);
}

async function registerMember(client: APIRequestContext, agentCode?: string) {
  const username = `attr_${suffix()}`;
  const password = `Attribution-${suffix()}-Member9`;
  const context = await client.get(`${publicBase}/context`);
  const contextData = await data<{ auth?: { captcha_enabled?: boolean } }>(context);
  let captcha: Record<string, string> = {};
  if (contextData.auth?.captcha_enabled) {
    const challenge = await data<{ id: string; svg: string }>(await client.get(`${publicBase}/auth/challenge`));
    const answer = challenge.svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? '';
    expect(answer).toMatch(/^[A-Za-z0-9]+$/);
    captcha = { captcha_id: challenge.id, captcha_answer: answer };
  }
  const response = await client.post(`${publicBase}/auth/register`, {
    headers: { Origin: origin!, 'Idempotency-Key': id() },
    data: {
      username, password, privacy_policy_version: 'dev-1', service_terms_version: 'dev-1',
      ...(agentCode ? { agent_code: agentCode } : {}), ...captcha,
    },
  });
  const member = await data<{ access_token: string; member: { id: string } }>(response, 201);
  return { username, password, memberId: member.member.id, token: member.access_token };
}

async function loginMember(client: APIRequestContext, name: string, secret: string) {
  const authContext = await data<{ auth?: { captcha_enabled?: boolean } }>(await client.get(`${publicBase}/context`));
  let captcha: Record<string, string> = {};
  if (authContext.auth?.captcha_enabled) {
    const challenge = await data<{ id: string; svg: string }>(await client.get(`${publicBase}/auth/challenge`));
    const answer = challenge.svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? '';
    expect(answer).toMatch(/^[A-Za-z0-9]+$/);
    captcha = { captcha_id: challenge.id, captcha_answer: answer };
  }
  return data<{ access_token: string }>(await client.post(`${publicBase}/auth/login`, {
    headers: { Origin: origin!, 'Idempotency-Key': id() },
    data: { identifier: name, password: secret, privacy_policy_version: 'dev-1', service_terms_version: 'dev-1', ...captcha },
  }));
}

const digitsModel = {
  model: 'DIGITS_0_9', regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
  special_pool: { min: 0, max: 0, values: [], allow_repeat: false }, regular_count: 0,
  special_count: 0, pool_size: 0, total_count: 0, length: 3, allow_repeat: true, ordered: true,
};
const digitsDefinition = {
  schema_version: 1, model: digitsModel,
  selection: { mode: 'numbers', regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: [], feature_choices: {} },
  number_attributes: {}, unit_points: '1',
  prize_tiers: [{ code: 'MATCH_ALL_DIGITS', condition: { op: 'equals', field: 'position_match', value: 3 }, odds: '10', exclusive: true, cap_points: null }],
  mixed_tier_policy: 'max_all', cap_points: null, rounding: 'half_up', rounding_scope: 'order',
  limits: { max_combinations: 1000, max_multiplier: '1000', max_bet_points: null },
};
const winningCase = {
  name: 'known three digit match',
  selection: { regular: [], special: [], digits: [[1], [2], [1]], exclude: [], attributes: {}, features: {} },
  draw: { regular: [], special: [], digits: [1, 2, 1] }, multiplier: '1',
  expected_bet_points: '1', expected_prize_points: '10', expected_won: true,
};

async function createBettingGame(api: ReturnType<typeof adminAPI>, reviewer: ReturnType<typeof adminAPI>, code: string) {
  const game = await api.write<{ id: string; version: number }>('/games', {
    code, name: `Attribution browser ${code}`, model: digitsModel, timezone: 'UTC', reason: 'owned attribution browser fixture game',
  }, 201);
  const play = await api.write<{ id: string }>(`/games/${game.id}/plays`, {
    code: 'position_match', name: 'Position match', reason: 'real attribution fixture play',
  }, 201);
  const rule = await api.write<{ id: string; version: number }>('/rule-versions', {
    play_id: play.id, definition: digitsDefinition, effect_mode: 'immediate', reason: 'real attribution fixture rule',
  }, 201);
  const validation = await api.write<{ version: number; validation: { passed: boolean } }>(`/rule-versions/${rule.id}/validate`, {
    version: rule.version, cases: [winningCase], reason: 'validate the real placed-order rule',
  });
  expect(validation.validation.passed).toBe(true);
  const submitted = await api.write<{ version: number }>(`/rule-versions/${rule.id}/submit-review`, {
    version: validation.version, reason: 'request independent fixture rule approval',
  });
  await reviewer.write(`/rule-versions/${rule.id}/approve`, {
    version: submitted.version, reason: 'independently approve attribution fixture rule', warnings_acknowledged: true,
  });
  const drawAt = new Date(Date.now() + 10 * 60_000);
  drawAt.setUTCMilliseconds(0);
  await api.put(`/games/${game.id}/schedule`, {
    version: game.version, reason: 'open a real attribution browser betting window',
    spec: {
      timezone: 'UTC', mode: 'daily', daily_draw_times: [drawAt.toISOString().slice(11, 19)],
      interval_seconds: 0, busy_windows: [], bet_open_before_seconds: 900, bet_close_before_seconds: 60,
      pause_dates: [], weekdays: [0, 1, 2, 3, 4, 5, 6], holiday_dates: [], holiday_policy: 'normal',
    },
  });
  const generated = await api.write<{ periods: Array<{ id: string; status: string }> }>(`/games/${game.id}/periods/generate`, {
    from: new Date(Date.now() - 1000).toISOString(), to: new Date(Date.now() + 10 * 60_000).toISOString(),
    reason: 'generate this real attribution test draw',
  });
  expect(generated.periods).toHaveLength(1);
  return { ...game, playId: play.id, periodId: generated.periods[0]!.id };
}

async function placeBet(client: APIRequestContext, member: { token: string }, game: { id: string; playId: string; periodId: string }) {
  const api = publicAPI(client, member.token);
  await expect.poll(async () => {
    const current = await api.get<{ period: { status: string } | null }>(`/games/${game.id}`);
    return current.period?.status;
  }, { timeout: 8_000, intervals: [200, 400, 800] }).toBe('betting');
  const catalog = await api.get<{
    period: { id: string; status: string } | null;
    plays: Array<{ id: string; rule_version_id: string }>;
    policy_versions: { brand: number; game: number };
  }>(`/games/${game.id}`);
  expect(catalog.period).toMatchObject({ id: game.periodId, status: 'betting' });
  const published = catalog.plays.find(play => play.id === game.playId);
  expect(published).toBeTruthy();
  const body = {
    period_id: game.periodId, play_id: game.playId, rule_version_id: published!.rule_version_id,
    selection: winningCase.selection, multiplier: '1', policy_versions: catalog.policy_versions,
  };
  const preview = await api.post<{ actor_context: string }>('/bet-previews', body);
  const order = await api.post<{ id: string; status: string; total_points: string }>('/bet-orders', {
    ...body, actor_context: preview.actor_context,
  }, 201);
  expect(order).toMatchObject({ status: 'placed', total_points: '1' });
  return order.id;
}

function parseCsv(source: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [], field = '', quoted = false;
  for (let i = 0; i < source.length; i++) {
    const char = source[i]!;
    if (quoted) {
      if (char === '"' && source[i + 1] === '"') { field += '"'; i++; }
      else if (char === '"') quoted = false;
      else field += char;
    } else if (char === '"') quoted = true;
    else if (char === ',') { row.push(field); field = ''; }
    else if (char === '\n') { row.push(field.replace(/\r$/, '')); rows.push(row); row = []; field = ''; }
    else field += char;
  }
  if (quoted) throw new Error('Unexpected unfinished CSV quote');
  return rows;
}

test('real attribution reports preserve saved agent scope and export unique order totals', async ({ page }, info) => {
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  assertAttributionFixtureEnvironment();
  expect(info.project.name).toBe(viewport);

  const creatorLogin = await loginAdmin(page.request, username!, password!);
  const creator = adminAPI(page.request, creatorLogin.access_token);
  const reviewerRequest = await request.newContext();
  try {
    const reviewerRole = await creator.write<{ id: string }>('/roles', {
      code: `attr_review_${suffix()}`, name: 'Attribution fixture reviewer', status: 'active',
      permissions: ['rule.view.brand', 'rule.review.brand'], reason: 'create distinct reviewer for genuine rules',
    }, 201);
    const reviewerName = `attr_reviewer_${suffix()}`;
    const reviewerPassword = `Attribution-${suffix()}-Reviewer9`;
    await creator.write('/accounts', {
      username: reviewerName, password: reviewerPassword, role_ids: [reviewerRole.id],
      reason: 'create a distinct reviewer for actual browser orders',
    }, 201);
    const reviewerLogin = await loginAdmin(reviewerRequest, reviewerName, reviewerPassword);
    const reviewer = adminAPI(reviewerRequest, reviewerLogin.access_token);

    let policy = await creator.get<{ version: number; config: { enabled: boolean; max_depth: number; ratio_cap: string; mode: string | null; cycle: string } }>('/agent-policy');
    if (!policy.config.enabled) {
      policy = await creator.put('/agent-policy', {
        version: policy.version,
        config: { enabled: true, max_depth: 3, ratio_cap: '0.1', mode: 'loss', cycle: 'monthly' },
        reason: 'enable an isolated agent tree for attribution history',
      });
    }
    const rootOwner = await creator.write<{ member_id: string }>('/users', {
      username: `attr_root_owner_${suffix()}`, password: `Attribution-${suffix()}-Owner9`, reason: 'create root agent owner',
    }, 201);
    const root = await creator.write<{ id: string; member_id: string; version: number }>('/agents', {
      policy_version: policy.version, member_id: rootOwner.member_id, parent_id: null, parent_version: null,
      config: { ratio: '0.08', mode: null, status: 'active', can_create_children: true }, reason: 'create attribution root agent',
    }, 201);
    const childOwner = await creator.write<{ member_id: string }>('/users', {
      username: `attr_child_owner_${suffix()}`, password: `Attribution-${suffix()}-Owner9`, reason: 'create child agent owner',
    }, 201);
    const child = await creator.write<{ id: string; member_id: string; parent_id: string | null; version: number }>('/agents', {
      policy_version: policy.version, member_id: childOwner.member_id, parent_id: root.id, parent_version: root.version,
      config: { ratio: '0.04', mode: null, status: 'active', can_create_children: false }, reason: 'create direct child attribution agent',
    }, 201);
    const rootCode = await creator.write<{ id: string; code: string; version: number }>('/join-codes', {
      kind: 'agent', owner_member_id: rootOwner.member_id, agent_id: root.id, starts_at: null, expires_at: null,
      reason: 'create the real root agent join code',
    }, 201);
    const childCode = await creator.write<{ id: string; code: string }>('/join-codes', {
      kind: 'agent', owner_member_id: child.member_id, agent_id: child.id, starts_at: null, expires_at: null,
      reason: 'create the real child agent join code',
    }, 201);

    const directMember = await registerMember(page.request, rootCode.code);
    const childMember = await registerMember(page.request, childCode.code);
    const directAttribution = await publicAPI(page.request, directMember.token).get<PublicAttribution>('/me/attribution');
    const childAttribution = await publicAPI(page.request, childMember.token).get<PublicAttribution>('/me/attribution');
    expectPublicAttribution(directAttribution, directMember.memberId, {
      join_method: 'agent_code', code_id: rootCode.id, source_code: rootCode.code,
    });
    expectPublicAttribution(childAttribution, childMember.memberId, {
      join_method: 'agent_code', code_id: childCode.id, source_code: childCode.code,
    });

    const operatorName = `attr_operator_${suffix()}`;
    const operatorPassword = `Attribution-${suffix()}-Operator9`;
    const operatorMember = await creator.write<{ member_id: string }>('/users', {
      username: operatorName, password: operatorPassword, reason: 'create operator-sourced betting member',
    }, 201);
    const operatorLogin = await loginMember(page.request, operatorName, operatorPassword);
    const operatorBetMember = { username: operatorName, password: operatorPassword, memberId: operatorMember.member_id, token: operatorLogin.access_token };
    const operatorAttribution = await publicAPI(page.request, operatorBetMember.token).get<PublicAttribution>('/me/attribution');
    expectPublicAttribution(operatorAttribution, operatorBetMember.memberId, { join_method: 'operator' });

    const bettors = [directMember, childMember, operatorBetMember];
    for (const member of bettors) {
      const recharge = await creator.write<{ id: string; version: number }>('/recharges', {
        member_id: member.memberId, points: '37', proof_reference: `attr-${suffix()}`,
        remark: 'owned attribution browser test funding', reason: 'fund real placed-order attribution fixture',
      }, 201);
      await creator.write(`/recharges/${recharge.id}/confirm`, { version: recharge.version, reason: 'confirm real test funding' });
    }

    const games = [
      await createBettingGame(creator, reviewer, `attr_${suffix()}`),
      await createBettingGame(creator, reviewer, `attr_${suffix()}`),
    ];
    const orders = [
      await placeBet(page.request, directMember, games[0]!), await placeBet(page.request, directMember, games[1]!),
      await placeBet(page.request, childMember, games[0]!), await placeBet(page.request, childMember, games[1]!),
      await placeBet(page.request, operatorBetMember, games[0]!),
    ];
    expect(new Set(orders).size).toBe(5);

    const disabledRootCode = await creator.put<{ status: string }>(`/join-codes/${rootCode.id}`, {
      version: rootCode.version, status: 'disabled', starts_at: null, expires_at: null,
      reason: 'disable new root-code joins after the immutable bet snapshots exist',
    });
    expect(disabledRootCode.status).toBe('disabled');

    const memberIds = [rootOwner.member_id, childOwner.member_id, directMember.memberId, childMember.memberId, operatorBetMember.memberId];
    const economicState = async () => Promise.all(memberIds.map(async memberId => ({
      wallet: await creator.get(`/wallets/${memberId}`),
      ledger: await creator.get(`/wallets/${memberId}/ledger?limit=100&offset=0`),
    })));
    const beforeRead = await economicState();
    expect(beforeRead.slice(2).map(snapshot => (snapshot.wallet as { available_points: string }).available_points)).toEqual(['35', '35', '36']);

    const errors: string[] = [];
    const adminMutations: string[] = [];
    const reportRequests: Array<{ method: string; pathname: string }> = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', browserRequest => {
      const url = new URL(browserRequest.url());
      if (url.origin !== origin || !url.pathname.startsWith('/api/v1/admin/')) return;
      if (browserRequest.method() !== 'GET') adminMutations.push(`${browserRequest.method()} ${url.pathname}`);
      if (url.pathname.startsWith('/api/v1/admin/reports/attribution')) reportRequests.push({ method: browserRequest.method(), pathname: url.pathname });
    });

    await page.addInitScript(() => localStorage.setItem('lottery.admin.locale', 'en'));
    await page.goto(origin!);
    await page.getByLabel('Select live admin brand', { exact: true }).selectOption(brand);
    if (info.project.name === 'mobile') {
      await page.locator('.mobile-nav').getByRole('button', { name: /More/ }).click();
      await page.locator('.mobile-more-menu').getByRole('button', { name: /Reports & reconciliation/ }).click();
    } else {
      await page.locator('.side-nav').getByRole('button', { name: /Reports & reconciliation/ }).click();
    }
    const panel = page.locator('.attribution-report');
    await expect(panel.getByRole('heading', { name: 'Attribution report', exact: true })).toBeVisible();
    const range = await page.evaluate(() => {
      const local = (date: Date) => {
        const pad = (value: number) => String(value).padStart(2, '0');
        return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
      };
      return { from: local(new Date(Date.now() - 2 * 60_000)), to: local(new Date(Date.now() + 5 * 60_000)) };
    });
    await panel.getByLabel('Start time', { exact: true }).fill(range.from);
    await panel.getByLabel('End time', { exact: true }).fill(range.to);
    await panel.getByLabel('Game UUID (optional)', { exact: true }).fill('');
    await panel.getByLabel('Member UUID (optional)', { exact: true }).fill('');
    await panel.getByLabel('Agent UUID (optional)', { exact: true }).fill(root.id);
    await panel.getByTestId('attribution-group').selectOption('game');
    await panel.getByTestId('attribution-agent-scope').selectOption('direct');
    await panel.getByTestId('attribution-join-method').selectOption('');
    const directResponse = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/admin/reports/attribution');
    await panel.getByTestId('attribution-query').click();
    const directHttpResponse = await directResponse;
    const direct = await data<{
      query: { group_by: string; agent_id: string | null; agent_scope: string; game_id: string | null; member_id: string | null; join_method: string | null };
      summary: { order_count: string; stake_points: string }; items: Array<{ key: string }>;
    }>(directHttpResponse);
    expect(direct.query).toMatchObject({ group_by: 'game', agent_id: root.id, agent_scope: 'direct', game_id: null, member_id: null, join_method: null });
    expect(direct.summary).toMatchObject({ order_count: '2', stake_points: '2' });
    expect(direct.items.map(item => item.key).sort()).toEqual(games.map(game => game.id).sort());
    const directURL = new URL(directHttpResponse.url());
    expect(directURL.searchParams.get('agent_scope')).toBe('direct');
    expect(directURL.searchParams.get('agent_id')).toBe(root.id);
    for (const filter of ['game_id', 'member_id', 'join_method']) expect(directURL.searchParams.has(filter)).toBe(false);
    await expect(panel.getByTestId('attribution-summary').locator('strong').first()).toHaveText('2');

    await panel.getByTestId('attribution-agent-scope').selectOption('downline');
    await panel.getByTestId('attribution-group').selectOption('agent');
    const downlineResponse = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/admin/reports/attribution');
    await panel.getByTestId('attribution-query').click();
    const downlineHttpResponse = await downlineResponse;
    const downline = await data<{
      query: { agent_id: string | null; agent_scope: string; group_by: string; game_id: string | null; member_id: string | null; join_method: string | null };
      summary: { order_count: string; stake_points: string; placed_count: string; unfinalized_stake_points: string };
      total_groups: string; items: Array<{ key: string; totals: { order_count: string; stake_points: string } }>;
    }>(downlineHttpResponse);
    expect(downline.query).toMatchObject({ agent_id: root.id, agent_scope: 'downline', group_by: 'agent', game_id: null, member_id: null, join_method: null });
    expect(downline.summary).toMatchObject({ order_count: '4', stake_points: '4', placed_count: '4', unfinalized_stake_points: '4' });
    expect(downline.total_groups).toBe('2');
    expect(downline.items.map(item => item.key).sort()).toEqual([root.id, child.id].sort());
    expect(downline.items.map(item => item.totals.order_count)).toEqual(['2', '2']);
    expect(downline.items.reduce((sum, item) => sum + BigInt(item.totals.stake_points), 0n)).toBe(4n);

    const downlineURL = new URL(downlineHttpResponse.url());
    expect(downlineURL.searchParams.get('agent_scope')).toBe('downline');
    expect(downlineURL.searchParams.get('group_by')).toBe('agent');
    expect(downlineURL.searchParams.has('game_id')).toBe(false);
    expect(downlineURL.searchParams.has('member_id')).toBe(false);
    expect(downlineURL.searchParams.has('join_method')).toBe(false);
    await panel.getByLabel('Game UUID (optional)', { exact: true }).fill(games[1]!.id);
    const exportResponsePromise = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/admin/reports/attribution/export');
    const downloadPromise = page.waitForEvent('download');
    await panel.getByTestId('attribution-export').click();
    const [exportResponse, download] = await Promise.all([exportResponsePromise, downloadPromise]);
    expect(exportResponse.status()).toBe(200);
    const headers = exportResponse.headers();
    expect(headers['cache-control']).toMatch(/no-store/i);
    expect(headers['x-report-format-version']).toBe('1');
    expect(headers['x-report-agent-scope']).toBe('downline');
    expect(headers['x-report-agent-id']).toBe(root.id);
    expect(headers['x-report-group-count']).toBe('2');
    expect(headers['x-report-audit-id']).toMatch(/^[0-9a-f-]{36}$/i);
    const exportURL = new URL(exportResponse.url());
    expect(exportURL.searchParams.get('agent_id')).toBe(root.id);
    expect(exportURL.searchParams.get('agent_scope')).toBe('downline');
    expect(exportURL.searchParams.get('group_by')).toBe('agent');
    expect(exportURL.searchParams.has('game_id')).toBe(false);
    expect(exportURL.searchParams.has('limit')).toBe(false);
    expect(exportURL.searchParams.has('offset')).toBe(false);
    const filePath = await download.path();
    expect(filePath).toBeTruthy();
    const bytes = readFileSync(filePath!);
    expect(createHash('sha256').update(bytes).digest('hex')).toBe(headers['x-report-sha256']);
    expect(String(bytes.length)).toBe(headers['x-report-byte-count']);
    expect(bytes.subarray(0, 3)).toEqual(Buffer.from([0xef, 0xbb, 0xbf]));
    const rows = parseCsv(bytes.subarray(3).toString('utf8'));
    expect(rows).toHaveLength(4);
    expect(rows[0]).toHaveLength(28);
    expect(rows[0]).not.toContain('legacy_attribution_count');
    expect(rows[1]![0]).toBe('summary');
    const orderCount = rows[0]!.indexOf('order_count');
    const stakePoints = rows[0]!.indexOf('stake_points');
    expect(rows[1]![orderCount]).toBe('4');
    expect(rows[1]![stakePoints]).toBe('4');
    const exportedGroups = rows.slice(2).filter(row => row[0] === 'group');
    expect(exportedGroups.map(row => row[12]).sort()).toEqual([root.id, child.id].sort());
    expect(exportedGroups.map(row => row[orderCount])).toEqual(['2', '2']);

    await panel.getByTestId('attribution-group').selectOption('join_method');
    await panel.getByTestId('attribution-agent-scope').selectOption('direct');
    await panel.getByTestId('attribution-join-method').selectOption('');
    await panel.getByLabel('Agent UUID (optional)', { exact: true }).fill('');
    await panel.getByLabel('Game UUID (optional)', { exact: true }).fill('');
    await panel.getByLabel('Member UUID (optional)', { exact: true }).fill('');
    const sourceResponse = page.waitForResponse(response => response.request().method() === 'GET' && new URL(response.url()).pathname === '/api/v1/admin/reports/attribution');
    await panel.getByTestId('attribution-query').click();
    const sourceHttpResponse = await sourceResponse;
    const sources = await data<{
      query: { group_by: string; agent_id: string | null; agent_scope: string; game_id: string | null; member_id: string | null; join_method: string | null };
      summary: { order_count: string }; items: Array<{ key: string; totals: { order_count: string } }>;
    }>(sourceHttpResponse);
    expect(sources.query).toMatchObject({ group_by: 'join_method', agent_id: null, agent_scope: 'direct', game_id: null, member_id: null, join_method: null });
    const sourceURL = new URL(sourceHttpResponse.url());
    expect(sourceURL.searchParams.get('agent_scope')).toBe('direct');
    expect(sourceURL.searchParams.get('group_by')).toBe('join_method');
    for (const filter of ['agent_id', 'game_id', 'member_id', 'join_method']) expect(sourceURL.searchParams.has(filter)).toBe(false);
    expect(sources.summary.order_count).toBe('5');
    expect(sources.items).toEqual(expect.arrayContaining([
      expect.objectContaining({ key: 'agent_code', totals: expect.objectContaining({ order_count: '4' }) }),
      expect.objectContaining({ key: 'operator', totals: expect.objectContaining({ order_count: '1' }) }),
    ]));

    expect(await economicState()).toEqual(beforeRead);
    expect(adminMutations).toEqual([]);
    expect(reportRequests.length).toBeGreaterThanOrEqual(4);
    expect(reportRequests.every(item => item.method === 'GET')).toBe(true);
    expect(errors).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    mkdirSync('.local', { recursive: true });
    await page.screenshot({ path: `.local/attribution-reports-${info.project.name}.png`, fullPage: true });
  } finally {
    await reviewerRequest.dispose();
  }
});
