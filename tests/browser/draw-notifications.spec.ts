import { expect, test, type APIRequestContext, type APIResponse, type Page } from '@playwright/test';

const brand = '0199a000-0000-7000-8000-000000000002';
const viewport = process.env.REPORT_ATTRIBUTION_VIEWPORT;
const adminOrigin = process.env.TEST_DRAW_NOTICE_ADMIN_ORIGIN;
const configuredUserOrigin = process.env.TEST_DRAW_NOTICE_USER_ORIGIN;
const username = process.env.TEST_ATTRIBUTION_ADMIN_USERNAME;
const password = process.env.TEST_ATTRIBUTION_ADMIN_PASSWORD;
const confirmation = 'owned_synthetic_database';
const id = () => crypto.randomUUID();
const suffix = () => id().replaceAll('-', '').slice(0, 12);

function assertOwnedFixture(infoProject: string) {
  if (
    !['desktop', 'mobile'].includes(viewport ?? '') ||
    process.env.APP_ENV !== 'test' ||
    process.env.REPORT_ATTRIBUTION_FIXTURE_CONFIRM !== confirmation
  ) {
    throw new Error('Draw notification browser tests require the explicitly owned desktop/mobile test fixture');
  }
  const database = `lottery_attribution_browser_${viewport}`;
  if (!new RegExp(`^postgres(?:ql)?:\\/\\/lottery_test@127\\.0\\.0\\.1:(?:5432|55432)\\/${database}\\?sslmode=disable$`).test(process.env.DATABASE_URL ?? '')) {
    throw new Error('Draw notification browser tests require their exact owned loopback database');
  }
  if (['DATABASE_READ_URL', 'DATABASE_READ_URLS'].some((name) => process.env[name])) {
    throw new Error('Draw notification browser tests forbid read database overrides');
  }
  if (
    !/^http:\/\/localhost:(?:5174|15292)$/.test(adminOrigin ?? '') ||
    !/^http:\/\/localhost:(?:5173|15293)$/.test(configuredUserOrigin ?? '') ||
    username !== `attribution_browser_${viewport}` ||
    !password
  ) {
    throw new Error('Draw notification browser tests require matching local origins and the isolated fixture administrator');
  }
  expect(infoProject).toBe(viewport);
}

const apiAdminOrigin = () => adminOrigin!;
const platformUserOrigin = () => configuredUserOrigin!;
const harborUserOrigin = () => configuredUserOrigin!.replace('://localhost', '://harbor.localhost');
const adminBase = () => `${apiAdminOrigin()}/api/v1/admin`;
const publicBase = () => `${platformUserOrigin()}/api/v1/b/harbor`;
type Envelope<T> = { success: boolean; data: T; error?: unknown };

async function data<T>(response: Pick<APIResponse, 'text' | 'status'>, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body) as Envelope<T>;
  expect(envelope.success, body).toBe(true);
  return envelope.data;
}

async function adminLogin(client: APIRequestContext, name: string, secret: string) {
  return data<{ access_token: string }>(await client.post(`${adminBase()}/auth/login`, {
    headers: { Origin: apiAdminOrigin(), 'X-Brand-ID': brand, 'Idempotency-Key': id() },
    data: { identifier: name, password: secret },
  }));
}

function adminAPI(client: APIRequestContext, token: string) {
  const headers = { Authorization: `Bearer ${token}`, Origin: apiAdminOrigin(), 'X-Brand-ID': brand };
  return {
    get: async <T>(path: string) => data<T>(await client.get(`${adminBase()}${path}`, { headers })),
    post: async <T>(path: string, body: unknown, status = 200) => data<T>(await client.post(`${adminBase()}${path}`, {
      headers: { ...headers, 'Idempotency-Key': id() }, data: body,
    }), status),
    put: async <T>(path: string, body: unknown) => data<T>(await client.put(`${adminBase()}${path}`, {
      headers: { ...headers, 'Idempotency-Key': id() }, data: body,
    })),
  };
}

function publicAPI(client: APIRequestContext, token: string) {
  const headers = { Authorization: `Bearer ${token}`, Origin: platformUserOrigin() };
  return {
    get: async <T>(path: string) => data<T>(await client.get(`${publicBase()}${path}`, { headers })),
    post: async <T>(path: string, body: unknown, status = 200) => data<T>(await client.post(`${publicBase()}${path}`, {
      headers: { ...headers, 'Idempotency-Key': id() }, data: body,
    }), status),
  };
}

async function registerMember(client: APIRequestContext) {
  const auth = await data<{
    terms: { privacy_policy_version: string; service_terms_version: string };
    auth?: { captcha_enabled?: boolean };
  }>(await client.get(`${publicBase()}/context`));
  let captcha: Record<string, string> = {};
  if (auth.auth?.captcha_enabled) {
    const challenge = await data<{ id: string; svg: string }>(await client.get(`${publicBase()}/auth/challenge`));
    const answer = challenge.svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? '';
    expect(answer).toMatch(/^[A-Za-z0-9]+$/);
    captcha = { captcha_id: challenge.id, captcha_answer: answer };
  }
  const name = `draw_notice_${suffix()}`;
  const secret = `DrawNotice-${suffix()}-Member9`;
  const registered = await data<{ access_token: string; member: { id: string } }>(await client.post(`${publicBase()}/auth/register`, {
    headers: { Origin: platformUserOrigin(), 'Idempotency-Key': id() },
    data: {
      username: name,
      password: secret,
      ...auth.terms,
      ...captcha,
    },
  }), 201);
  return { ...registered, username: name, password: secret };
}

const digitsModel = {
  model: 'DIGITS_0_9',
  regular_pool: { min: 0, max: 0, values: [], allow_repeat: false },
  special_pool: { min: 0, max: 0, values: [], allow_repeat: false },
  regular_count: 0,
  special_count: 0,
  pool_size: 0,
  total_count: 0,
  length: 3,
  allow_repeat: true,
  ordered: true,
};
const digitsDefinition = {
  schema_version: 1,
  model: digitsModel,
  selection: { mode: 'numbers', regular_count: 0, special_count: 0, exclude_count: 0, attribute_groups: [], feature_choices: {} },
  number_attributes: {},
  unit_points: '1',
  prize_tiers: [{ code: 'MATCH_ALL_DIGITS', condition: { op: 'equals', field: 'position_match', value: 3 }, odds: '10', exclusive: true, cap_points: null }],
  mixed_tier_policy: 'max_all',
  cap_points: null,
  rounding: 'half_up',
  rounding_scope: 'order',
  limits: { max_combinations: 1000, max_multiplier: '1000', max_bet_points: null },
};
const winningCase = {
  name: 'three ordered digits match',
  selection: { regular: [], special: [], digits: [[1], [0], [1]], exclude: [], attributes: {}, features: {} },
  draw: { regular: [], special: [], digits: [1, 0, 1] },
  multiplier: '1',
  expected_bet_points: '1',
  expected_prize_points: '10',
  expected_won: true,
};
const losingSelection = { regular: [], special: [], digits: [[9], [9], [9]], exclude: [], attributes: {}, features: {} };

type Member = Awaited<ReturnType<typeof registerMember>>;

async function placeBet(client: APIRequestContext, member: Member, game: { id: string; playId: string; periodId: string }) {
  const api = publicAPI(client, member.access_token);
  await expect.poll(async () => (await api.get<{ period: { status: string } | null }>(`/games/${game.id}`)).period?.status, {
    timeout: 8_000,
    intervals: [200, 400, 800],
  }).toBe('betting');
  const catalog = await api.get<{
    period: { id: string; status: string } | null;
    plays: Array<{ id: string; rule_version_id: string }>;
    policy_versions: { brand: number; game: number };
  }>(`/games/${game.id}`);
  expect(catalog.period).toMatchObject({ id: game.periodId, status: 'betting' });
  const play = catalog.plays.find((entry) => entry.id === game.playId);
  expect(play).toBeTruthy();
  const input = {
    period_id: game.periodId,
    play_id: game.playId,
    rule_version_id: play!.rule_version_id,
    selection: losingSelection,
    multiplier: '1',
    policy_versions: catalog.policy_versions,
  };
  const preview = await api.post<{ actor_context: string }>('/bet-previews', input);
  const order = await api.post<{ id: string; status: string; total_points: string }>('/bet-orders', {
    ...input,
    actor_context: preview.actor_context,
  }, 201);
  expect(order).toMatchObject({ status: 'placed', total_points: '1' });
  return order.id;
}

type DrawFact = {
  game_id: string;
  period_id: string;
  period_no: string;
  result: { regular: number[]; special: number[]; digits: number[] };
  drawn_at: string;
  previous_draw_id: string | null;
};
type DrawNotice = {
  id: string;
  event_type: 'draw.result.published' | 'draw.result.corrected';
  template_key: string;
  template_version: number;
  content: { en: { title: string; body: string }; 'zh-CN': { title: string; body: string } } | null;
  payload: { resource_id: string; points: string | null; draw: DrawFact };
  read_at: string | null;
};

function expectDrawSnapshot(notice: DrawNotice, expected: {
  event: DrawNotice['event_type'];
  drawId: string;
  gameId: string;
  periodId: string;
  periodNo: string;
  digits: number[];
  previousDrawId: string | null;
}) {
  expect(notice.event_type).toBe(expected.event);
  expect(notice.template_key).toBe(expected.event);
  expect(notice.template_version).toBeGreaterThan(0);
  expect(notice.content).not.toBeNull();
  expect(Object.keys(notice.payload).sort()).toEqual(['draw', 'points', 'resource_id']);
  expect(notice.payload.resource_id).toBe(expected.drawId);
  expect(notice.payload.points).toBeNull();
  expect(Object.keys(notice.payload.draw).sort()).toEqual(['drawn_at', 'game_id', 'period_id', 'period_no', 'previous_draw_id', 'result']);
  expect(notice.payload.draw).toMatchObject({
    game_id: expected.gameId,
    period_id: expected.periodId,
    period_no: expected.periodNo,
    previous_draw_id: expected.previousDrawId,
  });
  expect(notice.payload.draw.result).toEqual({ regular: [], special: [], digits: expected.digits });
  expect(Number.isNaN(Date.parse(notice.payload.draw.drawn_at))).toBe(false);
  const serialized = JSON.stringify(notice);
  expect(serialized).not.toMatch(/recipient_id|audit_log_id|created_by|private reason/i);
}

async function inbox(client: APIRequestContext, member: Member) {
  return publicAPI(client, member.access_token).get<{ items: DrawNotice[]; unread_count: string }>('/notifications?limit=100');
}

async function walletFingerprint(client: APIRequestContext, api: ReturnType<typeof adminAPI>, memberId: string) {
  const wallet = await api.get(`/wallets/${memberId}`);
  const ledger = await api.get(`/wallets/${memberId}/ledger?limit=100&offset=0`);
  return JSON.stringify({ wallet, ledger });
}

async function screenshotAndCheck(page: Page, path: string) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path, fullPage: true });
}

test('real Harbor draw notices publish and correct immutable facts on desktop and mobile', async ({ page, context }, info) => {
  test.setTimeout(60_000);
  page.setDefaultTimeout(7_000);
  assertOwnedFixture(info.project.name);

  const pageErrors: string[] = [];
  page.on('pageerror', (error) => pageErrors.push(error.message));
  const creatorAuth = await adminLogin(page.request, username!, password!);
  const creator = adminAPI(page.request, creatorAuth.access_token);

  const reviewerRole = await creator.post<{ id: string }>('/roles', {
    code: `draw_notice_review_${suffix()}`,
    name: 'Draw notice fixture reviewer',
    status: 'active',
    permissions: ['rule.view.brand', 'rule.review.brand'],
    reason: 'Create a distinct reviewer for the genuine draw notice rule',
  }, 201);
  const reviewerName = `draw_review_${suffix()}`;
  expect(reviewerName).toMatch(/^[a-z][a-z0-9_]{2,31}$/);
  const reviewerPassword = `DrawNotice-${suffix()}-Reviewer9`;
  await creator.post('/accounts', {
    username: reviewerName,
    password: reviewerPassword,
    role_ids: [reviewerRole.id],
    reason: 'Create a distinct reviewer for the genuine draw notice rule',
  }, 201);
  const reviewerAuth = await adminLogin(page.request, reviewerName, reviewerPassword);
  const reviewer = adminAPI(page.request, reviewerAuth.access_token);

  const nonBettor = await registerMember(page.request);
  // Leave the browser's ordinary Harbor session cookie attached to the bettor
  // that will open the inbox; API-only assertions use each member's own token.
  const bettor = await registerMember(page.request);
  const recharge = await creator.post<{ id: string; version: number }>('/recharges', {
    member_id: bettor.member.id,
    points: '37',
    proof_reference: `draw-notice-${suffix()}`,
    remark: 'owned synthetic draw notification browser funding',
    reason: 'Fund the real draw notice bettor with synthetic test points',
  }, 201);
  await creator.post(`/recharges/${recharge.id}/confirm`, {
    version: recharge.version,
    reason: 'Confirm synthetic points for real draw notice bets',
  });

  const game = await creator.post<{ id: string; version: number }>('/games', {
    code: `draw_notice_${suffix()}`,
    name: 'Draw notification browser fixture',
    model: digitsModel,
    timezone: 'UTC',
    reason: 'Create a genuine three-digit game for draw notification browser coverage',
  }, 201);
  const play = await creator.post<{ id: string }>('/games/' + game.id + '/plays', {
    code: 'position_match',
    name: 'Position match',
    reason: 'Create a genuine three-digit play for draw notification coverage',
  }, 201);
  const rule = await creator.post<{ id: string; version: number }>('/rule-versions', {
    play_id: play.id,
    definition: digitsDefinition,
    effect_mode: 'immediate',
    reason: 'Create a genuine reviewed three-digit rule for draw notification coverage',
  }, 201);
  const validation = await creator.post<{ version: number; validation: { passed: boolean } }>(`/rule-versions/${rule.id}/validate`, {
    version: rule.version,
    cases: [winningCase],
    reason: 'Validate the real ordered-digit rule through the standard workflow',
  });
  expect(validation.validation.passed).toBe(true);
  const submitted = await creator.post<{ version: number }>(`/rule-versions/${rule.id}/submit-review`, {
    version: validation.version,
    reason: 'Submit the real ordered-digit rule for independent review',
  });
  await reviewer.post(`/rule-versions/${rule.id}/approve`, {
    version: submitted.version,
    reason: 'Independently approve the real ordered-digit rule',
    warnings_acknowledged: true,
  });

  const drawAt = new Date(Date.now() + 20_000);
  drawAt.setUTCMilliseconds(0);
  await creator.put(`/games/${game.id}/schedule`, {
    version: game.version,
    reason: 'Open the controlled near-future real draw window',
    spec: {
      timezone: 'UTC',
      mode: 'daily',
      daily_draw_times: [drawAt.toISOString().slice(11, 19)],
      interval_seconds: 0,
      busy_windows: [],
      bet_open_before_seconds: 60,
      bet_close_before_seconds: 1,
      pause_dates: [],
      weekdays: [0, 1, 2, 3, 4, 5, 6],
      holiday_dates: [],
      holiday_policy: 'normal',
    },
  });
  const generated = await creator.post<{ periods: Array<{ id: string; status: string }> }>(`/games/${game.id}/periods/generate`, {
    from: new Date(Date.now() - 1_000).toISOString(),
    to: new Date(drawAt.valueOf() + 1_000).toISOString(),
    reason: 'Generate the single controlled real draw period',
  });
  expect(generated.periods).toHaveLength(1);
  const periodId = generated.periods[0]!.id;
  const gameInfo = { id: game.id, playId: play.id, periodId };
  const orderIds = [await placeBet(page.request, bettor, gameInfo), await placeBet(page.request, bettor, gameInfo)];
  expect(new Set(orderIds).size).toBe(2);
  await expect.poll(async () => (await creator.get<{ status: string }>(`/periods/${periodId}`)).status, {
    timeout: Math.max(1_000, drawAt.valueOf() - Date.now() + 8_000),
    intervals: [200, 400, 800],
  }).toBe('waiting_draw');

  const beforeDraw = await creator.get<{ id: string; period_no: string; version: number; status: string }>(`/periods/${periodId}`);
  expect(beforeDraw.status).toBe('waiting_draw');
  const originalDraw = await creator.post<{ id: string }>(`/periods/${periodId}/manual-draw`, {
    version: beforeDraw.version,
    period_no: beforeDraw.period_no,
    result: { regular: [], special: [], digits: [0, 1, 0] },
    drawn_at: drawAt.toISOString(),
    reason: 'Publish the controlled original 010 draw result',
  }, 201);
  const originalHistory = await creator.get<{ current: { id: string; result: { digits: number[] } } | null; history: Array<{ id: string; result: { digits: number[] } }> }>(`/periods/${periodId}/draw`);
  expect(originalHistory.current).toMatchObject({ id: originalDraw.id, result: { digits: [0, 1, 0] } });
  const drawMessages = async () => (await inbox(page.request, bettor)).items.filter((item) => item.event_type.startsWith('draw.result.'));
  await expect.poll(async () => (await drawMessages()).filter((item) => item.event_type === 'draw.result.published').length, {
    timeout: 5_000,
    intervals: [150, 300, 500],
  }).toBe(1);
  const publishedBeforeCorrection = (await drawMessages()).find((item) => item.event_type === 'draw.result.published');
  expect(publishedBeforeCorrection).toBeTruthy();

  const periodBeforeCorrection = await creator.get<{ id: string; period_no: string; version: number; status: string }>(`/periods/${periodId}`);
  expect(periodBeforeCorrection.status).toBe('drawn');
  const correction = await creator.post<{
    id: string;
    previous_draw_result_id: string;
    draw_result_id: string;
    result: { digits: number[] };
    state: string;
  }>(`/draw-results/${originalDraw.id}/correct`, {
    version: periodBeforeCorrection.version,
    policy_version: null,
    result: { regular: [], special: [], digits: [1, 0, 1] },
    reason: 'Correct the published result to the verified 101 digits',
  }, 201);
  expect(correction).toMatchObject({
    previous_draw_result_id: originalDraw.id,
    result: { regular: [], special: [], digits: [1, 0, 1] },
    state: 'completed',
  });
  expect(correction.draw_result_id).not.toBe(originalDraw.id);
  const correctedHistory = await creator.get<{ current: { id: string; result: { digits: number[] } } | null; history: Array<{ id: string; corrected_from_id: string; result: { digits: number[] } }> }>(`/periods/${periodId}/draw`);
  expect(correctedHistory.current).toMatchObject({ id: correction.draw_result_id, result: { digits: [1, 0, 1] } });
  expect(correctedHistory.history.find((entry) => entry.id === originalDraw.id)).toMatchObject({ result: { digits: [0, 1, 0] } });
  expect(correctedHistory.history.find((entry) => entry.id === correction.draw_result_id)).toMatchObject({ corrected_from_id: originalDraw.id });

  await expect.poll(async () => (await drawMessages()).length, { timeout: 7_000, intervals: [150, 300, 500] }).toBe(2);
  const originalInboxSnapshot = await inbox(page.request, bettor);
  const originalNotice = originalInboxSnapshot.items.find((item) => item.event_type === 'draw.result.published');
  const correctedNotice = originalInboxSnapshot.items.find((item) => item.event_type === 'draw.result.corrected');
  expect(originalNotice).toBeTruthy();
  expect(correctedNotice).toBeTruthy();
  expect(originalNotice).toEqual(publishedBeforeCorrection);
  expectDrawSnapshot(originalNotice!, {
    event: 'draw.result.published',
    drawId: originalDraw.id,
    gameId: game.id,
    periodId,
    periodNo: beforeDraw.period_no,
    digits: [0, 1, 0],
    previousDrawId: null,
  });
  expectDrawSnapshot(correctedNotice!, {
    event: 'draw.result.corrected',
    drawId: correction.draw_result_id,
    gameId: game.id,
    periodId,
    periodNo: beforeDraw.period_no,
    digits: [1, 0, 1],
    previousDrawId: originalDraw.id,
  });
  expect((await inbox(page.request, nonBettor)).items.filter((item) => item.event_type.startsWith('draw.result.'))).toHaveLength(0);

  const orders = await Promise.all(orderIds.map((orderId) => creator.get<{ id: string; status: string; total_points: string; prize_points: string }>(`/bet-orders/${orderId}`)));
  expect(orders).toHaveLength(2);
  for (const order of orders) expect(order).toMatchObject({ status: 'placed', total_points: '1', prize_points: '0' });
  const finalWalletAndLedger = await walletFingerprint(page.request, creator, bettor.member.id);
  // The admin GET /wallets/{memberID} returns the points.Wallet DTO directly.
  const wallet = await creator.get<{ available_points: string }>(`/wallets/${bettor.member.id}`);
  expect(wallet.available_points).toBe('35');

  const userCookies = (await context.cookies(publicBase())).filter((cookie) => cookie.name === `lottery_user_${brand.replaceAll('-', '')}`);
  expect(userCookies).toHaveLength(1);
  await context.addCookies(userCookies.map((cookie) => ({ ...cookie, domain: 'harbor.localhost' })));
  const userPage = await context.newPage();
  userPage.setDefaultTimeout(7_000);
  userPage.on('pageerror', (error) => pageErrors.push(error.message));
  await userPage.route(`${harborUserOrigin()}/api/**`, async (route) => {
    const response = await route.fetch({
      url: route.request().url().replace('harbor.localhost', 'localhost'),
      headers: { ...(await route.request().allHeaders()), host: new URL(harborUserOrigin()).host },
    });
    await route.fulfill({ response });
  });
  await userPage.goto(`${harborUserOrigin()}/notifications`);
  const panel = userPage.locator('.notifications-panel');
  const originalTitle = originalNotice!.content!.en.title.replaceAll('{resource_id}', originalDraw.id);
  const correctedTitle = correctedNotice!.content!.en.title.replaceAll('{resource_id}', correction.draw_result_id);
  await expect(panel.getByRole('heading', { name: originalTitle, exact: true })).toBeVisible();
  await expect(panel.getByRole('heading', { name: correctedTitle, exact: true })).toBeVisible();
  await expect(panel).toContainText(`Digits: 010`);
  await expect(panel).toContainText('does not indicate a win, prize payment, or guarantee of the current result');
  await expect(panel).toContainText('Digits: 101');
  await expect(panel).not.toContainText('draw_notice_');
  await userPage.getByRole('button', { name: 'Switch to Chinese', exact: true }).click();
  await expect(panel.getByRole('heading', { name: originalNotice!.content!['zh-CN'].title.replaceAll('{resource_id}', originalDraw.id), exact: true })).toBeVisible();
  await expect(panel.getByRole('heading', { name: correctedNotice!.content!['zh-CN'].title.replaceAll('{resource_id}', correction.draw_result_id), exact: true })).toBeVisible();
  await expect(panel).toContainText('数字：010');
  await expect(panel).toContainText('数字：101');
  await expect(panel).toContainText('不代表中奖、派奖或当前结果保证');
  const originalCard = panel.locator('.notification-card').filter({
    hasText: originalNotice!.payload.resource_id,
  });
  const correctedCard = panel.locator('.notification-card').filter({
    hasText: correctedNotice!.payload.resource_id,
  });
  await expect(originalCard).toHaveCount(1);
  await expect(correctedCard).toHaveCount(1);
  expect(await userPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);

  const readWrites: Array<{ body: string | null; key: string | undefined }> = [];
  await userPage.route(`${harborUserOrigin()}/api/v1/notifications/read`, async (route) => {
    readWrites.push({ body: route.request().postData(), key: route.request().headers()['idempotency-key'] });
    const response = await route.fetch({
      url: route.request().url().replace('harbor.localhost', 'localhost'),
      headers: { ...(await route.request().allHeaders()), host: new URL(harborUserOrigin()).host },
    });
    expect(response.status(), await response.text()).toBe(200);
    await route.fulfill({ response });
  });
  await userPage.getByRole('button', { name: 'Switch to English', exact: true }).click();
  await originalCard.getByRole('button', { name: 'Mark as read', exact: true }).click();
  await correctedCard.getByRole('button', { name: 'Mark as read', exact: true }).click();
  await expect.poll(async () => (await drawMessages()).filter((item) => item.read_at !== null).length).toBe(2);
  expect(readWrites).toHaveLength(2);
  for (const write of readWrites) expect(write.key).toBeTruthy();
  const readIds = readWrites.flatMap((write) => (JSON.parse(write.body ?? '{}') as { ids: string[] }).ids);
  expect(readIds.sort()).toEqual([originalNotice!.id, correctedNotice!.id].sort());
  await panel.getByRole('button', { name: 'Reload', exact: true }).click();
  await expect(panel.locator('.notification-card.read-card')).toHaveCount(2);
  const repeat = await page.request.post(`${publicBase()}/notifications/read`, {
    headers: { Origin: platformUserOrigin(), 'Idempotency-Key': id() },
    data: { ids: readIds },
  });
  const repeatedReceipt = await data<{ changed: number; ids: string[]; unread_count: string }>(repeat);
  expect(repeatedReceipt).toMatchObject({ changed: 0, ids: readIds });
  const afterRepeatedRead = await inbox(page.request, bettor);
  expect(afterRepeatedRead.items.find((item) => item.id === originalNotice!.id)).toEqual(expect.objectContaining({
    payload: originalNotice!.payload,
    content: originalNotice!.content,
    read_at: expect.any(String),
  }));
  expect(afterRepeatedRead.items.find((item) => item.id === correctedNotice!.id)).toEqual(expect.objectContaining({
    payload: correctedNotice!.payload,
    content: correctedNotice!.content,
    read_at: expect.any(String),
  }));
  expect(await walletFingerprint(page.request, creator, bettor.member.id)).toBe(finalWalletAndLedger);
  await screenshotAndCheck(userPage, info.outputPath(`draw-notifications-${info.project.name}.png`));
  expect(pageErrors).toEqual([]);
  await userPage.close();
});
