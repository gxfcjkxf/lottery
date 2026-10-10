import {
  test,
  expect,
  type APIRequestContext,
  type Page,
  type Route,
} from "@playwright/test";
import type {
  AdminBetOrder,
  BetException,
  BrandBetPolicy,
  GameBetPolicy,
} from "../../admin-web/src/bet-management-api";
import type { LedgerEntry, Wallet } from "../../admin-web/src/finance-api";
import type { Cancellation } from "../../admin-web/src/period-cancellation-api";
import type { Period } from "../../admin-web/src/period-schedules-api";

const brandId = "0199a000-0000-7000-8000-000000000002";
const apiOrigin = process.env.TEST_BET_USER_ORIGIN ?? "http://localhost:5173";
const adminOrigin = process.env.TEST_BET_ADMIN_ORIGIN ?? "http://localhost:5174";
// Port overrides are only for owned local test servers, never remote money.
for (const origin of [apiOrigin, adminOrigin]) {
  const url = new URL(origin);
  if (url.protocol !== "http:" || url.hostname !== "localhost" || url.username || url.password || url.pathname !== "/" || url.search || url.hash || origin !== url.origin)
    throw new Error("Betting browser tests require plain localhost HTTP origins");
}
const adminBase = `${apiOrigin}/api/v1/admin`;
const publicBase = `${apiOrigin}/api/v1/b/harbor`;
const harborSite = apiOrigin.replace("://localhost", "://harbor.localhost");
const unique = () => crypto.randomUUID().replaceAll("-", "").slice(0, 12);

// Node's fetch cannot resolve *.localhost while Chromium can. Send the actual
// intercepted request through loopback, preserving its registered HTTP authority,
// Origin, Cookie and body; the real server still authenticates the Harbor member.
async function fetchHarbor(route: Route) {
  return route.fetch({
    url: route.request().url().replace("harbor.localhost", "localhost"),
    headers: {
      ...(await route.request().allHeaders()),
      host: new URL(harborSite).host,
    },
  });
}

type Envelope<T> = { success: boolean; data: T; error?: unknown };
type RuleFixture = {
  code: string;
  name: string;
  model: Record<string, unknown>;
  mode: "numbers" | "exclude" | "attributes" | "features";
  definition: Record<string, unknown>;
  validationCase: Record<string, unknown>;
  uiSelection: Record<string, unknown>;
};

async function api<T>(
  request: APIRequestContext,
  url: string,
  method: "GET" | "POST" | "PUT",
  token: string,
  body?: unknown,
  status = 200,
) {
  const response = await request.fetch(url, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      "X-Brand-ID": brandId,
      Origin: apiOrigin,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      ...(method === "GET" ? {} : { "Idempotency-Key": crypto.randomUUID() }),
    },
    ...(body === undefined ? {} : { data: body }),
  });
  const text = await response.text();
  expect(response.status(), `${method} ${url}: ${text}`).toBe(status);
  const envelope = JSON.parse(text) as Envelope<T>;
  expect(envelope.success, `${method} ${url}: ${text}`).toBe(true);
  return envelope.data;
}

async function adminToken(
  request: APIRequestContext,
  username: string,
  password: string,
) {
  const response = await request.post(`${apiOrigin}/api/v1/admin/auth/login`, {
    headers: {
      "X-Brand-ID": brandId,
      Origin: apiOrigin,
      "Idempotency-Key": crypto.randomUUID(),
    },
    data: { identifier: username, password },
  });
  const text = await response.text();
  expect(response.status(), `Harbor admin sign-in: ${text}`).toBe(200);
  const envelope = JSON.parse(text) as Envelope<{ access_token: string }>;
  expect(envelope.success).toBe(true);
  return envelope.data.access_token;
}

function answerSvgChallenge(svg: string) {
  return svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? "";
}

function modelFor(type: "X_PLUS_Y" | "M_SELECT_N" | "DIGITS_0_9") {
  if (type === "DIGITS_0_9") {
    return {
      model: type,
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
  }
  const regularPool =
    type === "M_SELECT_N"
      ? { min: 0, max: 0, values: [1, 2, 3, 4, 5, 6], allow_repeat: false }
      : { min: 1, max: 6, values: [], allow_repeat: false };
  const specialPool =
    type === "M_SELECT_N"
      ? { min: 0, max: 0, values: [1, 2, 3, 4, 5, 6], allow_repeat: false }
      : { min: 10, max: 12, values: [], allow_repeat: false };
  return {
    model: type,
    regular_pool: regularPool,
    special_pool: specialPool,
    regular_count: 2,
    special_count: 1,
    pool_size: type === "M_SELECT_N" ? 6 : 0,
    total_count: type === "M_SELECT_N" ? 3 : 0,
    length: 0,
    allow_repeat: false,
    ordered: false,
  };
}

function fixture(
  code: string,
  modelType: "X_PLUS_Y" | "M_SELECT_N" | "DIGITS_0_9",
  mode: RuleFixture["mode"],
): RuleFixture {
  const model = modelFor(modelType);
  const selectionRule = {
    mode,
    regular_count: mode === "numbers" && modelType !== "DIGITS_0_9" ? 2 : 0,
    special_count: mode === "numbers" && modelType !== "DIGITS_0_9" ? 1 : 0,
    exclude_count: mode === "exclude" ? 1 : 0,
    attribute_groups: mode === "attributes" ? ["color"] : [],
    feature_choices: mode === "features" ? { odd_count: [0, 1, 2, 3] } : {},
  };
  const numberAttributes =
    mode === "attributes" ? { color: { red: [1, 2, 3], blue: [4, 5, 6] } } : {};
  let condition: Record<string, unknown>;
  let validationSelection: Record<string, unknown>;
  let draw: Record<string, unknown>;
  let prize = "5";
  let betPoints = "1";
  if (modelType === "DIGITS_0_9" && mode === "numbers") {
    condition = { op: "equals", field: "position_match", value: 3 };
    validationSelection = {
      regular: [],
      special: [],
      digits: [[1], [2], [1]],
      exclude: [],
      attributes: {},
      features: {},
    };
    draw = { regular: [], special: [], digits: [1, 2, 1] };
    prize = "8";
  } else if (mode === "features") {
    condition = {
      op: "equals",
      field: "draw_odd_count",
      target: "digits",
      value: 2,
    };
    validationSelection = {
      regular: [],
      special: [],
      digits: [],
      exclude: [],
      attributes: {},
      features: { odd_count: [2] },
    };
    draw = { regular: [], special: [], digits: [1, 2, 3] };
    prize = "3";
  } else if (mode === "exclude") {
    condition = {
      op: "equals",
      field: "excluded_match",
      target: "regular",
      value: 0,
    };
    validationSelection = {
      regular: [],
      special: [],
      digits: [],
      exclude: [6],
      attributes: {},
      features: {},
    };
    draw = { regular: [1, 2], special: [10], digits: [] };
    prize = "7";
  } else if (mode === "attributes") {
    condition = {
      op: "equals",
      field: "attribute_match",
      target: "regular",
      attribute_group: "color",
      attribute_value: "$selection",
      value: 2,
    };
    validationSelection = {
      regular: [],
      special: [],
      digits: [],
      exclude: [],
      attributes: { color: ["red"] },
      features: {},
    };
    draw = { regular: [1, 2], special: [10], digits: [] };
    prize = "9";
  } else {
    condition = {
      op: "all",
      children: [
        { op: "equals", field: "regular_match", value: 2 },
        { op: "equals", field: "special_match", value: 1 },
      ],
    };
    const special = modelType === "M_SELECT_N" ? 3 : 10;
    validationSelection = {
      regular: [1, 2],
      special: [special],
      digits: [],
      exclude: [],
      attributes: {},
      features: {},
    };
    draw = { regular: [1, 2], special: [special], digits: [] };
    prize = "35";
  }
  const definition = {
    schema_version: 1,
    model,
    selection: selectionRule,
    number_attributes: numberAttributes,
    unit_points: "1",
    prize_tiers: [
      {
        code: `WIN_${mode.toUpperCase()}`,
        condition,
        odds: prize,
        exclusive: true,
        cap_points: null,
      },
    ],
    mixed_tier_policy: "max_all",
    cap_points: null,
    rounding: "half_up",
    rounding_scope: "order",
    limits: {
      max_combinations: 1000,
      max_multiplier: "1000",
      max_bet_points: null,
    },
  };
  const validationCase = {
    name: `${code} server validation`,
    selection: validationSelection,
    draw,
    multiplier: "1",
    expected_bet_points: betPoints,
    expected_prize_points: prize,
    expected_won: true,
  };
  let uiSelection = validationSelection;
  if (modelType === "DIGITS_0_9" && mode === "numbers") {
    uiSelection = {
      regular: [],
      special: [],
      digits: [
        [1, 2],
        [1, 2],
        [1, 2],
      ],
      exclude: [],
      attributes: {},
      features: {},
    };
  } else if (mode === "features") {
    uiSelection = {
      regular: [],
      special: [],
      digits: [],
      exclude: [],
      attributes: {},
      features: { odd_count: [1, 2] },
    };
  }
  return {
    code,
    name: `${code} ${mode}`,
    model,
    mode,
    definition,
    validationCase,
    uiSelection,
  };
}

async function provisionGame(
  request: APIRequestContext,
  admin: string,
  reviewer: string,
  gameCode: string,
  name: string,
  model: Record<string, unknown>,
  plays: RuleFixture[],
) {
  const game = await api<{ id: string; version: number }>(
    request,
    `${adminBase}/games`,
    "POST",
    admin,
    {
      code: gameCode,
      name,
      model,
      timezone: "UTC",
      reason: "real browser betting E2E fixture",
    },
    201,
  );
  const published: Array<{ id: string; gameId: string; mode: string }> = [];
  for (const playFixture of plays) {
    const play = await api<{ id: string }>(
      request,
      `${adminBase}/games/${game.id}/plays`,
      "POST",
      admin,
      {
        code: playFixture.code,
        name: playFixture.name,
        reason: "betting UI coverage",
      },
      201,
    );
    let version = await api<{ id: string; version: number }>(
      request,
      `${adminBase}/rule-versions`,
      "POST",
      admin,
      {
        play_id: play.id,
        definition: playFixture.definition,
        effect_mode: "immediate",
        reason: "validated browser betting rule",
      },
      201,
    );
    const validated = await api<{
      version: number;
      validation?: { passed: boolean };
    }>(
      request,
      `${adminBase}/rule-versions/${version.id}/validate`,
      "POST",
      admin,
      {
        version: version.version,
        cases: [playFixture.validationCase],
        reason: "check known winning and stake totals",
      },
    );
    expect(
      validated.validation?.passed,
      `validation failed for ${playFixture.code}`,
    ).toBe(true);
    version.version = validated.version;
    const submitted = await api<{ version: number }>(
      request,
      `${adminBase}/rule-versions/${version.id}/submit-review`,
      "POST",
      admin,
      { version: version.version, reason: "request independent Harbor review" },
    );
    await api(
      request,
      `${adminBase}/rule-versions/${version.id}/approve`,
      "POST",
      reviewer,
      {
        version: submitted.version,
        reason: "independently reviewed browser rule",
        warnings_acknowledged: true,
      },
    );
    published.push({ id: play.id, gameId: game.id, mode: playFixture.mode });
  }
  return { ...game, published };
}

async function scheduleOpenWindow(
  request: APIRequestContext,
  admin: string,
  game: { id: string; version: number },
) {
  const now = Date.now();
  // One explicit daily draw gives this long financial scenario a real open
  // window without omitting an earlier interval boundary. The worker may fill
  // the calendar concurrently; a late earlier reservation must never bypass
  // the prior-period settlement/refund gate merely to make this fixture pass.
  const draw = new Date(now + 600_000);
  draw.setUTCMilliseconds(0);
  const schedule = await api<{ id: string }>(
    request,
    `${adminBase}/games/${game.id}/schedule`,
    "PUT",
    admin,
    {
      version: game.version,
      reason: "reserve near-future genuine betting window",
      spec: {
        timezone: "UTC",
        mode: "daily",
        daily_draw_times: [draw.toISOString().slice(11, 19)],
        interval_seconds: 0,
        busy_windows: [],
        bet_open_before_seconds: 900,
        bet_close_before_seconds: 60,
        pause_dates: [],
        weekdays: [0, 1, 2, 3, 4, 5, 6],
        holiday_dates: [],
        holiday_policy: "normal",
      },
    },
  );
  expect(schedule.id).toBeTruthy();
  const generated = await api<{
    created: number;
    periods: Array<{ id: string; status: string }>;
  }>(request, `${adminBase}/games/${game.id}/periods/generate`, "POST", admin, {
    from: new Date(now).toISOString(),
    to: new Date(now + 600_000).toISOString(),
    reason: "generate the explicit real daily draw",
  });
  // FillCalendar may reserve the same immutable period first; created can be
  // zero, but the real generated result must still contain this single draw.
  expect(generated.periods).toHaveLength(1);
  return generated.periods[0];
}

async function chooseSelection(
  panel: ReturnType<Page["locator"]>,
  mode: string,
  modelType: string,
) {
  if (mode === "numbers") {
    if (modelType === "DIGITS_0_9") {
      const positions = panel.locator(".position-group");
      await expect(positions).toHaveCount(3);
      for (let i = 0; i < 3; i++) {
        await positions
          .nth(i)
          .getByLabel(`Choose digit 1 for position ${i + 1}`, { exact: true })
          .check();
        await positions
          .nth(i)
          .getByLabel(`Choose digit 2 for position ${i + 1}`, { exact: true })
          .check();
      }
    } else {
      const groups = panel.locator(".selection-group");
      await groups
        .nth(0)
        .getByLabel("Choose regular number 1", { exact: true })
        .check();
      await groups
        .nth(0)
        .getByLabel("Choose regular number 2", { exact: true })
        .check();
      await groups
        .nth(1)
        .getByLabel(
          `Choose special number ${modelType === "M_SELECT_N" ? "3" : "10"}`,
          { exact: true },
        )
        .check();
      if (modelType === "X_PLUS_Y")
        await groups
          .nth(1)
          .getByLabel("Choose special number 11", { exact: true })
          .check();
    }
  } else if (mode === "exclude") {
    await panel
      .locator(".selection-group")
      .getByLabel("Exclude number 6", { exact: true })
      .check();
  } else if (mode === "attributes") {
    await panel
      .locator(".selection-group")
      .getByLabel("Choose attribute color: red", { exact: true })
      .check();
  } else {
    await panel
      .locator(".selection-group")
      .getByLabel("Choose feature odd_count: 2", { exact: true })
      .check();
    await panel
      .locator(".selection-group")
      .getByLabel("Choose feature odd_count: 1", { exact: true })
      .check();
  }
}

async function previewPlay(
  page: Page,
  gameId: string,
  playId: string,
  mode: string,
  modelType: string,
  combinations: number,
) {
  await page.goto(`${harborSite}/games/${gameId}/bet`);
  const panel = page.getByTestId("bet-selection-page");
  await expect(panel).toBeVisible();
  await panel.getByTestId("play-select").selectOption(playId);
  // Randomization only edits the ticket; the real server still validates it.
  await panel.getByRole('button', { name: 'Random selection', exact: true }).click();
  await expect(panel.locator('.selection-editor input:checked').first()).toBeChecked();
  await panel.getByTestId('preview-button').click();
  await expect(panel.getByTestId('bet-quote')).toBeVisible();
  await panel.getByRole('button', { name: 'Clear selection', exact: true }).click();
  await expect(panel.locator('.selection-editor input:checked')).toHaveCount(0);
  await expect(panel.getByTestId('bet-quote')).toHaveCount(0);
  await chooseSelection(panel, mode, modelType);
  await panel.getByTestId("multiplier-input").fill("1");
  await panel.getByTestId("preview-button").click();
  const quote = panel.getByTestId("bet-quote");
  await expect(quote).toContainText(`Combinations ${combinations}`);
  await expect(quote).toContainText("Total points");
}

test("real Harbor catalog quotes, places and cancels an audited bet", async ({
  page,
  context,
}, info) => {
  test.setTimeout(150_000);
  page.setDefaultTimeout(10_000);
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD ||
      !process.env.TEST_RULE_REVIEWER_USERNAME ||
      !process.env.TEST_RULE_REVIEWER_PASSWORD,
    "Provide the isolated Harbor creator and distinct reviewer credentials",
  );

  // Exactly one authenticated API session per role is reused for all setup operations.
  const admin = await adminToken(
    page.request,
    process.env.TEST_HARBOR_ADMIN_USERNAME!,
    process.env.TEST_HARBOR_ADMIN_PASSWORD!,
  );
  // Preserve the genuine server-issued creator session before the separate
  // reviewer login replaces the context's admin cookie. Reuse it for the UI,
  // rather than performing another password login under real rate limits.
  const adminCookies = (await context.cookies(`${adminBase}/me`)).filter(
    (cookie) => cookie.name === "lottery_admin",
  );
  expect(adminCookies).toHaveLength(1);
  const reviewer = await adminToken(
    page.request,
    process.env.TEST_RULE_REVIEWER_USERNAME!,
    process.env.TEST_RULE_REVIEWER_PASSWORD!,
  );
  const reviewerCookies = (await context.cookies(`${adminBase}/me`)).filter(cookie => cookie.name === "lottery_admin");

  const digitsCode = `e2e_digits_${unique()}`;
  const xyCode = `e2e_xy_${unique()}`;
  const mnCode = `e2e_mn_${unique()}`;
  const digitsRule = fixture("digits", "DIGITS_0_9", "numbers");
  const featureRule = fixture("features", "DIGITS_0_9", "features");
  const xyRule = fixture("xy", "X_PLUS_Y", "numbers");
  const excludeRule = fixture("excluded", "X_PLUS_Y", "exclude");
  const attributeRule = fixture("attributes", "X_PLUS_Y", "attributes");
  const mnRule = fixture("mn", "M_SELECT_N", "numbers");
  const digits = await provisionGame(
    page.request,
    admin,
    reviewer,
    digitsCode,
    "E2E positional digits",
    modelFor("DIGITS_0_9"),
    [digitsRule, featureRule],
  );
  const xy = await provisionGame(
    page.request,
    admin,
    reviewer,
    xyCode,
    "E2E X plus Y",
    modelFor("X_PLUS_Y"),
    [xyRule, excludeRule, attributeRule],
  );
  const mn = await provisionGame(
    page.request,
    admin,
    reviewer,
    mnCode,
    "E2E M select N",
    modelFor("M_SELECT_N"),
    [mnRule],
  );

  const gamePolicy = await api<{ version: number }>(
    page.request,
    `${adminBase}/games/${digits.id}/bet-policy`,
    "GET",
    admin,
  );
  await api(
    page.request,
    `${adminBase}/games/${digits.id}/bet-policy`,
    "PUT",
    admin,
    {
      version: gamePolicy.version,
      reason: "allow this test game to cancel its own placed order",
      config: {
        min_bet_points: { mode: "inherit", points: null },
        max_bet_points: { mode: "inherit", points: null },
        max_period_points: { mode: "inherit", points: null },
        max_user_period_points: { mode: "inherit", points: null },
        user_cancel_allowed: true,
      },
    },
  );

  const periods = [
    {
      game: digits,
      period: await scheduleOpenWindow(page.request, admin, digits),
    },
    { game: xy, period: await scheduleOpenWindow(page.request, admin, xy) },
    { game: mn, period: await scheduleOpenWindow(page.request, admin, mn) },
  ];

  const username = `bet_${unique()}`;
  const password = `real-bet-${unique()}-Pass9`;
  const authContextResponse = await page.request.get(`${publicBase}/context`);
  const authContextText = await authContextResponse.text();
  expect(authContextResponse.status(), authContextText).toBe(200);
  const authContext = (
    JSON.parse(authContextText) as Envelope<{
      auth?: { captcha_enabled?: boolean };
    }>
  ).data;
  let registrationCaptcha:
    | { captcha_id: string; captcha_answer: string }
    | Record<string, never> = {};
  if (authContext.auth?.captcha_enabled) {
    const challengeResponse = await page.request.get(
      `${publicBase}/auth/challenge`,
    );
    const challengeText = await challengeResponse.text();
    expect(challengeResponse.status(), challengeText).toBe(200);
    const challenge = (
      JSON.parse(challengeText) as Envelope<{ id: string; svg: string }>
    ).data;
    const answer = answerSvgChallenge(challenge.svg);
    expect(answer).toMatch(/^[A-Za-z0-9]+$/);
    registrationCaptcha = { captcha_id: challenge.id, captcha_answer: answer };
  }
  const registered = await page.request.post(`${publicBase}/auth/register`, {
    headers: { Origin: apiOrigin, "Idempotency-Key": crypto.randomUUID() },
    data: {
      username,
      password,
      privacy_policy_version: "dev-1",
      service_terms_version: "dev-1",
      ...registrationCaptcha,
    },
  });
  const registrationText = await registered.text();
  expect(
    registered.status(),
    `Harbor member registration: ${registrationText}`,
  ).toBe(201);
  const registration = JSON.parse(registrationText) as Envelope<{
    access_token: string;
    member: { id: string };
  }>;
  expect(registration.success).toBe(true);
  const memberId = registration.data.member.id;

  const recharge = await api<{ id: string; version: number }>(
    page.request,
    `${adminBase}/recharges`,
    "POST",
    admin,
    {
      member_id: memberId,
      points: "100",
      proof_reference: `e2e-${unique()}`,
      remark: "real browser bet funds",
      reason: "fund isolated browser betting user",
    },
    201,
  );
  await api(
    page.request,
    `${adminBase}/recharges/${recharge.id}/confirm`,
    "POST",
    admin,
    { version: recharge.version, reason: "confirm genuine E2E funding" },
  );

  await page.goto(`${harborSite}/login`);
  await page.getByLabel("Username or phone", { exact: true }).fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  const captchaImage = page.getByRole("img", {
    name: "Verification code image",
    exact: true,
  });
  if (authContext.auth?.captcha_enabled) {
    await expect(captchaImage).toBeVisible();
    const answer = await captchaImage.evaluate((image) => {
      const source = image.getAttribute("src") ?? "";
      const svg = atob(source.slice(source.indexOf(",") + 1));
      return answerSvgChallenge(svg);
    });
    expect(answer).toMatch(/^[A-Za-z0-9]+$/);
    await page
      .getByLabel("Enter the characters shown", { exact: true })
      .fill(answer);
  }
  await page.getByRole("button", { name: /Continue/ }).click();
  await expect(page).toHaveURL(/\/account$/);

  // The actual public hostname resolves through Chromium's *.localhost loopback mapping.
  const periodStatus = async (gameId: string) => {
    const response = await page.request.get(`${publicBase}/games/${gameId}`);
    const text = await response.text();
    expect(response.status(), text).toBe(200);
    const data = (
      JSON.parse(text) as Envelope<{
        period: { id: string; status: string } | null;
      }>
    ).data;
    return data.period?.status;
  };
  for (const item of periods) {
    await expect
      .poll(() => periodStatus(item.game.id), {
        timeout: 20_000,
        intervals: [250, 500, 1000],
      })
      .toBe("betting");
    expect(item.period.id).toBeTruthy();
  }

  const userToken = registration.data.access_token;
  const walletBefore = await api<{
    available_points: string;
    display_points: string;
  }>(page.request, `${publicBase}/wallet`, "GET", userToken);
  expect(walletBefore.available_points).toBe("100");
  expect(walletBefore.display_points).toBe("100");
  const ledgerBefore = await api<{ items: Array<Record<string, any>> }>(
    page.request,
    `${adminBase}/wallets/${memberId}/ledger?limit=100`,
    "GET",
    admin,
  );
  expect(ledgerBefore.items).toHaveLength(1);
  const fundingEntry = ledgerBefore.items[0];
  expect(fundingEntry.entry_type).toBe("recharge");
  expect(fundingEntry.before_snapshot.recharge.available).toBe("0");
  expect(fundingEntry.delta_snapshot.recharge.available).toBe("100");
  expect(fundingEntry.after_snapshot.recharge.available).toBe("100");

  // Exercise each published model and each supported selection editor through server quotes.
  const catalog = page.getByTestId("games-catalog");
  await page.goto(`${harborSite}/games`, { waitUntil: "domcontentloaded" });
  await expect(
    catalog
      .getByTestId("catalog-game")
      .filter({ hasText: digitsCode }),
  ).toBeVisible();
  await expect(
    catalog.getByTestId("catalog-game").filter({ hasText: xyCode }),
  ).toBeVisible();
  await expect(
    catalog.getByTestId("catalog-game").filter({ hasText: mnCode }),
  ).toBeVisible();
  // The server computes this quote normally; delay only delivery while the user changes intent.
  await page.goto(`${harborSite}/games/${digits.id}/bet`);
  const stalePanel = page.getByTestId("bet-selection-page");
  await expect(stalePanel).toBeVisible();
  await stalePanel
    .getByTestId("play-select")
    .selectOption(featurePlayId(digits.published, "numbers"));
  await chooseSelection(stalePanel, "numbers", "DIGITS_0_9");
  let releaseStaleQuote!: () => void;
  let resolveStaleFetched!: (status: number) => void;
  const staleRelease = new Promise<void>((resolve) => {
    releaseStaleQuote = resolve;
  });
  const staleFetched = new Promise<number>((resolve) => {
    resolveStaleFetched = resolve;
  });
  const previewRoute = "**/api/v1/bet-previews";
  await page.route(previewRoute, async (route) => {
    const serverResponse = await fetchHarbor(route);
    resolveStaleFetched(serverResponse.status());
    await staleRelease;
    await route.fulfill({ response: serverResponse });
  });
  await stalePanel.getByTestId("preview-button").click();
  expect(await staleFetched).toBe(200);
  await stalePanel
    .locator(".position-group")
    .first()
    .getByLabel("Choose digit 2 for position 1", { exact: true })
    .uncheck();
  releaseStaleQuote();
  await expect(stalePanel.getByTestId("bet-quote")).toHaveCount(0);
  await expect(stalePanel.getByTestId("preview-button")).toBeEnabled();
  await page.unroute(previewRoute);
  await stalePanel.getByTestId("preview-button").click();
  await expect(stalePanel.getByTestId("bet-quote")).toContainText(
    "Combinations 4",
  );

  await previewPlay(
    page,
    digits.id,
    featurePlayId(digits.published, "features"),
    "features",
    "DIGITS_0_9",
    2,
  );
  await previewPlay(
    page,
    xy.id,
    featurePlayId(xy.published, "numbers"),
    "numbers",
    "X_PLUS_Y",
    2,
  );
  await previewPlay(
    page,
    xy.id,
    featurePlayId(xy.published, "exclude"),
    "exclude",
    "X_PLUS_Y",
    1,
  );
  await previewPlay(
    page,
    xy.id,
    featurePlayId(xy.published, "attributes"),
    "attributes",
    "X_PLUS_Y",
    1,
  );
  await previewPlay(
    page,
    mn.id,
    featurePlayId(mn.published, "numbers"),
    "numbers",
    "M_SELECT_N",
    1,
  );

  // Start the submitted flow from the public catalog and use its actual accessible controls.
  await page.goto(`${harborSite}/games`);
  const digitsCard = page
    .getByTestId("catalog-game")
    .filter({ hasText: digitsCode });
  await digitsCard.getByRole("link", { name: /View game/ }).click();
  const detail = page.getByTestId("game-detail");
  await expect(detail).toContainText("betting");
  await detail
    .locator(".play-card")
    .filter({
      has: page.getByRole("heading", { name: digitsRule.name, exact: true }),
    })
    .getByRole("button", { name: /Choose numbers/ })
    .click();
  const selectionPage = page.getByTestId("bet-selection-page");
  await expect(selectionPage).toBeVisible();
  await selectionPage
    .getByTestId("play-select")
    .selectOption(featurePlayId(digits.published, "numbers"));
  await chooseSelection(selectionPage, "numbers", "DIGITS_0_9");
  await selectionPage.getByTestId("multiplier-input").fill("2");
  await selectionPage.getByTestId("preview-button").click();
  const quote = selectionPage.getByTestId("bet-quote");
  await expect(quote).toContainText("Combinations 8");
  await expect(quote).toContainText("Total points 16");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("betting-selection.png"),
    fullPage: true,
  });
  await quote.getByTestId("review-bet").click();
  await expect(page.getByTestId("bet-confirmation")).toBeVisible();
  await expect(page.getByTestId("selection-summary")).toContainText(
    "Position 3",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("betting-confirmation.png"),
    fullPage: true,
  });
  const confirm = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/bet-orders") &&
      response.request().method() === "POST",
  );
  const placeRequests: Array<{ key: string; body: unknown }> = [];
  let resolveFirstCommit!: (value: { status: number; id: string }) => void;
  const firstCommit = new Promise<{ status: number; id: string }>((resolve) => {
    resolveFirstCommit = resolve;
  });
  const placeRoute = "**/api/v1/bet-orders";
  await page.route(placeRoute, async (route) => {
    const request = route.request();
    if (request.method() !== "POST") return route.continue();
    placeRequests.push({
      key: (await request.allHeaders())["idempotency-key"],
      body: request.postDataJSON(),
    });
    if (placeRequests.length === 1) {
      const committedResponse = await fetchHarbor(route);
      const committedText = await committedResponse.text();
      const committed = JSON.parse(committedText) as Envelope<{ id: string }>;
      resolveFirstCommit({
        status: committedResponse.status(),
        id: committed.data.id,
      });
      await route.abort("failed");
    } else {
      await route.continue();
    }
  });
  await page.getByTestId("confirm-bet").click();
  const committedOnce = await firstCommit;
  expect(committedOnce.status).toBe(201);
  await expect(page.getByRole("alert")).toContainText(
    "network result is uncertain",
  );
  await expect(page.getByTestId("confirm-bet")).toHaveText(/Retry confirm/);
  const retryConfirm = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/bet-orders") &&
      response.request().method() === "POST",
  );
  await page.getByTestId("confirm-bet").click();
  const placedResponse = await confirm;
  const retryResponse = await retryConfirm;
  expect(retryResponse.status()).toBe(201);
  expect(placeRequests).toHaveLength(2);
  expect(placeRequests[1]).toEqual(placeRequests[0]);
  // Keep this narrowly scoped route installed until page teardown. Removing
  // interception immediately after the receipt can strand simultaneous
  // background order/wallet reads before their route callbacks run.
  const placedText = await placedResponse.text();
  expect(placedResponse.status(), placedText).toBe(201);
  const placedEnvelope = JSON.parse(placedText) as Envelope<
    Record<string, any>
  >;
  const placed = placedEnvelope.data;
  expect(placed.id).toBe(committedOnce.id);
  expect(placed.status).toBe("placed");
  expect(placed.combination_count).toBe(8);
  expect(placed.total_points).toBe("16");
  expect(placed.policy_snapshot.user_cancel_allowed).toBe(true);
  expect(placed.selection_raw.digits).toEqual([
    [1, 2],
    [1, 2],
    [1, 2],
  ]);
  expect(
    placed.expanded_bets.map(
      (line: { digits: number[][] | null }) => line.digits,
    ),
  ).toContainEqual([[1], [2], [1]]);
  const immutableFields = {
    game_id: placed.game_id,
    period_id: placed.period_id,
    play_id: placed.play_id,
    rule_version_id: placed.rule_version_id,
    definition_hash: placed.definition_hash,
    definition_snapshot: placed.definition_snapshot,
    selection_raw: placed.selection_raw,
    selection_normalized: placed.selection_normalized,
    expanded_bets: placed.expanded_bets,
    unit_points: placed.unit_points,
    combination_count: placed.combination_count,
    multiplier: placed.multiplier,
    total_points: placed.total_points,
    policy_snapshot: placed.policy_snapshot,
    debit_entry_id: placed.debit_entry_id,
  };
  const debitedWallet = await api<{
    available_points: string;
    display_points: string;
  }>(page.request, `${publicBase}/wallet`, "GET", userToken);
  expect(debitedWallet.available_points).toBe("84");

  const orderDetail = page.getByTestId("order-detail");
  await expect(orderDetail).toContainText(placed.id);
  await expect(orderDetail.locator(".cancel-form")).toBeVisible();
  await orderDetail
    .getByLabel("Cancellation reason (required)", { exact: true })
    .fill("E2E verifies full ledger reversal");
  const cancelRoute = `**/api/v1/bet-orders/${placed.id}/cancel`;
  const cancelRequests: Array<{ key: string; body: unknown }> = [];
  let resolveCancelCommit!: (value: { status: number; id: string }) => void;
  const cancelCommit = new Promise<{ status: number; id: string }>(
    (resolve) => {
      resolveCancelCommit = resolve;
    },
  );
  await page.route(cancelRoute, async (route) => {
    const request = route.request();
    if (request.method() !== "POST") return route.continue();
    cancelRequests.push({
      key: (await request.allHeaders())["idempotency-key"],
      body: request.postDataJSON(),
    });
    if (cancelRequests.length === 1) {
      const committedResponse = await fetchHarbor(route);
      const committed = (await committedResponse.json()) as Envelope<{
        id: string;
      }>;
      resolveCancelCommit({
        status: committedResponse.status(),
        id: committed.data.id,
      });
      await route.abort("failed");
    } else {
      await route.continue();
    }
  });
  await orderDetail
    .getByRole("button", { name: "Cancel this order", exact: true })
    .click();
  const cancelCommittedOnce = await cancelCommit;
  expect(cancelCommittedOnce.status).toBe(200);
  await expect(orderDetail.getByRole("alert")).toContainText(
    "network result is uncertain",
  );
  const retryCancel = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/api/v1/bet-orders/${placed.id}/cancel`) &&
      response.request().method() === "POST",
  );
  await orderDetail
    .getByRole("button", { name: "Retry cancellation", exact: true })
    .click();
  const cancelledResponse = await retryCancel;
  expect(cancelRequests).toHaveLength(2);
  expect(cancelRequests[1]).toEqual(cancelRequests[0]);
  // The cancellation receipt also triggers follow-up reads. Page teardown
  // removes the route after the full audited cancellation flow has finished.
  const cancelledText = await cancelledResponse.text();
  expect(cancelledResponse.status(), cancelledText).toBe(200);
  const cancelled = (JSON.parse(cancelledText) as Envelope<Record<string, any>>)
    .data;
  expect(cancelled.status).toBe("bet_cancelled");
  expect(cancelled.version).toBe(placed.version + 1);
  expect(cancelled.refund_entry_id).toBeTruthy();
  expect({
    game_id: cancelled.game_id,
    period_id: cancelled.period_id,
    play_id: cancelled.play_id,
    rule_version_id: cancelled.rule_version_id,
    definition_hash: cancelled.definition_hash,
    definition_snapshot: cancelled.definition_snapshot,
    selection_raw: cancelled.selection_raw,
    selection_normalized: cancelled.selection_normalized,
    expanded_bets: cancelled.expanded_bets,
    unit_points: cancelled.unit_points,
    combination_count: cancelled.combination_count,
    multiplier: cancelled.multiplier,
    total_points: cancelled.total_points,
    policy_snapshot: cancelled.policy_snapshot,
    debit_entry_id: cancelled.debit_entry_id,
  }).toEqual(immutableFields);
  await expect(orderDetail).toContainText("bet cancelled");
  await expect(orderDetail.locator(".cancel-form")).toHaveCount(0);

  const walletAfter = await api<{
    available_points: string;
    display_points: string;
  }>(page.request, `${publicBase}/wallet`, "GET", userToken);
  expect(walletAfter.available_points).toBe("100");
  expect(walletAfter.display_points).toBe(walletBefore.display_points);
  const ledgerAfter = await api<{ items: Array<Record<string, any>> }>(
    page.request,
    `${adminBase}/wallets/${memberId}/ledger?limit=100`,
    "GET",
    admin,
  );
  expect(ledgerAfter.items).toHaveLength(3);
  const debit = ledgerAfter.items.find((entry) => entry.entry_type === "bet");
  const refund = ledgerAfter.items.find(
    (entry) => entry.entry_type === "refund",
  );
  expect(debit).toBeTruthy();
  expect(refund).toBeTruthy();
  if (!debit || !refund)
    throw new Error("Missing original user debit or refund ledger evidence");
  expect(debit.reference_id).toBe(placed.id);
  expect(refund.reference_id).toBe(placed.id);
  expect(refund.reversal_of).toBe(debit.id);
  expect(debit.before_snapshot).toEqual(fundingEntry.after_snapshot);
  expect(debit.delta_snapshot.recharge.available).toBe("-16");
  expect(debit.after_snapshot.recharge.available).toBe("84");
  expect(refund.before_snapshot).toEqual(debit.after_snapshot);
  expect(refund.delta_snapshot.recharge.available).toBe("16");
  expect(refund.after_snapshot).toEqual(fundingEntry.after_snapshot);
  expect(
    ledgerAfter.items.filter((entry) => entry.entry_type === "bet"),
  ).toHaveLength(1);
  const memberLedger = await api<{ items: Array<Record<string, any>> }>(
    page.request,
    `${publicBase}/wallet/ledger?limit=100`,
    "GET",
    userToken,
  );
  expect(memberLedger.items.map((entry) => entry.id)).toContain(refund.id);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await expect(orderDetail.locator(".wallet-balance")).toContainText("100");
  await page.screenshot({
    path: info.outputPath("betting-refunded.png"),
    fullPage: true,
  });

  // S5-a3 reuses this member, rule, period and exact selection after the user's
  // refund. A 24-point second stake exceeds the 16-point brand default but fits
  // the 32-point game override. Both viewport projects use the same brand cap,
  // which still permits the other project's original 16-point user flow.
  test.setTimeout(240_000);
  // API setup left the reviewer admin cookie in this shared browser context.
  // Clear only that cookie so the new page actually logs in as the Harbor creator;
  // the fixture's bearer tokens and member cookies remain usable.
  await context.clearCookies({ name: "lottery_admin" });
  const adminPage = await context.newPage();
  adminPage.setDefaultTimeout(10_000);
  const showS5AdminPage = async () => {
    if (adminPage.viewportSize()!.width <= 700) {
      await adminPage
        .locator(".mobile-nav")
        .getByRole("button", { name: /更多/ })
        .click();
      await adminPage
        .getByRole("navigation", { name: "全部管理页面", exact: true })
        .getByRole("button", { name: /注单和异常/ })
        .click();
    } else {
      await adminPage
        .locator(".side-nav")
        .getByRole("button", { name: /注单和异常/ })
        .click();
    }
    await expect(adminPage.getByTestId("bet-policy-settings")).toBeVisible();
    await expect(adminPage.getByTestId("bet-order-management")).toBeVisible();
  };
  const captureS5Admin = async (name: string) => {
    expect(
      await adminPage.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    ).toBe(true);
    await adminPage.screenshot({ path: info.outputPath(name), fullPage: true });
  };
  try {
    await context.addCookies(adminCookies);
    await adminPage.goto(adminOrigin);
    await adminPage
      .getByLabel("选择真实后台品牌", { exact: true })
      .selectOption(brandId);
    await showS5AdminPage();

    const policyPanel = adminPage.getByTestId("bet-policy-settings");
    const brandCap = policyPanel.getByLabel("品牌：单注上限积分（留空不限）", {
      exact: true,
    });
    await expect(brandCap).toBeEnabled();
    await policyPanel
      .getByLabel("品牌：最低单注积分", { exact: true })
      .fill("1");
    await brandCap.fill("16");
    await policyPanel
      .getByLabel("品牌：单期上限积分（留空不限）", { exact: true })
      .fill("");
    await policyPanel
      .getByLabel("品牌：单用户单期上限积分（留空不限）", { exact: true })
      .fill("");
    await policyPanel
      .getByLabel("品牌：变更原因（必填，最多 500 UTF-8 字节）", {
        exact: true,
      })
      .fill(`S5-a3 ${info.project.name}: persist brand single-bet default`);
    let savedBrand: BrandBetPolicy | undefined;
    // The two real viewport fixtures may concurrently advance this shared brand
    // version. Only a definitive version conflict permits a new UI confirmation.
    for (let attempt = 0; attempt < 3; attempt++) {
      const save = policyPanel.getByRole("button", {
        name: "保存品牌策略",
        exact: true,
      });
      await expect(save).toBeEnabled();
      const pendingSave = adminPage.waitForResponse(
        (response) =>
          response.url().endsWith("/api/v1/admin/bet-policy") &&
          response.request().method() === "PUT",
      );
      await save.click();
      const response = await pendingSave;
      const result = await response.json();
      if (response.status() === 409) {
        expect(result.error?.code).toBe("BET_VERSION_CONFLICT");
        continue;
      }
      expect(response.status(), JSON.stringify(result)).toBe(200);
      expect(result.success).toBe(true);
      expect(response.request().postDataJSON().config.max_bet_points).toBe(
        "16",
      );
      savedBrand = (result as Envelope<BrandBetPolicy>).data;
      break;
    }
    if (!savedBrand)
      throw new Error(
        "S5-a3 brand policy still conflicted after three confirmations",
      );
    expect(savedBrand.config.max_bet_points).toBe("16");
    const persistedBrand = await api<BrandBetPolicy>(
      page.request,
      `${adminBase}/bet-policy`,
      "GET",
      admin,
    );
    expect(persistedBrand.config.max_bet_points).toBe("16");
    expect(persistedBrand.version).toBeGreaterThanOrEqual(savedBrand.version);

    await adminPage.reload();
    await adminPage
      .getByLabel("选择真实后台品牌", { exact: true })
      .selectOption(brandId);
    await showS5AdminPage();
    await expect(brandCap).toHaveValue("16");
    await policyPanel
      .getByLabel("游戏：彩种 ID", { exact: true })
      .fill(digits.id);
    await policyPanel
      .getByRole("button", { name: "读取彩种策略", exact: true })
      .click();
    await expect(policyPanel.locator(".policy-version")).toContainText(
      digits.id,
    );
    await policyPanel
      .getByLabel("游戏：单注上限积分", { exact: true })
      .selectOption("value");
    await policyPanel
      .getByLabel("游戏：单注上限积分数值", { exact: true })
      .fill("32");
    await policyPanel
      .getByLabel("游戏：变更原因（必填，最多 500 UTF-8 字节）", {
        exact: true,
      })
      .fill(
        `S5-a3 ${info.project.name}: game override takes priority over brand default`,
      );
    const gameSave = adminPage.waitForResponse(
      (response) =>
        response
          .url()
          .endsWith(`/api/v1/admin/games/${digits.id}/bet-policy`) &&
        response.request().method() === "PUT",
    );
    await policyPanel
      .getByRole("button", { name: "保存彩种策略", exact: true })
      .click();
    const gameSavedResponse = await gameSave;
    const gameSavedEnvelope =
      (await gameSavedResponse.json()) as Envelope<GameBetPolicy>;
    expect(gameSavedResponse.status(), JSON.stringify(gameSavedEnvelope)).toBe(
      200,
    );
    expect(gameSavedEnvelope.success).toBe(true);
    const savedGame = gameSavedEnvelope.data;
    expect(savedGame.config.max_bet_points).toEqual({
      mode: "value",
      points: "32",
    });
    expect(BigInt(savedGame.config.max_bet_points.points!)).toBeGreaterThan(
      BigInt(persistedBrand.config.max_bet_points!),
    );
    const persistedGame = await api<GameBetPolicy>(
      page.request,
      `${adminBase}/games/${digits.id}/bet-policy`,
      "GET",
      admin,
    );
    expect(persistedGame.version).toBe(savedGame.version);
    expect(persistedGame.config).toEqual(savedGame.config);
    await captureS5Admin("s5-a3-admin-policy.png");

    const secondInput = {
      period_id: placed.period_id,
      play_id: placed.play_id,
      rule_version_id: placed.rule_version_id,
      selection: placed.selection_raw,
      multiplier: "3",
    };
    // Catalog selection can advance between overlapping generated windows. Use
    // the period captured by the real placed order, not the fixture's first row.
    let secondOrder: AdminBetOrder | undefined;
    for (let attempt = 0; attempt < 3; attempt++) {
      const secondPreview = await api<{
        actor_context: string;
        bet_points: string;
        combination_count: number;
        policy: { max_bet_points: string | null };
        policy_versions: { brand: number; game: number };
      }>(
        page.request,
        `${publicBase}/bet-previews`,
        "POST",
        userToken,
        secondInput,
      );
      expect(secondPreview.bet_points).toBe("24");
      expect(secondPreview.combination_count).toBe(placed.combination_count);
      expect(secondPreview.policy.max_bet_points).toBe("32");
      expect(secondPreview.policy_versions.brand).toBeGreaterThanOrEqual(
        savedBrand.version,
      );
      expect(secondPreview.policy_versions.game).toBe(savedGame.version);
      // Use the latest real quote's actor context and both policy versions.
      const response = await page.request.post(`${publicBase}/bet-orders`, {
        headers: {
          Authorization: `Bearer ${userToken}`,
          "X-Brand-ID": brandId,
          Origin: apiOrigin,
          "Idempotency-Key": crypto.randomUUID(),
        },
        data: {
          ...secondInput,
          actor_context: secondPreview.actor_context,
          policy_versions: secondPreview.policy_versions,
        },
      });
      const result = await response.json();
      if (response.status() === 409) {
        expect(result.error?.code).toBe("BET_VERSION_CONFLICT");
        continue;
      }
      expect(response.status(), JSON.stringify(result)).toBe(201);
      expect(result.success).toBe(true);
      secondOrder = (result as Envelope<AdminBetOrder>).data;
      expect(secondOrder.policy_versions).toEqual(
        secondPreview.policy_versions,
      );
      break;
    }
    if (!secondOrder)
      throw new Error(
        "S5-a3 second placement still conflicted after three fresh quotes",
      );
    expect(secondOrder.id).not.toBe(placed.id);
    expect(secondOrder.status).toBe("placed");
    expect(secondOrder.game_id).toBe(digits.id);
    expect(secondOrder.period_id).toBe(placed.period_id);
    expect(secondOrder.total_points).toBe("24");
    expect(BigInt(secondOrder.total_points)).toBeGreaterThan(
      BigInt(persistedBrand.config.max_bet_points!),
    );
    expect(secondOrder.policy_snapshot.max_bet_points).toBe("32");
    expect(secondOrder.selection_raw).toEqual(placed.selection_raw);
    const secondWallet = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(secondWallet.available_points).toBe("76");
    expect(secondWallet.display_points).toBe("76");
    const secondLedger = await api<{ items: LedgerEntry[] }>(
      page.request,
      `${adminBase}/wallets/${memberId}/ledger?limit=100`,
      "GET",
      admin,
    );
    expect(secondLedger.items).toHaveLength(ledgerAfter.items.length + 1);
    const secondDebit = secondLedger.items.find(
      (entry) => entry.id === secondOrder!.debit_entry_id,
    );
    if (!secondDebit)
      throw new Error("Missing S5-a3 original debit ledger entry");
    expect(secondDebit.reference_id).toBe(secondOrder.id);
    expect(secondDebit.before_snapshot).toEqual(refund.after_snapshot);
    expect(secondDebit.delta_snapshot.recharge.available).toBe("-24");
    expect(secondDebit.after_snapshot.recharge.available).toBe("76");
    expect(secondDebit.source_allocation).toEqual(
      secondOrder.deduction_allocation,
    );

    const ordersPanel = adminPage.getByTestId("bet-order-management");
    await ordersPanel
      .getByLabel("直接查询注单 UUID", { exact: true })
      .fill(secondOrder.id);
    await ordersPanel
      .getByRole("button", { name: "查询", exact: true })
      .click();
    await expect(ordersPanel.locator(".order-row")).toHaveCount(1);
    const adminDetail = ordersPanel.getByRole("region", {
      name: "注单详情",
      exact: true,
    });
    await expect(adminDetail).toContainText(secondOrder.id);
    await expect(adminDetail.locator(".amounts")).toContainText("3 倍");
    await expect(
      adminDetail.getByRole("button", { name: "标记为异常", exact: true }),
    ).toBeVisible();
    await expect(adminDetail.locator(".evidence")).toContainText(
      "当前注单没有异常标记记录",
    );

    const abnormalReason = `S5-a3 ${info.project.name}: real uncertain abnormal write`;
    const abnormalRoute = `**/api/v1/admin/bet-orders/${secondOrder.id}/abnormal`;
    const abnormalRequests: Array<{ key: string; body: string | null }> = [];
    let resolveAbnormalCommit!: (value: {
      status: number;
      order: AdminBetOrder;
    }) => void;
    const abnormalCommit = new Promise<{
      status: number;
      order: AdminBetOrder;
    }>((resolve) => {
      resolveAbnormalCommit = resolve;
    });
    await adminPage.route(abnormalRoute, async (route) => {
      const request = route.request();
      const headers = await request.allHeaders();
      abnormalRequests.push({
        key: headers["idempotency-key"],
        body: request.postData(),
      });
      if (abnormalRequests.length === 1) {
        // Commit against the real API before dropping only the browser response.
        const committedResponse = await route.fetch();
        const result =
          (await committedResponse.json()) as Envelope<AdminBetOrder>;
        resolveAbnormalCommit({
          status: committedResponse.status(),
          order: result.data,
        });
        await route.abort("failed");
      } else {
        await route.continue();
      }
    });
    await adminDetail
      .getByRole("button", { name: "标记为异常", exact: true })
      .click();
    const abnormalDialog = adminPage.getByRole("dialog", {
      name: "确认标记异常",
      exact: true,
    });
    await abnormalDialog
      .getByLabel("操作原因（必填，UTF-8 最多 500 字节）", { exact: true })
      .fill(abnormalReason);
    await abnormalDialog
      .getByRole("button", { name: "检查并确认", exact: true })
      .click();
    await expect(abnormalDialog.getByRole("textbox")).toBeDisabled();
    await captureS5Admin("s5-a3-admin-abnormal-confirmation.png");
    await abnormalDialog
      .getByRole("button", { name: "确认并提交", exact: true })
      .click();
    const abnormalOnce = await abnormalCommit;
    expect(abnormalOnce.status).toBe(200);
    expect(abnormalOnce.order.id).toBe(secondOrder.id);
    expect(abnormalOnce.order.status).toBe("abnormal");
    expect(abnormalOnce.order.version).toBe(secondOrder.version + 1);
    expect(abnormalRequests).toHaveLength(1);
    expect(abnormalRequests[0].key).toBeTruthy();
    expect(JSON.parse(abnormalRequests[0].body!)).toEqual({
      version: secondOrder.version,
      reason: abnormalReason,
    });
    await expect(adminDetail.getByRole("alert")).toContainText(
      "请求结果尚未确定",
    );
    await expect(
      adminDetail.getByRole("button", { name: "标记为异常", exact: true }),
    ).toHaveCount(0);
    const committedEvidence = await api<{ exception: BetException | null }>(
      page.request,
      `${adminBase}/bet-orders/${secondOrder.id}/exception`,
      "GET",
      admin,
    );
    expect(committedEvidence.exception).toMatchObject({
      brand_id: brandId,
      order_id: secondOrder.id,
      order_version: secondOrder.version + 1,
      reason: abnormalReason,
    });
    expect(committedEvidence.exception?.id).toBeTruthy();
    expect(
      await api<Wallet>(page.request, `${publicBase}/wallet`, "GET", userToken),
    ).toEqual(secondWallet);
    expect(
      await api<{ items: LedgerEntry[] }>(
        page.request,
        `${adminBase}/wallets/${memberId}/ledger?limit=100`,
        "GET",
        admin,
      ),
    ).toEqual(secondLedger);
    await captureS5Admin("s5-a3-admin-abnormal-uncertain.png");

    // Acknowledging the replay does not complete the subsequent order,
    // exception and judgment reads. Hold real exception data to verify that
    // the panel reports loading rather than inventing "no evidence".
    let releaseException!: () => void;
    let exceptionReached!: (status: number) => void;
    const heldException = new Promise<void>(resolve => { releaseException = resolve; });
    const exceptionResponseReached = new Promise<number>(resolve => { exceptionReached = resolve; });
    const exceptionRoute = `**/api/v1/admin/bet-orders/${secondOrder.id}/exception`;
    await adminPage.route(exceptionRoute, async route => {
      const response = await route.fetch();
      exceptionReached(response.status());
      await heldException;
      await route.fulfill({ response });
    }, { times: 1 });
    // The unchanged 150-second scenario deadline bounds these observations;
    // a read need not finish within the shorter locator assertion timeout.
    const exceptionAfterRetry = adminPage.waitForResponse(response =>
      response.request().method() === "GET" &&
      new URL(response.url()).pathname === `/api/v1/admin/bet-orders/${secondOrder.id}/exception`,
      { timeout: 0 },
    );
    const judgmentAfterRetry = adminPage.waitForResponse(response =>
      response.request().method() === "GET" &&
      new URL(response.url()).pathname === `/api/v1/admin/bet-orders/${secondOrder.id}/judgment`,
      { timeout: 0 },
    );
    const abnormalRetry = adminPage.waitForResponse(
      (response) =>
        response
          .url()
          .endsWith(`/api/v1/admin/bet-orders/${secondOrder!.id}/abnormal`) &&
        response.request().method() === "POST",
    );
    try {
      await adminDetail
        .getByRole("button", { name: "使用相同请求编号重试", exact: true })
        .click();
      expect(await exceptionResponseReached).toBe(200);
      await expect(adminDetail.locator(".evidence")).toContainText("正在读取异常标记记录");
      await expect(adminDetail.locator(".evidence")).not.toContainText("当前注单没有异常标记记录");
      await expect(adminDetail.locator(".judgment-evidence")).toContainText("正在读取判定取消记录");
    } finally {
      releaseException();
    }
    const abnormalRetryResponse = await abnormalRetry;
    expect(abnormalRetryResponse.status()).toBe(200);
    expect(abnormalRequests).toHaveLength(2);
    expect(abnormalRequests[1]).toEqual(abnormalRequests[0]);
    expect(
      ((await abnormalRetryResponse.json()) as Envelope<AdminBetOrder>).data,
    ).toEqual(abnormalOnce.order);
    await adminPage.unroute(abnormalRoute);
    const [exceptionRead, judgmentRead] = await Promise.all([exceptionAfterRetry, judgmentAfterRetry]);
    for (const response of [exceptionRead, judgmentRead]) {
      expect(response.status()).toBe(200);
      expect(await response.finished()).toBeNull();
    }
    await adminPage.unroute(exceptionRoute);
    await expect(adminDetail.locator(".evidence")).toContainText(
      committedEvidence.exception!.id,
    );
    await expect(adminDetail.locator(".evidence")).toContainText(
      abnormalReason,
    );
    await expect(adminDetail.locator(".detail-top")).toContainText("异常注单");
    expect(
      await api<{ exception: BetException | null }>(
        page.request,
        `${adminBase}/bet-orders/${secondOrder.id}/exception`,
        "GET",
        admin,
      ),
    ).toEqual(committedEvidence);
    expect(
      await api<Wallet>(page.request, `${publicBase}/wallet`, "GET", userToken),
    ).toEqual(secondWallet);
    expect(
      await api<{ items: LedgerEntry[] }>(
        page.request,
        `${adminBase}/wallets/${memberId}/ledger?limit=100`,
        "GET",
        admin,
      ),
    ).toEqual(secondLedger);

    await adminDetail
      .getByRole("button", { name: "管理员取消并退款", exact: true })
      .click();
    const cancelDialog = adminPage.getByRole("dialog", {
      name: "确认管理员取消",
      exact: true,
    });
    const adminCancelReason = `S5-a3 ${info.project.name}: refund abnormal order to original source`;
    await cancelDialog
      .getByLabel("操作原因（必填，UTF-8 最多 500 字节）", { exact: true })
      .fill(adminCancelReason);
    await cancelDialog
      .getByRole("button", { name: "检查并确认", exact: true })
      .click();
    const adminCancel = adminPage.waitForResponse(
      (response) =>
        response
          .url()
          .endsWith(`/api/v1/admin/bet-orders/${secondOrder!.id}/cancel`) &&
        response.request().method() === "POST",
    );
    await cancelDialog
      .getByRole("button", { name: "确认并提交", exact: true })
      .click();
    const adminCancelResponse = await adminCancel;
    const adminCancelEnvelope =
      (await adminCancelResponse.json()) as Envelope<AdminBetOrder>;
    expect(
      adminCancelResponse.status(),
      JSON.stringify(adminCancelEnvelope),
    ).toBe(200);
    expect(adminCancelEnvelope.success).toBe(true);
    expect(adminCancelResponse.request().postDataJSON()).toEqual({
      version: abnormalOnce.order.version,
      reason: adminCancelReason,
    });
    const adminCancelled = adminCancelEnvelope.data;
    expect(adminCancelled.id).toBe(secondOrder.id);
    expect(adminCancelled.status).toBe("bet_cancelled");
    expect(adminCancelled.version).toBe(secondOrder.version + 2);
    expect(adminCancelled.refund_entry_id).toBeTruthy();
    for (const field of [
      "selection_raw",
      "selection_normalized",
      "expanded_bets",
      "definition_snapshot",
      "definition_hash",
      "total_points",
      "policy_snapshot",
      "policy_versions",
      "deduction_allocation",
      "debit_entry_id",
    ] as const) {
      expect(adminCancelled[field], `immutable ${field}`).toEqual(
        secondOrder[field],
      );
    }
    const adminRefundLedger = await api<{ items: LedgerEntry[] }>(
      page.request,
      `${adminBase}/wallets/${memberId}/ledger?limit=100`,
      "GET",
      admin,
    );
    expect(adminRefundLedger.items).toHaveLength(secondLedger.items.length + 1);
    const adminRefund = adminRefundLedger.items.find(
      (entry) => entry.id === adminCancelled.refund_entry_id,
    );
    if (!adminRefund)
      throw new Error("Missing S5-a3 administrator refund ledger entry");
    expect(adminRefund.entry_type).toBe("refund");
    expect(adminRefund.actor_type).toBe("admin");
    expect(adminRefund.reason).toBe(adminCancelReason);
    expect(adminRefund.reference_id).toBe(secondOrder.id);
    expect(adminRefund.reversal_of).toBe(secondDebit.id);
    expect(adminRefund.source_allocation).toEqual(
      secondDebit.source_allocation,
    );
    expect(adminRefund.before_snapshot).toEqual(secondDebit.after_snapshot);
    expect(adminRefund.delta_snapshot.recharge.available).toBe("24");
    expect(adminRefund.after_snapshot).toEqual(secondDebit.before_snapshot);
    expect(adminRefund.after_snapshot).toEqual(refund.after_snapshot);
    expect(
      adminRefundLedger.items.filter(
        (entry) =>
          entry.reference_id === secondOrder!.id && entry.entry_type === "bet",
      ),
    ).toHaveLength(1);
    expect(
      adminRefundLedger.items.filter(
        (entry) =>
          entry.reference_id === secondOrder!.id &&
          entry.entry_type === "refund",
      ),
    ).toHaveLength(1);
    for (const entry of secondLedger.items)
      expect(
        adminRefundLedger.items.find((item) => item.id === entry.id),
      ).toEqual(entry);
    const adminRefundWallet = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(adminRefundWallet.available_points).toBe(
      walletAfter.available_points,
    );
    expect(adminRefundWallet.display_points).toBe(walletBefore.display_points);
    expect(adminRefundWallet.by_source).toEqual(secondDebit.before_snapshot);
    expect(
      await api<{ exception: BetException | null }>(
        page.request,
        `${adminBase}/bet-orders/${secondOrder.id}/exception`,
        "GET",
        admin,
      ),
    ).toEqual(committedEvidence);
    await expect(adminDetail).toContainText(adminRefund.id);
    await expect(adminDetail.locator(".detail-top")).toContainText("投注取消");
    await expect(
      adminDetail.getByRole("button", {
        name: "管理员取消并退款",
        exact: true,
      }),
    ).toHaveCount(0);
    await captureS5Admin("s5-a3-admin-refunded.png");

    // This Harbor-only account cannot select Aurora. A UUID outside this real
    // Harbor fixture must return 404 and clear the previously visible detail.
    const foreignOrderId = crypto.randomUUID();
    await ordersPanel
      .getByLabel("直接查询注单 UUID", { exact: true })
      .fill(foreignOrderId);
    const missingOrder = adminPage.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/v1/admin/bet-orders/${foreignOrderId}`) &&
        response.request().method() === "GET",
    );
    await ordersPanel
      .getByRole("button", { name: "查询", exact: true })
      .click();
    expect((await missingOrder).status()).toBe(404);
    await expect(adminDetail).toHaveCount(0);
    await expect(ordersPanel.locator(".order-row")).toHaveCount(0);
    await expect(ordersPanel.getByRole("alert")).toBeVisible();

    // S5-a4 places two genuine orders against the captured open period, then
    // cancels that whole period and verifies every source refund.
    expect(adminRefundWallet.available_points).toBe("100");
    const cancellationOrders: AdminBetOrder[] = [];
    for (const multiplier of ["1", "2"] as const) {
      const input = { ...secondInput, multiplier };
      const quote = await api<{
        actor_context: string;
        bet_points: string;
        policy_versions: { brand: number; game: number };
      }>(page.request, `${publicBase}/bet-previews`, "POST", userToken, input);
      expect(quote.bet_points).toBe(multiplier === "1" ? "8" : "16");
      const placeResponse = await page.request.post(
        `${publicBase}/bet-orders`,
        {
          headers: {
            Authorization: `Bearer ${userToken}`,
            "X-Brand-ID": brandId,
            Origin: apiOrigin,
            "Idempotency-Key": crypto.randomUUID(),
          },
          data: {
            ...input,
            actor_context: quote.actor_context,
            policy_versions: quote.policy_versions,
          },
        },
      );
      const placeText = await placeResponse.text();
      expect(placeResponse.status(), placeText).toBe(201);
      const order = (JSON.parse(placeText) as Envelope<AdminBetOrder>).data;
      expect(order.status).toBe("placed");
      expect(order.period_id).toBe(placed.period_id);
      expect([placed.id, secondOrder.id]).not.toContain(order.id);
      expect(order.total_points).toBe(multiplier === "1" ? "8" : "16");
      cancellationOrders.push(order);
    }
    const cancellationWallet = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(cancellationWallet.available_points).toBe("76");
    const oldPeriodInput = { ...secondInput, multiplier: "1" };
    const oldPeriodQuote = await api<{
      actor_context: string;
      policy_versions: { brand: number; game: number };
    }>(
      page.request,
      `${publicBase}/bet-previews`,
      "POST",
      userToken,
      oldPeriodInput,
    );
    const cancellationLedgerBefore = await api<{ items: LedgerEntry[] }>(
      page.request,
      `${adminBase}/wallets/${memberId}/ledger?limit=100`,
      "GET",
      admin,
    );
    expect(cancellationLedgerBefore.items).toHaveLength(
      adminRefundLedger.items.length + 2,
    );

    const periodBefore = await api<Period>(
      page.request,
      `${adminBase}/periods/${placed.period_id}`,
      "GET",
      admin,
    );
    expect(periodBefore.id).toBe(placed.period_id);
    expect(["betting", "closed"]).toContain(periodBefore.status);
    const periodPage = async () => {
      if (adminPage.viewportSize()!.width <= 700) {
        await adminPage.locator(".mobile-nav button").nth(2).click();
      } else {
        await adminPage
          .locator(".side-nav")
          .getByRole("button", { name: /期次和开奖/ })
          .click();
      }
      const panel = adminPage.getByTestId("period-cancellation");
      await expect(panel).toBeVisible();
      return panel;
    };
    const cancellationPanel = await periodPage();
    const readPeriod = async (panel: typeof cancellationPanel) => {
      await panel
        .getByLabel("期次 UUID", { exact: true })
        .fill(placed.period_id);
      await panel
        .getByRole("button", { name: "读取期次详情", exact: true })
        .click();
      await expect(
        panel.getByLabel("期次 version", { exact: true }),
      ).toHaveValue(String(periodBefore.version));
    };
    await readPeriod(cancellationPanel);
    await cancellationPanel
      .getByRole("button", { name: "读取取消摘要", exact: true })
      .click();
    await expect(cancellationPanel).toContainText("该期次没有取消任务");
    await cancellationPanel
      .getByLabel("取消模式", { exact: true })
      .selectOption("judged_cancelled");
    await cancellationPanel
      .getByLabel("取消原因", { exact: true })
      .selectOption("no_result");
    const periodCancelReason = `S5-a4 ${info.project.name}: cancel the real period and refund both orders`;
    await cancellationPanel
      .getByLabel("操作原因（UTF-8 不超过 500 字节）", { exact: true })
      .fill(periodCancelReason);
    await cancellationPanel
      .getByRole("button", { name: "检查并确认影响", exact: true })
      .click();

    const periodCancelRoute = `**/api/v1/admin/periods/${placed.period_id}/cancel`;
    const periodCancelRequests: Array<{ key: string; body: string | null }> =
      [];
    let resolvePeriodCancelCommit!: (value: {
      status: number;
      cancellation: Cancellation;
    }) => void;
    const periodCancelCommit = new Promise<{
      status: number;
      cancellation: Cancellation;
    }>((resolve) => {
      resolvePeriodCancelCommit = resolve;
    });
    await adminPage.route(periodCancelRoute, async (route) => {
      const request = route.request();
      periodCancelRequests.push({
        key: (await request.allHeaders())["idempotency-key"],
        body: request.postData(),
      });
      if (periodCancelRequests.length === 1) {
        const committed = await route.fetch();
        const result = (await committed.json()) as Envelope<Cancellation>;
        resolvePeriodCancelCommit({
          status: committed.status(),
          cancellation: result.data,
        });
        await route.abort("failed");
      } else {
        await route.continue();
      }
    });
    await cancellationPanel
      .getByRole("button", { name: "确认取消期次", exact: true })
      .click();
    const cancelCommittedOnce = await periodCancelCommit;
    expect(cancelCommittedOnce.status).toBe(202);
    expect(cancelCommittedOnce.cancellation.state).toBe("processing");
    expect(periodCancelRequests).toHaveLength(1);
    expect(JSON.parse(periodCancelRequests[0].body!)).toEqual({
      version: periodBefore.version,
      mode: "judged_cancelled",
      cause: "no_result",
      reason: periodCancelReason,
    });
    await expect(cancellationPanel).toContainText("有一项写入结果待确认");
    await expect(
      cancellationPanel.getByRole("button", {
        name: "使用原请求重试",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      cancellationPanel.getByLabel("期次 UUID", { exact: true }),
    ).toBeDisabled();

    await adminPage.reload();
    await adminPage
      .getByLabel("选择真实后台品牌", { exact: true })
      .selectOption(brandId);
    const restoredPanel = await periodPage();
    await expect(restoredPanel).toContainText("有一项写入结果待确认");
    await expect(
      restoredPanel.getByLabel("期次 UUID", { exact: true }),
    ).toHaveValue(placed.period_id);
    await expect(
      restoredPanel.getByLabel("期次 UUID", { exact: true }),
    ).toBeDisabled();
    const retryCancel = adminPage.waitForResponse(
      (response) =>
        response
          .url()
          .endsWith(`/api/v1/admin/periods/${placed.period_id}/cancel`) &&
        response.request().method() === "POST",
    );
    await restoredPanel
      .getByRole("button", { name: "使用原请求重试", exact: true })
      .click();
    const retryResponse = await retryCancel;
    const retryText = await retryResponse.text();
    expect(retryResponse.status(), retryText).toBe(202);
    expect(periodCancelRequests).toHaveLength(2);
    expect(periodCancelRequests[1]).toEqual(periodCancelRequests[0]);
    expect((JSON.parse(retryText) as Envelope<Cancellation>).data).toEqual(
      cancelCommittedOnce.cancellation,
    );
    await adminPage.unroute(periodCancelRoute);

    const cancellationSummary = adminPage.getByTestId("period-cancellation");
    await expect(cancellationSummary.locator(".state")).toHaveText("已完成", {
      timeout: 70_000,
    });
    await cancellationSummary
      .getByRole("button", { name: "读取取消摘要", exact: true })
      .click();
    const completed = await api<{ cancellation: Cancellation }>(
      page.request,
      `${adminBase}/periods/${placed.period_id}/cancellation`,
      "GET",
      admin,
    );
    expect(completed.cancellation).toMatchObject({
      id: cancelCommittedOnce.cancellation.id,
      state: "completed",
      total_count: 2,
      refunded_count: 2,
      pending_count: 0,
      already_refunded_count: 0,
      failed_count: 0,
    });
    expect(periodCancelRequests).toHaveLength(2);
    const periodAfter = await api<Period>(
      page.request,
      `${adminBase}/periods/${placed.period_id}`,
      "GET",
      admin,
    );
    expect(periodAfter.status).toBe("judged_cancelled");
    expect(periodAfter.version).toBe(periodBefore.version + 1);

    const cancelledOrders: AdminBetOrder[] = [];
    for (const order of cancellationOrders) {
      const currentOrder = await api<AdminBetOrder>(
        page.request,
        `${adminBase}/bet-orders/${order.id}`,
        "GET",
        admin,
      );
      expect(currentOrder.status).toBe("judged_cancelled");
      expect(currentOrder.refund_entry_id).toBeTruthy();
      cancelledOrders.push(currentOrder);
    }
    const cancellationWalletAfter = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(cancellationWalletAfter.available_points).toBe("100");
    expect(cancellationWalletAfter.display_points).toBe("100");
    expect(cancellationWalletAfter.by_source).toEqual(
      adminRefundWallet.by_source,
    );
    const cancellationLedgerAfter = await api<{ items: LedgerEntry[] }>(
      page.request,
      `${adminBase}/wallets/${memberId}/ledger?limit=100`,
      "GET",
      admin,
    );
    expect(cancellationLedgerAfter.items).toHaveLength(
      cancellationLedgerBefore.items.length + 2,
    );
    for (const oldEntry of cancellationLedgerBefore.items)
      expect(
        cancellationLedgerAfter.items.find((entry) => entry.id === oldEntry.id),
      ).toEqual(oldEntry);
    for (const order of cancelledOrders) {
      const debit = cancellationLedgerBefore.items.find(
        (entry) => entry.id === order.debit_entry_id,
      );
      const refund = cancellationLedgerAfter.items.find(
        (entry) => entry.id === order.refund_entry_id,
      );
      expect(debit).toBeTruthy();
      expect(refund).toMatchObject({
        entry_type: "refund",
        reference_id: order.id,
        reversal_of: order.debit_entry_id,
      });
      expect(refund!.source_allocation).toEqual(debit!.source_allocation);
      expect(refund!.delta_snapshot.recharge.available).toBe(
        order.total_points,
      );
      expect(BigInt(refund!.after_snapshot.recharge.available)).toBe(
        BigInt(refund!.before_snapshot.recharge.available) +
          BigInt(order.total_points),
      );
      expect(
        cancellationLedgerAfter.items.filter(
          (entry) =>
            entry.entry_type === "refund" && entry.reference_id === order.id,
        ),
      ).toHaveLength(1);
    }
    // Multiple outstanding bets are refunded against the current wallet, not
    // against each debit's historical snapshot. Verify the actual version chain.
    const targetIds = new Set(cancelledOrders.map((order) => order.id));
    const taskRefunds = cancellationLedgerAfter.items
      .filter(
        (entry) =>
          entry.entry_type === "refund" && targetIds.has(entry.reference_id),
      )
      .sort((a, b) => a.version - b.version);
    expect(taskRefunds).toHaveLength(2);
    expect(taskRefunds[0].before_snapshot).toEqual(
      cancellationWallet.by_source,
    );
    expect(taskRefunds[1].before_snapshot).toEqual(
      taskRefunds[0].after_snapshot,
    );
    expect(taskRefunds[1].after_snapshot).toEqual(adminRefundWallet.by_source);
    const oldPeriodBet = await page.request.post(`${publicBase}/bet-orders`, {
      headers: {
        Authorization: `Bearer ${userToken}`,
        "X-Brand-ID": brandId,
        Origin: apiOrigin,
        "Idempotency-Key": crypto.randomUUID(),
      },
      data: {
        ...oldPeriodInput,
        actor_context: oldPeriodQuote.actor_context,
        policy_versions: oldPeriodQuote.policy_versions,
      },
    });
    const oldPeriodBetText = await oldPeriodBet.text();
    expect(oldPeriodBet.status(), oldPeriodBetText).toBe(409);
    await expect(cancellationSummary.locator(".counts")).toContainText("2");
    await captureS5Admin("s5-a4-period-cancelled.png");

    // S5-a5 judges one genuine XY order using the currently published numbers
    // play, then proves the lost-response retry reuses its exact request.
    const xyCatalog = await api<{
      game: { id: string };
      plays: Array<{
        id: string;
        definition: { selection: { mode: string } };
        rule_version_id: string;
      }>;
      period: { id: string; status: string } | null;
      policy_versions: { brand: number; game: number };
    }>(page.request, `${publicBase}/games/${xy.id}`, "GET", userToken);
    expect(xyCatalog.game.id).toBe(xy.id);
    expect(xyCatalog.period).toBeTruthy();
    expect(xyCatalog.period!.status).toBe("betting");
    const xyNumbersPlay = xyCatalog.plays.find(
      (play) =>
        play.id === featurePlayId(xy.published, "numbers") &&
        play.definition.selection.mode === "numbers",
    );
    expect(xyNumbersPlay).toBeTruthy();
    const singlePeriodBefore = await api<Period>(
      page.request,
      `${adminBase}/periods/${xyCatalog.period!.id}`,
      "GET",
      admin,
    );
    expect(singlePeriodBefore).toMatchObject({
      id: xyCatalog.period!.id,
      status: "betting",
    });
    const singleInput = {
      period_id: xyCatalog.period!.id,
      play_id: xyNumbersPlay!.id,
      rule_version_id: xyNumbersPlay!.rule_version_id,
      selection: xyRule.validationCase.selection,
      multiplier: "3",
    };
    const singleQuote = await api<{
      actor_context: string;
      unit_points: string;
      bet_points: string;
      policy_versions: { brand: number; game: number };
      period_id: string;
      play_id: string;
      rule_version_id: string;
    }>(
      page.request,
      `${publicBase}/bet-previews`,
      "POST",
      userToken,
      singleInput,
    );
    expect(singleQuote.unit_points).toMatch(/^\d+$/);
    expect(singleQuote.bet_points).toMatch(/^\d+$/);
    expect(BigInt(singleQuote.bet_points)).toBe(
      BigInt(singleQuote.unit_points) * 3n,
    );
    expect(singleQuote.period_id).toBe(singlePeriodBefore.id);
    expect(singleQuote.play_id).toBe(xyNumbersPlay!.id);
    expect(singleQuote.rule_version_id).toBe(xyNumbersPlay!.rule_version_id);
    expect(singleQuote.policy_versions).toEqual(xyCatalog.policy_versions);
    const singleWalletBefore = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(singleWalletBefore.available_points).toBe("100");
    const singlePlaceResponse = await page.request.post(
      `${publicBase}/bet-orders`,
      {
        headers: {
          Authorization: `Bearer ${userToken}`,
          "X-Brand-ID": brandId,
          Origin: apiOrigin,
          "Idempotency-Key": crypto.randomUUID(),
        },
        data: {
          ...singleInput,
          actor_context: singleQuote.actor_context,
          policy_versions: singleQuote.policy_versions,
        },
      },
    );
    const singlePlaceText = await singlePlaceResponse.text();
    expect(singlePlaceResponse.status(), singlePlaceText).toBe(201);
    const singleOrder = (JSON.parse(singlePlaceText) as Envelope<AdminBetOrder>)
      .data;
    expect(singleOrder.status).toBe("placed");
    expect(singleOrder.period_id).toBe(singlePeriodBefore.id);
    expect(singleOrder.unit_points).toBe(singleQuote.unit_points);
    expect(singleOrder.total_points).toBe(singleQuote.bet_points);
    expect(singleOrder.policy_versions).toEqual(singleQuote.policy_versions);
    expect(singleOrder.selection_raw).toEqual(singleInput.selection);
    const singleDebitWallet = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(BigInt(singleDebitWallet.available_points)).toBe(
      BigInt(singleWalletBefore.available_points) -
        BigInt(singleQuote.bet_points),
    );
    const singleLedgerBefore = await api<{ items: LedgerEntry[] }>(
      page.request,
      `${adminBase}/wallets/${memberId}/ledger?limit=100`,
      "GET",
      admin,
    );
    expect(singleLedgerBefore.items).toHaveLength(
      cancellationLedgerAfter.items.length + 1,
    );
    const singleDebit = singleLedgerBefore.items.find(
      (entry) => entry.id === singleOrder.debit_entry_id,
    );
    expect(singleDebit).toMatchObject({
      entry_type: "bet",
      reference_id: singleOrder.id,
    });
    expect(singleDebit!.delta_snapshot.recharge.available).toBe(
      `-${singleQuote.bet_points}`,
    );

    await showS5AdminPage();
    const singleOrdersPanel = adminPage.getByTestId("bet-order-management");
    await singleOrdersPanel
      .getByLabel("直接查询注单 UUID", { exact: true })
      .fill(singleOrder.id);
    await singleOrdersPanel
      .getByRole("button", { name: "查询", exact: true })
      .click();
    const singleDetail = singleOrdersPanel.locator(".detail-panel");
    await expect(singleDetail).toBeVisible();
    await expect(singleDetail).toContainText(singleOrder.id);
    await expect(singleDetail).toContainText(singleOrder.rule_version_id);
    await expect(singleDetail).toContainText(singleOrder.total_points);
    await expect(
      singleDetail.getByRole("button", { name: "判定取消", exact: true }),
    ).toBeVisible();
    await singleDetail
      .getByRole("button", { name: "判定取消", exact: true })
      .click();
    const singleDialog = adminPage.getByRole("dialog");
    await expect(
      singleDialog.getByRole("heading", { name: "确认判定取消", exact: true }),
    ).toBeVisible();
    await singleDialog
      .getByLabel("判定取消原因", { exact: true })
      .selectOption("no_result");
    const singleReason = `S5-a5 ${info.project.name}: no result for the real XY period`;
    await singleDialog
      .getByLabel("操作原因（必填，UTF-8 最多 500 字节）", { exact: true })
      .fill(singleReason);
    expect(new TextEncoder().encode(singleReason).length).toBeLessThanOrEqual(
      500,
    );
    await singleDialog
      .getByRole("button", { name: "检查并确认", exact: true })
      .click();
    await expect(
      singleDialog.getByLabel("判定取消原因", { exact: true }),
    ).toBeDisabled();
    await expect(
      singleDialog.getByLabel("操作原因（必填，UTF-8 最多 500 字节）", {
        exact: true,
      }),
    ).toBeDisabled();
    await expect(singleDialog).toContainText(singleReason);
    const confirmSingleJudge = singleDialog.getByRole("button", {
      name: "确认并提交",
      exact: true,
    });
    const singleJudgeRoute = `**/api/v1/admin/bet-orders/${singleOrder.id}/judge-cancel`;
    const singleJudgeRequests: Array<{
      key: string | undefined;
      body: string | null;
    }> = [];
    let resolveSingleJudgeCommit!: (value: {
      status: number;
      order: AdminBetOrder;
    }) => void;
    const singleJudgeCommit = new Promise<{
      status: number;
      order: AdminBetOrder;
    }>((resolve) => {
      resolveSingleJudgeCommit = resolve;
    });
    await adminPage.route(singleJudgeRoute, async (route) => {
      const request = route.request();
      singleJudgeRequests.push({
        key: (await request.allHeaders())["idempotency-key"],
        body: request.postData(),
      });
      if (singleJudgeRequests.length === 1) {
        const committed = await route.fetch();
        const result = (await committed.json()) as Envelope<AdminBetOrder>;
        resolveSingleJudgeCommit({
          status: committed.status(),
          order: result.data,
        });
        await route.abort("failed");
      } else {
        await route.continue();
      }
    });
    await confirmSingleJudge.click();
    const singleJudgeCommitted = await singleJudgeCommit;
    expect(singleJudgeCommitted.status).toBe(200);
    expect(singleJudgeCommitted.order.status).toBe("judged_cancelled");
    expect(singleJudgeCommitted.order.version).toBe(singleOrder.version + 1);
    expect(singleJudgeRequests).toHaveLength(1);
    const expectedSingleJudgeBody = {
      version: singleOrder.version,
      reason: singleReason,
      cause: "no_result",
    };
    expect(singleJudgeRequests[0].body).toBe(
      JSON.stringify(expectedSingleJudgeBody),
    );
    await expect(singleDetail.getByRole("alert")).toContainText(
      "请求结果尚未确定",
    );
    const retrySingleJudge = singleOrdersPanel.getByRole("button", {
      name: "使用相同请求编号重试",
      exact: true,
    });
    await expect(retrySingleJudge).toBeVisible();
    const retrySingleJudgeResponse = adminPage.waitForResponse(
      (response) =>
        response
          .url()
          .endsWith(
            `/api/v1/admin/bet-orders/${singleOrder.id}/judge-cancel`,
          ) && response.request().method() === "POST",
    );
    // The component refreshes the selected order and then reads these two
    // evidence endpoints in parallel. A separately-issued page.request below
    // does not prove that the UI's own reads have completed, so synchronize on
    // both observable responses before asserting rendered evidence.
    const judgmentEvidenceResponse = adminPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        new URL(response.url()).pathname ===
          `/api/v1/admin/bet-orders/${singleOrder.id}/judgment`,
      { timeout: 0 },
    );
    const exceptionEvidenceResponse = adminPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        new URL(response.url()).pathname ===
          `/api/v1/admin/bet-orders/${singleOrder.id}/exception`,
      { timeout: 0 },
    );
    await retrySingleJudge.click();
    const singleRetryResponse = await retrySingleJudgeResponse;
    const singleRetryText = await singleRetryResponse.text();
    expect(singleRetryResponse.status(), singleRetryText).toBe(200);
    expect(singleJudgeRequests).toHaveLength(2);
    expect(singleJudgeRequests[1]).toEqual(singleJudgeRequests[0]);
    expect(singleJudgeRequests[1].body).toBe(
      JSON.stringify(expectedSingleJudgeBody),
    );
    const singleRetryOrder = (
      JSON.parse(singleRetryText) as Envelope<AdminBetOrder>
    ).data;
    expect(singleRetryOrder).toEqual(singleJudgeCommitted.order);
    const [judgmentEvidenceRead, exceptionEvidenceRead] = await Promise.all([
      judgmentEvidenceResponse,
      exceptionEvidenceResponse,
    ]);
    expect(judgmentEvidenceRead.status()).toBe(200);
    expect(exceptionEvidenceRead.status()).toBe(200);
    await adminPage.unroute(singleJudgeRoute);

    const singleJudgmentResult = await api<{
      judgment: {
        id: string;
        cause: string;
        order_version: number;
        judged_by: string;
        refund_entry_id: string;
        draw_result_id: string;
        reason: string;
      } | null;
    }>(
      page.request,
      `${adminBase}/bet-orders/${singleOrder.id}/judgment`,
      "GET",
      admin,
    );
    expect(singleJudgmentResult.judgment).toMatchObject({
      cause: "no_result",
      order_version: singleOrder.version + 1,
      judged_by: expect.any(String),
      refund_entry_id: expect.any(String),
      draw_result_id: "",
      reason: singleReason,
    });
    const singleJudgment = singleJudgmentResult.judgment!;
    const singleOrderAfter = await api<AdminBetOrder>(
      page.request,
      `${adminBase}/bet-orders/${singleOrder.id}`,
      "GET",
      admin,
    );
    expect(singleOrderAfter.status).toBe("judged_cancelled");
    expect(singleOrderAfter.version).toBe(singleOrder.version + 1);
    expect(singleOrderAfter.refund_entry_id).toBe(
      singleJudgment.refund_entry_id,
    );
    for (const field of [
      "id",
      "brand_id",
      "global_user_id",
      "brand_member_id",
      "account_id",
      "game_id",
      "period_id",
      "play_id",
      "rule_version_id",
      "definition_hash",
      "definition_snapshot",
      "selection_raw",
      "selection_normalized",
      "expanded_bets",
      "unit_points",
      "combination_count",
      "multiplier",
      "total_points",
      "deduction_allocation",
      "policy_snapshot",
      "policy_versions",
      "debit_entry_id",
      "client_key",
      "placed_at",
    ] as const)
      expect(singleOrderAfter[field]).toEqual(singleOrder[field]);
    const judgmentSection = singleDetail.locator("section.subsection").filter({
      has: adminPage.getByRole("heading", {
        name: "判定取消记录",
        exact: true,
      }),
    });
    await expect(judgmentSection).toBeVisible();
    await expect(judgmentSection).toContainText(singleJudgment.id);
    await expect(judgmentSection).toContainText(singleJudgment.judged_by);
    await expect(judgmentSection).toContainText(singleJudgment.refund_entry_id);
    await expect(
      adminPage.locator(".order-row").filter({ hasText: singleOrder.id }),
    ).toContainText("判定取消");
    await expect(
      singleDetail.getByRole("button", { name: "判定取消", exact: true }),
    ).toHaveCount(0);
    const singlePeriodAfter = await api<Period>(
      page.request,
      `${adminBase}/periods/${singlePeriodBefore.id}`,
      "GET",
      admin,
    );
    expect(singlePeriodAfter).toEqual(singlePeriodBefore);
    expect(singlePeriodAfter.status).toBe("betting");
    const singleWalletAfter = await api<Wallet>(
      page.request,
      `${publicBase}/wallet`,
      "GET",
      userToken,
    );
    expect(singleWalletAfter.available_points).toBe("100");
    expect(singleWalletAfter.display_points).toBe(
      singleWalletBefore.display_points,
    );
    expect(BigInt(singleWalletAfter.available_points)).toBe(
      BigInt(singleDebitWallet.available_points) +
        BigInt(singleQuote.bet_points),
    );
    expect(singleWalletAfter.by_source).toEqual(singleWalletBefore.by_source);
    const singleLedgerAfter = await api<{ items: LedgerEntry[] }>(
      page.request,
      `${adminBase}/wallets/${memberId}/ledger?limit=100`,
      "GET",
      admin,
    );
    expect(singleLedgerAfter.items).toHaveLength(
      singleLedgerBefore.items.length + 1,
    );
    for (const oldEntry of singleLedgerBefore.items)
      expect(
        singleLedgerAfter.items.find((entry) => entry.id === oldEntry.id),
      ).toEqual(oldEntry);
    const singleRefund = singleLedgerAfter.items.find(
      (entry) => entry.id === singleJudgment.refund_entry_id,
    );
    expect(singleRefund).toMatchObject({
      entry_type: "refund",
      reference_id: singleOrder.id,
      reversal_of: singleDebit!.id,
      source_allocation: singleDebit!.source_allocation,
    });
    expect(singleRefund!.source_allocation).toEqual(
      singleDebit!.source_allocation,
    );
    expect(singleDebit!.before_snapshot).toEqual(singleWalletBefore.by_source);
    expect(singleDebit!.after_snapshot).toEqual(singleDebitWallet.by_source);
    expect(singleRefund!.before_snapshot).toEqual(singleDebitWallet.by_source);
    expect(singleRefund!.after_snapshot).toEqual(singleWalletAfter.by_source);
    expect(singleDebit!.version).toBe(singleWalletBefore.version + 1);
    expect(singleDebit!.version).toBe(singleDebitWallet.version);
    expect(singleRefund!.version).toBe(singleDebit!.version + 1);
    expect(singleWalletAfter.version).toBe(singleRefund!.version);
    for (const source of ["recharge", "winning", "gift"] as const)
      for (const state of [
        "available",
        "manual_frozen",
        "system_frozen",
        "withdrawal",
      ] as const) {
        const allocated = singleDebit!.source_allocation
          .filter((part) => part.source === source && part.state === state)
          .reduce((sum, part) => sum + BigInt(part.points), 0n);
        const before = BigInt(singleRefund!.before_snapshot[source][state]);
        const debitDelta = BigInt(singleDebit!.delta_snapshot[source][state]);
        const refundDelta = BigInt(singleRefund!.delta_snapshot[source][state]);
        expect(debitDelta).toBe(-allocated);
        expect(refundDelta).toBe(allocated);
        expect(BigInt(singleRefund!.after_snapshot[source][state])).toBe(
          before + allocated,
        );
      }
    expect(
      singleLedgerAfter.items.filter(
        (entry) =>
          entry.entry_type === "refund" &&
          entry.reference_id === singleOrder.id,
      ),
    ).toHaveLength(1);
    await expect(judgmentSection).toContainText("未出结果");
    await expect(judgmentSection).toContainText(singleReason);
    await captureS5Admin("s5-a5-single-judgment.png");

    // S5-c1: use a real near-future period, place through the existing user
    // session, let the worker close it, then record a genuine manual draw.
    const previewRule = fixture("preview_digits", "DIGITS_0_9", "numbers");
    const previewGame = await provisionGame(page.request,admin,reviewer,`e2e_preview_${unique()}`,"Settlement calculation fixture",modelFor("DIGITS_0_9"),[previewRule]);
    const drawAt = new Date(Date.now()+15_000);drawAt.setUTCMilliseconds(0);
    await api(page.request,`${adminBase}/games/${previewGame.id}/schedule`,"PUT",admin,{
      version:previewGame.version,reason:"real short settlement preview window",spec:{timezone:"UTC",mode:"daily",daily_draw_times:[drawAt.toISOString().slice(11,19)],interval_seconds:0,busy_windows:[],bet_open_before_seconds:60,bet_close_before_seconds:5,pause_dates:[],weekdays:[0,1,2,3,4,5,6],holiday_dates:[],holiday_policy:"normal"},
    });
    const generatedPreview = await api<{periods:Period[]}>(page.request,`${adminBase}/games/${previewGame.id}/periods/generate`,"POST",admin,{from:new Date(Date.now()-1_000).toISOString(),to:new Date(Date.now()+60_000).toISOString(),reason:"reserve short draw period"});
    expect(generatedPreview.periods).toHaveLength(1);
    const previewPeriodId=generatedPreview.periods[0].id;
    await expect.poll(async()=> (await api<Period>(page.request,`${adminBase}/periods/${previewPeriodId}`,"GET",admin)).status,{timeout:10_000}).toBe("betting");
    const previewPlayId=previewGame.published[0].id;
    const previewCatalog=await api<{plays:Array<{id:string,rule_version_id:string}>,policy_versions:{brand:number,game:number}}>(page.request,`${publicBase}/games/${previewGame.id}`,"GET",userToken);
    const previewInput={period_id:previewPeriodId,play_id:previewPlayId,rule_version_id:previewCatalog.plays[0].rule_version_id,selection:previewRule.validationCase.selection,multiplier:"1",policy_versions:previewCatalog.policy_versions};
    const realQuote=await api<{actor_context:string}>(page.request,`${publicBase}/bet-previews`,"POST",userToken,previewInput);
    const previewOrder=await api<AdminBetOrder>(page.request,`${publicBase}/bet-orders`,"POST",userToken,{...previewInput,actor_context:realQuote.actor_context},201);
    await expect.poll(async()=> (await api<Period>(page.request,`${adminBase}/periods/${previewPeriodId}`,"GET",admin)).status,{timeout:25_000}).toBe("waiting_draw");
    const previewPeriodBefore=await api<Period>(page.request,`${adminBase}/periods/${previewPeriodId}`,"GET",admin);
    const previewDraw=await api<{id:string}>(page.request,`${adminBase}/periods/${previewPeriodId}/manual-draw`,"POST",admin,{version:previewPeriodBefore.version,period_no:previewPeriodBefore.period_no,result:previewRule.validationCase.draw,drawn_at:drawAt.toISOString(),reason:"genuine manually entered preview result"},201);
    const previewPeriodSnapshot=await api<Period>(page.request,`${adminBase}/periods/${previewPeriodId}`,"GET",admin);
    const previewWallet=await api<Wallet>(page.request,`${publicBase}/wallet`,"GET",userToken);
    const previewLedger=await api<{items:LedgerEntry[]}>(page.request,`${adminBase}/wallets/${memberId}/ledger?limit=100`,"GET",admin);
    await singleOrdersPanel.getByLabel("直接查询注单 UUID",{exact:true}).fill(previewOrder.id);
    await singleOrdersPanel.getByRole("button",{name:"查询",exact:true}).click();
    const computation=singleOrdersPanel.locator(".settlement-preview");
    await expect(computation).toContainText("不派奖、不改变注单或期次状态");
    await expect(singleOrdersPanel).not.toContainText("后台已确认判定取消");
    await expect(computation).toContainText(previewDraw.id);
    const previewRequests:Array<{key:string;body:string|null}>=[];
    const previewRoute=`**/bet-orders/${previewOrder.id}/settlement-previews`;
    let committedPreviewId="";
    await adminPage.route(previewRoute,async route=>{
      if(route.request().method()!=="POST") {await route.continue();return;}
      previewRequests.push({key:route.request().headers()["idempotency-key"]!,body:route.request().postData()});
      const response=await route.fetch();expect(response.status(),await response.text()).toBe(201);
      const record=(await response.json()).data;committedPreviewId=record.id;expect(record.applied).toBe(false);
      if(previewRequests.length===1) await route.abort("failed");else await route.fulfill({response});
    });
    const previewReason=`S5-c1 ${info.project.name}: compute purchased snapshot only`;
    await computation.getByLabel("核对原因（UTF-8 不超过 500 字节）",{exact:true}).fill(previewReason);
    await computation.getByRole("button",{name:"核对本次预览",exact:true}).click();
    const review=computation.locator(".sp-confirm");
    await review.getByRole("checkbox").check();
    await review.getByRole("button",{name:"确认保存只读预览",exact:true}).click();
    await expect(computation.locator(".sp-pending")).toContainText("预览写入结果未知");
    await computation.getByRole("button",{name:"重新读取",exact:true}).click();
    await expect(computation).toContainText(committedPreviewId);
    await expect(computation.locator(".sp-pending")).toContainText("预览写入结果未知");
    await computation.getByRole("button",{name:"使用同一请求与幂等键重试",exact:true}).click();
    await expect(computation).toContainText("预览已保存并重新读取为当前记录");
    expect(previewRequests).toHaveLength(2);expect(previewRequests[0]).toEqual(previewRequests[1]);
    await adminPage.unroute(previewRoute);
    const previewHistory=await api<{items:Array<{id:string,outcome:string,applied:boolean,calculation:{prize_points:string},current:boolean}>}>(page.request,`${adminBase}/bet-orders/${previewOrder.id}/settlement-previews?limit=20&offset=0`,"GET",admin);
    expect(previewHistory.items).toHaveLength(1);expect(previewHistory.items[0]).toMatchObject({id:committedPreviewId,outcome:"won",applied:false,current:true,calculation:{prize_points:"8"}});
    await expect(computation).toContainText("命中");
    await expect(computation).toContainText("WIN");
    expect(await api<Wallet>(page.request,`${publicBase}/wallet`,"GET",userToken)).toEqual(previewWallet);
    expect(await api<{items:LedgerEntry[]}>(page.request,`${adminBase}/wallets/${memberId}/ledger?limit=100`,"GET",admin)).toEqual(previewLedger);
    expect(await api<Period>(page.request,`${adminBase}/periods/${previewPeriodId}`,"GET",admin)).toEqual(previewPeriodSnapshot);
    expect((await api<AdminBetOrder>(page.request,`${adminBase}/bet-orders/${previewOrder.id}`,"GET",admin))).toEqual(previewOrder);
    await captureS5Admin("s5-c1-real-settlement-preview.png");
    await computation.screenshot({path:info.outputPath("s5-c1-calculation-panel.png")});

    // A valid POST receipt stays definitive when a subsequent readonly GET
    // loses its response. This is a network abort, not a fake business result.
    let acknowledgedId="";
    const postReceiptReason=`S5-c1 ${info.project.name}: acknowledgement precedes read outage`;
    await adminPage.route(previewRoute,async route=>{
      if(route.request().method()!=="POST") {await route.continue();return;}
      const response=await route.fetch();expect(response.status()).toBe(201);
      acknowledgedId=(await response.json()).data.id;
      await adminPage.route(`**/settlement-previews/${acknowledgedId}`,r=>r.abort("failed"));
      await route.fulfill({response});
    });
    await computation.getByLabel("核对原因（UTF-8 不超过 500 字节）",{exact:true}).fill(postReceiptReason);
    await computation.getByRole("button",{name:"核对本次预览",exact:true}).click();
    await computation.locator(".sp-confirm").getByRole("checkbox").check();
    await computation.locator(".sp-confirm").getByRole("button",{name:"确认保存只读预览",exact:true}).click();
    await expect(computation).toContainText("预览已由服务端确认保存，但后续读取失败");
    await expect(computation.locator(".sp-pending")).toHaveCount(0);
    await adminPage.unroute(previewRoute);
    await adminPage.unroute(`**/settlement-previews/${acknowledgedId}`);
    await computation.getByRole("button",{name:"重新读取",exact:true}).click();
    await expect(computation).toContainText(acknowledgedId);
    await expect(computation.locator(".sp-pending")).toHaveCount(0);

    // A third independently confirmed computation is committed first, then
    // its real administrator session is revoked before the follow-up GETs.
    let sessionEndedPreviewId="";
    await adminPage.route(previewRoute,async route=>{
      if(route.request().method()!=="POST") {await route.continue();return;}
      const response=await route.fetch();expect(response.status()).toBe(201);
      sessionEndedPreviewId=(await response.json()).data.id;
      const logout=await adminPage.request.post(`${adminOrigin}/api/v1/admin/auth/logout`,{headers:{Authorization:`Bearer ${admin}`,Origin:adminOrigin,"Idempotency-Key":crypto.randomUUID()},data:{}});
      expect(logout.status(),await logout.text()).toBe(200);
      await route.fulfill({response});
    });
    await computation.getByLabel("核对原因（UTF-8 不超过 500 字节）",{exact:true}).fill(`S5-c1 ${info.project.name}: acknowledgement precedes real logout`);
    await computation.getByRole("button",{name:"核对本次预览",exact:true}).click();
    await computation.locator(".sp-confirm").getByRole("checkbox").check();
    await computation.locator(".sp-confirm").getByRole("button",{name:"确认保存只读预览",exact:true}).click();
    await expect(adminPage.getByTestId("bet-order-management")).toHaveCount(0);
    await expect(adminPage.getByRole("button",{name:"退出登录",exact:true})).toHaveCount(0);
    await adminPage.unroute(previewRoute);
    const finalPreviewHistory=await api<{items:Array<{id:string,applied:boolean}>}>(page.request,`${adminBase}/bet-orders/${previewOrder.id}/settlement-previews?limit=20&offset=0`,"GET",reviewer);
    expect(finalPreviewHistory.items).toHaveLength(3);
    expect(new Set(finalPreviewHistory.items.map(v=>v.id)).size).toBe(3);
    expect(finalPreviewHistory.items.map(v=>v.id)).toContain(sessionEndedPreviewId);
    expect(finalPreviewHistory.items.every(v=>v.applied===false)).toBe(true);
    expect(await api<Wallet>(page.request,`${publicBase}/wallet`,"GET",userToken)).toEqual(previewWallet);

    // S5-c2: reuse the genuine independent reviewer session after the creator's
    // real logout. No additional login and no manufactured business response.
    await context.addCookies(reviewerCookies);
    await adminPage.reload();
    await adminPage.getByLabel("选择真实后台品牌",{exact:true}).selectOption(brandId);
    await periodPage();
    const settlement=adminPage.locator(".settlement-management");
    await expect(settlement).toContainText("未配置");
    await settlement.getByLabel("新模式",{exact:true}).selectOption("manual");
    await settlement.getByLabel("操作原因（UTF-8 不超过 500 字节）",{exact:true}).fill(`S5-c2 ${info.project.name}: explicit isolated manual mode`);
    await settlement.getByRole("button",{name:"核对配置变更",exact:true}).click();
    await settlement.locator(".sm-confirm").getByRole("checkbox").check();
    await settlement.getByRole("button",{name:"确认提交结算操作",exact:true}).click();
    await expect(settlement).toContainText("服务器已确认写入");
    await settlement.getByLabel("期次 ID",{exact:true}).fill(previewPeriodId);
    await expect(settlement).toContainText(previewDraw.id);
    await expect(settlement.getByRole("button",{name:"核对并启动新结算任务",exact:true})).toBeDisabled();
    await settlement.getByLabel("启动原因（UTF-8 不超过 500 字节）",{exact:true}).fill(`S5-c2 ${info.project.name}: purchased snapshot batch`);
    const settleRequests:Array<{key:string;body:string|null}>=[];
    const settleRoute=`**/periods/${previewPeriodId}/settle`;
    let settlementId="";
    await adminPage.route(settleRoute,async route=>{
      if(route.request().method()!=="POST"){await route.continue();return;}
      settleRequests.push({key:route.request().headers()["idempotency-key"]!,body:route.request().postData()});
      const response=await route.fetch();expect(response.status(),await response.text()).toBe(201);
      settlementId=(await response.json()).data.id;
      if(settleRequests.length===1)await route.abort("failed");else await route.fulfill({response});
    });
    await settlement.getByRole("button",{name:"核对并启动新结算任务",exact:true}).click();
    await settlement.locator(".sm-confirm").getByRole("checkbox").check();
    await settlement.getByRole("button",{name:"确认提交结算操作",exact:true}).click();
    await expect(settlement.locator(".sm-pending")).toContainText("写入结果未知");
    await expect.poll(async()=> (await api<{state:string}>(page.request,`${adminBase}/settlement-jobs/${settlementId}`,"GET",reviewer)).state,{timeout:10_000}).toBe("awaiting_approval");
    await settlement.getByRole("button",{name:"重新读取",exact:true}).click();
    await expect(settlement).toContainText(settlementId);
    await expect(settlement.locator(".sm-pending")).toContainText("写入结果未知");
    expect(await api<Wallet>(page.request,`${publicBase}/wallet`,"GET",userToken)).toEqual(previewWallet);
    await settlement.getByRole("button",{name:"使用原请求和幂等键重试",exact:true}).click();
    await expect(settlement.locator(".sm-pending")).toHaveCount(0);
    await expect(settlement).toContainText("全部可结算目标已完成核算，仍未入账");
    expect(settleRequests).toHaveLength(2);expect(settleRequests[0]).toEqual(settleRequests[1]);
    await adminPage.unroute(settleRoute);
    const actualJob=(await api<{settlement:{id:string,state:string,prize_points:string,paid_points:string,ready_count:number}}>(page.request,`${adminBase}/periods/${previewPeriodId}/settlement`,"GET",reviewer)).settlement;
    expect(actualJob).toMatchObject({id:settlementId,state:"awaiting_approval",prize_points:"8",paid_points:"0",ready_count:1});
    const approvalRoute=`**/settlement-jobs/${settlementId}/approve`;
    const jobRoute=`**/settlement-jobs/${settlementId}`;
    await adminPage.route(approvalRoute,async route=>{
      const response=await route.fetch();expect(response.status(),await response.text()).toBe(200);
      await adminPage.route(jobRoute,r=>r.abort("failed"));
      await route.fulfill({response});
    });
    await settlement.getByLabel("运营批准原因（UTF-8 不超过 500 字节）",{exact:true}).fill(`S5-c2 ${info.project.name}: approve verified integer winnings`);
    await settlement.getByRole("button",{name:"核对并批准，允许 worker 入账",exact:true}).click();
    await settlement.locator(".sm-confirm").getByRole("checkbox").check();
    await settlement.getByRole("button",{name:"确认提交结算操作",exact:true}).click();
    await expect(settlement).toContainText("服务器已确认写入，但后续读取失败");
    await expect(settlement.locator(".sm-pending")).toHaveCount(0);
    await adminPage.unroute(approvalRoute);await adminPage.unroute(jobRoute);
    await expect.poll(async()=> (await api<{state:string}>(page.request,`${adminBase}/settlement-jobs/${settlementId}`,"GET",reviewer)).state,{timeout:10_000}).toBe("completed");
    await settlement.getByRole("button",{name:"重新读取",exact:true}).click();
    await expect(settlement).toContainText("服务器任务已完成，且全部目标均已入账或排除");
    await expect(settlement).toContainText("已入账积分");
    const paidWallet=await api<Wallet>(page.request,`${publicBase}/wallet`,"GET",userToken);
    expect(BigInt(paidWallet.available_points)).toBe(BigInt(previewWallet.available_points)+8n);
    expect(paidWallet.by_source.recharge).toEqual(previewWallet.by_source.recharge);
    expect(paidWallet.by_source.gift).toEqual(previewWallet.by_source.gift);
    expect(BigInt(paidWallet.by_source.winning.available)).toBe(BigInt(previewWallet.by_source.winning.available)+8n);
    const paidOrder=await api<AdminBetOrder>(page.request,`${adminBase}/bet-orders/${previewOrder.id}`,"GET",reviewer);
    expect(paidOrder).toMatchObject({status:"won",prize_points:"8",version:previewOrder.version+1});
    const paidLedger=await api<{items:LedgerEntry[]}>(page.request,`${adminBase}/wallets/${memberId}/ledger?limit=100`,"GET",reviewer);
    const prizes=paidLedger.items.filter(v=>v.entry_type==="prize"&&v.reference_id===paidOrder.settlement_calculation_id);
    expect(prizes).toHaveLength(1);expect(prizes[0].id).toBe(paidOrder.payout_entry_id);
    expect(prizes[0].before_snapshot).toEqual(previewWallet.by_source);
    expect(prizes[0].after_snapshot).toEqual(paidWallet.by_source);
    expect((await api<Period>(page.request,`${adminBase}/periods/${previewPeriodId}`,"GET",reviewer)).status).toBe("settled");
    expect(await adminPage.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
    await settlement.screenshot({path:info.outputPath("s5-c2-real-settlement-panel.png")});
    await adminPage.screenshot({path:info.outputPath("s5-c2-real-settlement-page.png"),fullPage:true});
    await page.goto(`${harborSite}/orders/${previewOrder.id}`);
    await expect(page.getByTestId("order-detail")).toContainText("Prize points 8");
  } finally {
    await adminPage.close();
  }
});

function featurePlayId(
  plays: Array<{ id: string; mode: string }>,
  mode: string,
) {
  const play = plays.find((item) => item.mode === mode);
  if (!play) throw new Error(`Missing provisioned ${mode} play`);
  return play.id;
}
