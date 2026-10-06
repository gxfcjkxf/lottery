import { test, expect } from "@playwright/test";

const harbor = "0199a000-0000-7000-8000-000000000002";
test("source revisions, worker attempt evidence and manual draw persist in the real database", async ({
  page,
}, info) => {
  test.setTimeout(90_000);
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
  await panel.getByLabel(/数字位置/).fill("0,1,0");
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
  // Calendar prefill can add a future period while this test runs. Select the
  // actual manually drawn period rather than relying on newest-first defaults.
  await page
    .locator(".draw-management")
    .getByRole("combobox", { name: "期数", exact: true })
    .selectOption(period.id);
  await expect(page.locator(".draw-management .current-result")).toContainText(
    "0 1 0",
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

  // Public reads expose the chosen result, not source configuration or actors.
  const publicBase = "http://localhost:5173/api/v1/b/harbor";
  const publicResult = await page.request.get(
    `${publicBase}/draw-results?game_id=${game.id}&period_no=${encodeURIComponent(period.period_no)}`,
  );
  expect(publicResult.status(), await publicResult.text()).toBe(200);
  const published = (await publicResult.json()).data;
  expect(published.items).toHaveLength(1);
  expect(published.items[0]).toMatchObject({
    id: result.id,
    game: { id: game.id },
    period: { id: period.id, status: "drawn", draw_result_id: result.id },
    result: { regular: [], special: [], digits: [0, 1, 0] },
    origin: "manual",
  });
  for (const privateField of [
    "source_id",
    "created_by",
    "corrected_from_id",
    "credential_ref",
    "endpoint",
    "claim",
    "source_set_id",
    "result_hash",
  ])
    expect(await publicResult.text()).not.toContain(privateField);
  expect(
    (
      await page.request.get(
        `http://localhost:5173/api/v1/b/aurora/draw-results/${result.id}`,
        { headers: { "X-Brand-ID": harbor } },
      )
    ).status(),
  ).toBe(404);

  const publicPage = await page.context().newPage();
  const publicApiRequests: { method: string; pathname: string }[] = [];
  publicPage.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/v1/")) {
      publicApiRequests.push({
        method: request.method(),
        pathname: url.pathname,
      });
    }
  });
  try {
    await publicPage.addInitScript(() =>
      localStorage.setItem("luma-language", "en"),
    );
    await publicPage.goto(
      `http://harbor.localhost:5173/results?game_id=${game.id}`,
    );
    const resultsPanel = publicPage.locator(
      ".draw-results:not(.draw-results--compact)",
    );
    const resultCard = resultsPanel
      .locator(".dr-card")
      .filter({ hasText: period.period_no });
    await expect(resultCard).toBeVisible();
    await expect(publicPage.locator(".page-footer")).toContainText(
      "Live public draw records",
    );
    await expect(resultCard.locator(".dr-digit-positions .dr-ball")).toHaveText(
      ["0", "1", "0"],
    );
    const currentResultRead = publicPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        response.url().endsWith(`/draw-results/${result.id}`),
    );
    await resultCard.locator("button.dr-card-main").click();
    const currentResultResponse = await currentResultRead;
    expect(
      currentResultResponse.status(),
      await currentResultResponse.text(),
    ).toBe(200);
    expect((await currentResultResponse.json()).data.item.id).toBe(result.id);
    const detailPanel = resultsPanel.locator(".dr-detail");
    await expect(detailPanel.locator(".dr-detail-id code")).toHaveText(
      result.id,
    );
    await expect(
      detailPanel.locator(".dr-digit-positions .dr-ball"),
    ).toHaveText(["0", "1", "0"]);
    await expect(resultsPanel).not.toContainText("SAMPLE");
    await resultsPanel
      .getByLabel("Exact period", { exact: true })
      .fill("unknown-exact-period");
    await resultsPanel
      .getByLabel("Exact period", { exact: true })
      .press("Enter");
    await expect(
      resultsPanel.getByText("No matching draw results.", { exact: true }),
    ).toBeVisible();
    await expect(resultsPanel.locator(".dr-card")).toHaveCount(0);
    await expect(detailPanel).toHaveCount(0);
    await resultsPanel
      .getByLabel("Exact period", { exact: true })
      .fill(period.period_no);
    await resultsPanel
      .getByLabel("Exact period", { exact: true })
      .press("Enter");
    await expect(resultCard).toBeVisible();

    // Retain the chosen numbers but mark the entire cancelled period explicitly.
    const cancellation = await page.request.post(
      `${base}/periods/${period.id}/cancel`,
      {
        headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
        data: {
          version: locked.version,
          mode: "judged_cancelled",
          cause: "invalid_result",
          reason: "public archive must retain invalidated-result warning",
        },
      },
    );
    expect(cancellation.status(), await cancellation.text()).toBe(200);
    expect((await cancellation.json()).data).toMatchObject({
      state: "completed",
      total_count: 0,
    });
    await resultsPanel
      .getByRole("button", { name: "Refresh", exact: true })
      .click();
    await expect(resultCard).toContainText("not a valid outcome or payout");
    await expect(resultCard.locator(".dr-digit-positions .dr-ball")).toHaveText(
      ["0", "1", "0"],
    );
    expect(
      await publicPage.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await publicPage.screenshot({
      path: info.outputPath("public-draw-cancelled.png"),
      fullPage: true,
    });

    // Actual persisted periods provide enough history to test the next page.
    // All these future draws have already-started betting windows; future-only
    // reservations are still excluded by the public API.
    const currentGame = await page.request.get(`${base}/games?limit=100`, {
      headers,
    });
    expect(currentGame.status(), await currentGame.text()).toBe(200);
    const currentVersion = (await currentGame.json()).data.games.find(
      (item: { id: string }) => item.id === game.id,
    ).version;
    const paginationSchedule = await page.request.put(
      `${base}/games/${game.id}/schedule`,
      {
        headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
        data: {
          version: currentVersion,
          reason: "real started-window archive pagination",
          spec: {
            timezone: "UTC",
            mode: "interval",
            daily_draw_times: [],
            interval_seconds: 60,
            busy_windows: [],
            bet_open_before_seconds: 7200,
            bet_close_before_seconds: 5,
            weekdays: [0, 1, 2, 3, 4, 5, 6],
            pause_dates: [],
            holiday_dates: [],
            holiday_policy: "normal",
          },
        },
      },
    );
    expect(paginationSchedule.status(), await paginationSchedule.text()).toBe(
      200,
    );
    const start = new Date(Date.now() + 60_000);
    const historyGeneration = await page.request.post(
      `${base}/games/${game.id}/periods/generate`,
      {
        headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
        data: {
          from: start.toISOString(),
          to: new Date(start.valueOf() + 60 * 60_000).toISOString(),
          reason: "reserve actual started periods for public history",
        },
      },
    );
    expect(historyGeneration.status(), await historyGeneration.text()).toBe(
      200,
    );
    expect((await historyGeneration.json()).data.created).toBeGreaterThan(50);
    await resultsPanel.getByLabel("Exact period", { exact: true }).fill("");
    await resultsPanel
      .getByLabel("Exact period", { exact: true })
      .press("Enter");
    await resultsPanel
      .getByRole("tab", { name: "History", exact: true })
      .click();
    await expect(resultsPanel.locator(".dr-history-card")).toHaveCount(50);
    const firstPagePeriods = await resultsPanel
      .locator(".dr-history-card .dr-period")
      .allTextContents();
    const nextHistory = publicPage.waitForResponse(
      (r) =>
        r.url().includes(`/games/${game.id}/periods?`) &&
        r.url().includes("offset=50"),
    );
    await resultsPanel
      .getByRole("button", { name: "Next", exact: true })
      .click();
    const nextResponse = await nextHistory;
    expect(nextResponse.status(), await nextResponse.text()).toBe(200);
    await expect(
      resultsPanel.locator(".dr-history-card").first(),
    ).toBeVisible();
    const secondPagePeriods = await resultsPanel
      .locator(".dr-history-card .dr-period")
      .allTextContents();
    expect(secondPagePeriods.length).toBeGreaterThan(0);
    expect(
      secondPagePeriods.every((number) => !firstPagePeriods.includes(number)),
    ).toBe(true);
    await expect(
      resultsPanel.getByRole("button", { name: "Previous", exact: true }),
    ).toBeEnabled();
    await expect(
      resultsPanel.getByRole("button", { name: "Next", exact: true }),
    ).toBeDisabled();
    expect(
      await publicPage.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await publicPage.screenshot({
      path: info.outputPath("public-period-history.png"),
      fullPage: true,
    });

    // A lost read must not retain numbers from the previous filter or page.
    await publicPage.route("**/api/v1/draw-results?**", (route) =>
      route.abort("failed"),
    );
    await resultsPanel
      .getByRole("tab", { name: "Results", exact: true })
      .click();
    await expect(
      resultsPanel
        .getByRole("alert")
        .filter({ hasText: "Network request failed" }),
    ).toBeVisible();
    await expect(resultsPanel.locator(".dr-card")).toHaveCount(0);
    await publicPage.unroute("**/api/v1/draw-results?**");
    await resultsPanel
      .getByRole("button", { name: "Retry", exact: true })
      .click();
    await expect(resultCard).toBeVisible();
    await publicPage.goto(`http://harbor.localhost:5173/games/${game.id}`);
    const compact = publicPage.locator(".draw-results--compact");
    await expect(compact).toContainText(period.period_no);
    await expect(compact.locator(".dr-digit-positions .dr-ball")).toHaveText([
      "0",
      "1",
      "0",
    ]);
    await expect(compact).toContainText("not a valid outcome or payout");
    expect(
      await publicPage.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await publicPage.screenshot({
      path: info.outputPath("public-draw-game-detail.png"),
      fullPage: true,
    });
    const resultReads = publicApiRequests.filter(
      ({ pathname }) =>
        /\/draw-results(?:\/[^/]+)?$/.test(pathname) ||
        /\/games\/[^/]+\/periods$/.test(pathname),
    );
    expect(resultReads.length).toBeGreaterThan(0);
    expect(resultReads.every(({ method }) => method === "GET")).toBe(true);
  } finally {
    await publicPage.close();
  }
});
