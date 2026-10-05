import { test, expect, type Page, type TestInfo } from "@playwright/test";

const harbor = "0199a000-0000-7000-8000-000000000002";
async function showPeriods(page: Page, info: TestInfo) {
  if (info.project.name === "mobile") {
    await page
      .locator(".mobile-nav")
      .getByRole("button", { name: /期次/ })
      .click();
  } else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /期次和开奖/ })
      .click();
}
test("schedule revisions and generated periods persist without activating future windows", async ({
  page,
}, info) => {
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Provide isolated brand-admin credentials",
  );
  await page.goto("http://localhost:5174");
  if (info.project.name === "mobile")
    await page.locator(".mobile-nav button").nth(1).click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /用户和成员/ })
      .click();
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.TEST_HARBOR_ADMIN_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.TEST_HARBOR_ADMIN_PASSWORD!);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  const code = `schedule_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`;
  const created = await page.request.post(
    "http://localhost:5174/api/v1/admin/games",
    {
      headers: {
        "X-Brand-ID": harbor,
        "Idempotency-Key": crypto.randomUUID(),
        Origin: "http://localhost:5174",
      },
      data: {
        code,
        name: "Browser schedule game",
        model: {
          model: "DIGITS_0_9",
          length: 3,
          allow_repeat: true,
          ordered: true,
        },
        timezone: "UTC",
        reason: "isolated browser calendar game",
      },
    },
  );
  expect(created.status(), await created.text()).toBe(201);
  const game = (await created.json()).data;
  await showPeriods(page, info);
  const panel = page.locator(".period-schedules");
  await panel.getByLabel("彩种", { exact: true }).selectOption(game.id);
  await expect(
    panel.getByText("尚无已保存排期", { exact: false }),
  ).toBeVisible();
  const draw = new Date(Date.now() + 3 * 3600 * 1000);
  draw.setUTCMilliseconds(0);
  await panel
    .getByLabel("每日开奖时间（HH:MM:SS，以逗号分隔）", { exact: true })
    .fill(draw.toISOString().slice(11, 19));
  await panel.getByLabel("投注开放提前秒数", { exact: true }).fill("300");
  await panel.getByLabel("投注截止提前秒数", { exact: true }).fill("30");
  const scheduleSection = panel.locator("section.panel").filter({
    has: page.getByRole("heading", { name: "排期配置", exact: true }),
  });
  const save = async (reason: string) => {
    await scheduleSection
      .getByLabel("操作原因（必填）", { exact: true })
      .fill(reason);
    const response = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/games/${game.id}/schedule`) &&
        r.request().method() === "PUT",
    );
    await scheduleSection
      .getByRole("button", { name: "保存新修订版", exact: true })
      .click();
    const reply = await response;
    expect(reply.status(), await reply.text()).toBe(200);
    await expect(
      scheduleSection.getByRole("button", {
        name: "保存新修订版",
        exact: true,
      }),
    ).toBeVisible();
    return (await reply.json()).data;
  };
  const first = await save("first calendar revision");
  const second = await save("new immutable revision with unchanged windows");
  expect(first.revision).toBe(1);
  expect(second.revision).toBe(2);
  expect(second.id).not.toBe(first.id);
  const generation = panel.locator("section.panel").filter({
    has: page.getByRole("heading", { name: "生成期数", exact: true }),
  });
  await generation
    .getByLabel("开始（彩种本地时间）", { exact: true })
    .fill(new Date(draw.valueOf() - 3600 * 1000).toISOString().slice(0, 19));
  await generation
    .getByLabel("结束（彩种本地时间）", { exact: true })
    .fill(new Date(draw.valueOf() + 3600 * 1000).toISOString().slice(0, 19));
  await generation
    .getByLabel("操作原因（必填）", { exact: true })
    .fill("future period reservation");
  const generateResponse = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/games/${game.id}/periods/generate`) &&
      r.request().method() === "POST",
  );
  await generation
    .getByRole("button", { name: "生成期数", exact: true })
    .click();
  const generated = await generateResponse;
  expect(generated.status(), await generated.text()).toBe(200);
  const result = (await generated.json()).data;
  expect(result.created + result.existing).toBe(1);
  expect(result.periods).toHaveLength(1);
  expect(result.periods[0].status).toBe("pending");
  // A concurrently running worker may have reserved the same window under v1.
  // Immutable reservations retain their original schedule, never get rewritten.
  expect([first.id, second.id]).toContain(result.periods[0].schedule_id);
  await expect(
    generation.getByRole("button", { name: "生成期数", exact: true }),
  ).toBeDisabled(); // Reason is cleared, not stuck busy.
  await generation
    .getByLabel("操作原因（必填）", { exact: true })
    .fill("repeat reservation without duplicates");
  const repeatResponse = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/games/${game.id}/periods/generate`) &&
      r.request().method() === "POST",
  );
  await generation
    .getByRole("button", { name: "生成期数", exact: true })
    .click();
  const repeated = await repeatResponse;
  expect(repeated.status(), await repeated.text()).toBe(200);
  const repeatedData = (await repeated.json()).data;
  expect(repeatedData.created).toBe(0);
  expect(repeatedData.existing).toBe(1);
  expect(repeatedData.periods[0].id).toBe(result.periods[0].id);
  await page.reload();
  await showPeriods(page, info);
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  await panel.getByLabel("彩种", { exact: true }).selectOption(game.id);
  await expect(scheduleSection).toContainText("当前修订版 2");
  await expect(
    panel
      .locator(".period-card")
      .filter({ hasText: result.periods[0].period_no }),
  ).toContainText("待开始");
  const catalog = await page.request.get(
    "http://localhost:5174/api/v1/admin/games",
    { headers: { "X-Brand-ID": harbor } },
  );
  expect(catalog.status()).toBe(200);
  expect(
    (await catalog.json()).data.games.find(
      (item: { id: string }) => item.id === game.id,
    ).started_sequence,
  ).toBe(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("period-schedules.png"),
    fullPage: true,
  });
});

test("running worker opens, closes and advances a real period using the database clock", async ({
  page,
}, info) => {
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Provide isolated brand-admin credentials and run platform worker",
  );
  await page.goto("http://localhost:5174");
  if (info.project.name === "mobile")
    await page.locator(".mobile-nav button").nth(1).click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /用户和成员/ })
      .click();
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.TEST_HARBOR_ADMIN_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.TEST_HARBOR_ADMIN_PASSWORD!);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  const headers = { "X-Brand-ID": harbor, Origin: "http://localhost:5174" };
  const post = async (path: string, data: unknown) =>
    page.request.post(`http://localhost:5174/api/v1/admin${path}`, {
      headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
      data,
    });
  const gameReply = await post("/games", {
    code: `clock_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`,
    name: "Worker clock game",
    model: {
      model: "DIGITS_0_9",
      length: 3,
      allow_repeat: true,
      ordered: true,
    },
    timezone: "UTC",
    reason: "isolated worker clock test",
  });
  expect(gameReply.status(), await gameReply.text()).toBe(201);
  const game = (await gameReply.json()).data;
  const draw = new Date(Date.now() + 15_000);
  draw.setUTCMilliseconds(0);
  const schedule = await page.request.put(
    `http://localhost:5174/api/v1/admin/games/${game.id}/schedule`,
    {
      headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
      data: {
        version: game.version,
        spec: {
          timezone: "UTC",
          mode: "daily",
          daily_draw_times: [draw.toISOString().slice(11, 19)],
          interval_seconds: 0,
          busy_windows: [],
          bet_open_before_seconds: 20,
          bet_close_before_seconds: 5,
          pause_dates: [],
          weekdays: [0, 1, 2, 3, 4, 5, 6],
          holiday_dates: [],
          holiday_policy: "normal",
        },
        reason: "near-future controlled clock window",
      },
    },
  );
  expect(schedule.status(), await schedule.text()).toBe(200);
  const generated = await post(`/games/${game.id}/periods/generate`, {
    from: new Date(draw.valueOf() - 1000).toISOString(),
    to: new Date(draw.valueOf() + 1000).toISOString(),
    reason: "reserve actual clock period",
  });
  expect(generated.status(), await generated.text()).toBe(200);
  const period = (await generated.json()).data.periods[0];
  expect(["pending", "betting"]).toContain(period.status);
  const status = async () => {
    const reply = await page.request.get(
      `http://localhost:5174/api/v1/admin/games/${game.id}/periods`,
      { headers },
    );
    expect(reply.status(), await reply.text()).toBe(200);
    return (await reply.json()).data.periods.find(
      (p: { id: string }) => p.id === period.id,
    )?.status;
  };
  await expect
    .poll(status, { timeout: 7000, intervals: [250, 500, 1000] })
    .toBe("betting");
  await expect
    .poll(status, { timeout: 15_000, intervals: [500, 1000] })
    .toBe("waiting_draw");
  const catalog = await page.request.get(
    "http://localhost:5174/api/v1/admin/games?limit=100",
    { headers },
  );
  expect(catalog.status()).toBe(200);
  expect(
    (await catalog.json()).data.games.find(
      (p: { id: string }) => p.id === game.id,
    ).started_sequence,
  ).toBe(1);
  await showPeriods(page, info);
  await page
    .locator(".period-schedules")
    .getByLabel("彩种", { exact: true })
    .selectOption(game.id);
  await expect(
    page.locator(".period-card").filter({ hasText: period.period_no }),
  ).toContainText("待开奖");
});
