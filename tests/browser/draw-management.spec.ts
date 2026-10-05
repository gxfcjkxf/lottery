import { test, expect } from "@playwright/test";

const harbor = "0199a000-0000-7000-8000-000000000002";
test("source revisions, worker attempt evidence and manual draw persist in the real database", async ({
  page,
}, info) => {
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Requires isolated admin credentials and platform worker",
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
  const base = "http://localhost:5174/api/v1/admin";
  const gameReply = await page.request.post(`${base}/games`, {
    headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
    data: {
      code: `draw_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`,
      name: "Browser manual draw",
      timezone: "UTC",
      model: {
        model: "DIGITS_0_9",
        length: 3,
        allow_repeat: true,
        ordered: true,
      },
      reason: "isolated draw regression",
    },
  });
  expect(gameReply.status(), await gameReply.text()).toBe(201);
  const game = (await gameReply.json()).data;
  const drawAt = new Date(Date.now() + 20_000);
  drawAt.setUTCMilliseconds(0);
  const scheduleReply = await page.request.put(
    `${base}/games/${game.id}/schedule`,
    {
      headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
      data: {
        version: game.version,
        reason: "controlled near-future draw",
        spec: {
          timezone: "UTC",
          mode: "daily",
          daily_draw_times: [drawAt.toISOString().slice(11, 19)],
          interval_seconds: 0,
          busy_windows: [],
          bet_open_before_seconds: 30,
          bet_close_before_seconds: 5,
          weekdays: [0, 1, 2, 3, 4, 5, 6],
          pause_dates: [],
          holiday_dates: [],
          holiday_policy: "normal",
        },
      },
    },
  );
  expect(scheduleReply.status(), await scheduleReply.text()).toBe(200);
  const generated = await page.request.post(
    `${base}/games/${game.id}/periods/generate`,
    {
      headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
      data: {
        from: new Date(drawAt.valueOf() - 1000).toISOString(),
        to: new Date(drawAt.valueOf() + 1000).toISOString(),
        reason: "reserve draw regression window",
      },
    },
  );
  expect(generated.status(), await generated.text()).toBe(200);
  const period = (await generated.json()).data.periods[0];
  const getPeriod = async () => {
    const response = await page.request.get(
      `${base}/games/${game.id}/periods`,
      { headers },
    );
    expect(response.status(), await response.text()).toBe(200);
    return (await response.json()).data.periods.find(
      (p: { id: string }) => p.id === period.id,
    );
  };
  await expect
    .poll(async () => (await getPeriod()).status, { timeout: 5000 })
    .toBe("betting");
  if (info.project.name === "mobile")
    await page
      .locator(".mobile-nav")
      .getByRole("button", { name: /期次/ })
      .click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /期次和开奖/ })
      .click();
  const panel = page.locator(".draw-management");
  await panel
    .getByRole("combobox", { name: "彩种", exact: true })
    .selectOption(game.id);
  await expect(
    panel.getByText("尚无来源配置。", { exact: true }),
  ).toBeVisible();
  for (let i = 0; i < 2; i++) {
    await panel.getByRole("button", { name: "添加来源", exact: true }).click();
    const card = panel.locator(".source-card").nth(i);
    await card
      .getByLabel("名称", { exact: true })
      .fill(i ? "Backup DOM" : "Primary API");
    await card
      .getByRole("textbox", { name: /^HTTPS/ })
      .fill(`https://results.example.com/${i ? "backup" : "main"}`);
    if (i) {
      await card
        .getByRole("combobox", { name: "类型", exact: true })
        .selectOption("dom");
      await card.getByLabel("DOM 选择器", { exact: true }).fill("#results");
    }
  }
  const save = async (reason: string) => {
    await panel.getByLabel("修订原因", { exact: true }).fill(reason);
    const replyPromise = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/games/${game.id}/draw-sources`) &&
        r.request().method() === "PUT",
    );
    await panel
      .getByRole("button", { name: "保存新修订", exact: true })
      .click();
    const reply = await replyPromise;
    expect(reply.status(), await reply.text()).toBe(200);
    await expect(
      panel.getByRole("button", { name: "保存新修订", exact: true }),
    ).toBeVisible();
    return (await reply.json()).data;
  };
  const first = await save("primary and fallback no-network stubs");
  await panel
    .locator(".source-card")
    .first()
    .getByLabel("名称", { exact: true })
    .fill("Primary API revision 2");
  const second = await save("append immutable revision");
  expect(second.revision).toBe(2);
  expect(second.id).not.toBe(first.id);
  await panel
    .locator("section.panel")
    .filter({
      has: page.getByRole("heading", { name: "自动开奖来源", exact: true }),
    })
    .getByRole("button", { name: "重新读取", exact: true })
    .click();
  await expect(
    panel.locator(".source-card").first().getByLabel("名称", { exact: true }),
  ).toHaveValue("Primary API revision 2");
  await expect
    .poll(async () => (await getPeriod()).status, {
      timeout: 25_000,
      intervals: [500, 1000],
    })
    .toBe("waiting_draw");
  const getHistory = async () => {
    const response = await page.request.get(
      `${base}/periods/${period.id}/draw`,
      { headers },
    );
    expect(response.status(), await response.text()).toBe(200);
    return (await response.json()).data;
  };
  await expect
    .poll(async () => (await getHistory()).attempts.length, { timeout: 7000 })
    .toBeGreaterThan(0);
  const attempts = (await getHistory()).attempts[0];
  expect(attempts.status).toBe("no_data");
  expect(attempts.source_set_id).toBe(second.id);
  expect(
    attempts.attempts.map((a: { source_id: string }) => a.source_id),
  ).toEqual(second.sources.map((s: { id: string }) => s.id));
  await panel.getByRole("button", { name: "刷新期数", exact: true }).click();
  await panel.getByLabel(/数字位置/).fill("1,2,1");
  await panel.getByLabel(/开奖时间/).fill(drawAt.toISOString());
  await panel
    .getByLabel("操作原因（必填）", { exact: true })
    .fill("verified isolated manual result");
  await panel.getByLabel(/我确认这是核实后的真实开奖结果/).check();
  const manualPromise = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/periods/${period.id}/manual-draw`) &&
      r.request().method() === "POST",
  );
  await panel
    .getByRole("button", { name: "确认并持久保存", exact: true })
    .click();
  const manual = await manualPromise;
  expect(manual.status(), await manual.text()).toBe(201);
  const result = (await manual.json()).data;
  const sent = manual.request();
  const replay = await page.request.post(sent.url(), {
    headers: {
      ...headers,
      "Idempotency-Key": sent.headers()["idempotency-key"],
    },
    data: sent.postDataJSON(),
  });
  expect(replay.status(), await replay.text()).toBe(201);
  expect((await replay.json()).data.id).toBe(result.id);
  const locked = await getPeriod();
  expect(locked.status).toBe("drawn");
  const overwrite = await page.request.post(sent.url(), {
    headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
    data: { ...sent.postDataJSON(), version: locked.version },
  });
  expect(overwrite.status(), await overwrite.text()).toBe(409);
  const history = await getHistory();
  expect(history.current.id).toBe(result.id);
  expect(history.history).toHaveLength(1);
  await expect(panel.getByText(/当前开奖结果已由人工录入/)).toBeVisible();
  await page.reload();
  if (info.project.name === "mobile")
    await page
      .locator(".mobile-nav")
      .getByRole("button", { name: /期次/ })
      .click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /期次和开奖/ })
      .click();
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  await page
    .locator(".draw-management")
    .getByRole("combobox", { name: "彩种", exact: true })
    .selectOption(game.id);
  await expect(page.locator(".draw-management .current-result")).toContainText(
    "1 2 1",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("draw-management.png"),
    fullPage: true,
  });
});
