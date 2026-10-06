import {
  test,
  expect,
  type APIRequestContext,
  type Page,
  type Route,
} from "@playwright/test";

const brandId = "0199a000-0000-7000-8000-000000000002";
const apiOrigin = "http://localhost:5173";
const adminBase = `${apiOrigin}/api/v1/admin`;
const publicBase = `${apiOrigin}/api/v1/b/harbor`;
const harborSite = "http://harbor.localhost:5173";
const unique = () => crypto.randomUUID().replaceAll("-", "").slice(0, 12);

// Node's fetch cannot resolve *.localhost while Chromium can. Send the actual
// intercepted request through loopback, preserving its registered HTTP authority,
// Origin, Cookie and body; the real server still authenticates the Harbor member.
async function fetchHarbor(route: Route) {
  return route.fetch({
    url: route.request().url().replace("harbor.localhost", "localhost"),
    headers: {
      ...(await route.request().allHeaders()),
      host: "harbor.localhost:5173",
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
        mode: "interval",
        daily_draw_times: [],
        interval_seconds: 300,
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
    from: new Date(now + 120_000).toISOString(),
    to: new Date(now + 600_000).toISOString(),
    reason: "generate real near-future interval draws",
  });
  expect(generated.created).toBeGreaterThan(0);
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
  const reviewer = await adminToken(
    page.request,
    process.env.TEST_RULE_REVIEWER_USERNAME!,
    process.env.TEST_RULE_REVIEWER_PASSWORD!,
  );

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
  await page.goto(`${harborSite}/games`);
  await expect(
    catalog
      .getByTestId("catalog-game")
      .filter({ hasText: "E2E positional digits" }),
  ).toBeVisible();
  await expect(
    catalog.getByTestId("catalog-game").filter({ hasText: "E2E X plus Y" }),
  ).toBeVisible();
  await expect(
    catalog.getByTestId("catalog-game").filter({ hasText: "E2E M select N" }),
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
    .filter({ hasText: "E2E positional digits" });
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
  expect(await page.evaluate(() => document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await page.screenshot({path:info.outputPath("betting-selection.png"),fullPage:true});
  await quote.getByTestId("review-bet").click();
  await expect(page.getByTestId("bet-confirmation")).toBeVisible();
  await expect(page.getByTestId("selection-summary")).toContainText("Position 3");
  expect(await page.evaluate(() => document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await page.screenshot({path:info.outputPath("betting-confirmation.png"),fullPage:true});
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
  await page.unroute(placeRoute);
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
  await page.unroute(cancelRoute);
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
  expect(await page.evaluate(() => document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
  await expect(orderDetail.locator(".wallet-balance")).toContainText("100");
  await page.screenshot({path:info.outputPath("betting-refunded.png"),fullPage:true});
});

function featurePlayId(
  plays: Array<{ id: string; mode: string }>,
  mode: string,
) {
  const play = plays.find((item) => item.mode === mode);
  if (!play) throw new Error(`Missing provisioned ${mode} play`);
  return play.id;
}
