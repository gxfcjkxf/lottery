import { test, expect, type Page } from "@playwright/test";
const harbor = "0199a000-0000-7000-8000-000000000002";
async function login(page: Page, mobile: boolean) {
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Provide isolated Harbor administrator credentials",
  );
  await page.goto("http://localhost:5174");
  if (mobile) await page.locator(".mobile-nav button").nth(1).click();
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
  if (mobile) await page.locator(".mobile-nav button").nth(3).click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /资金与账本/ })
      .click();
}
test("brand point policy persists; pending recharge cancellation does not credit points", async ({
  page,
}, info) => {
  await login(page, info.project.name === "mobile");
  const created = await page.request.post(
    "http://localhost:5174/api/v1/admin/users",
    {
      headers: {
        Origin: "http://localhost:5174",
        "X-Brand-ID": harbor,
        "Idempotency-Key": crypto.randomUUID(),
      },
      data: {
        username: `cap_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`,
        password: "cap-test-only-password-2026",
        reason: "create cap browser fixture",
      },
    },
  );
  expect(created.status()).toBe(201);
  const member = (await created.json()).data.member_id;
  const policy = page.locator(".point-policy-settings");
  await expect(
    policy.getByRole("heading", { name: "当前策略", exact: true }),
  ).toBeVisible();
  await policy.getByLabel(/^余额上限/).fill("200");
  await policy.getByLabel(/^单笔充值上限/).fill("50");
  await policy.getByLabel(/^单次调整上限/).fill("30");
  await policy.getByLabel(/^变更原因/).fill("browser brand limits");
  await policy.getByRole("button", { name: /保存/ }).click();
  await expect(policy.getByRole("status")).toContainText("已保存");
  await policy.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(policy.getByLabel(/^单笔充值上限/)).toHaveValue("50");
  const finance = page.locator(".finance-management");
  await finance.getByLabel("会员 UUID", { exact: true }).fill(member);
  await finance.getByRole("button", { name: "查询会员", exact: true }).click();
  await expect(finance.locator(".wallet-panel")).toBeVisible();
  const form = finance
    .locator(".form-panel")
    .filter({ has: page.getByRole("heading", { name: /创建.*充值/ }) });
  await form.getByLabel(/充值积分/).fill("51");
  await form.getByLabel("原因", { exact: true }).fill("exceeds configured cap");
  await form.getByRole("button", { name: /创建待确认充值单/ }).click();
  await expect(finance.getByRole("alert")).toContainText(
    "超出当前品牌积分限额",
  );
  await form.getByLabel(/充值积分/).fill("40");
  await form
    .getByLabel("原因", { exact: true })
    .fill("pending order to cancel");
  await form.getByRole("button", { name: /创建待确认充值单/ }).click();
  const order = finance.locator(".recharge-row").first();
  await expect(order).toContainText("待确认");
  await order.getByLabel("取消原因", { exact: true }).fill("unverified proof");
  await order.getByRole("button", { name: "取消充值单", exact: true }).click();
  await expect(order).toContainText("已取消");
  await expect(
    finance.locator(".wallet-panel .total.prominent strong"),
  ).toHaveText("0");
  const repair = page.locator(".balance-repair");
  await repair.getByLabel("差错会员 UUID", { exact: true }).fill(member);
  await repair
    .getByRole("button", { name: "检查差错与修复记录", exact: true })
    .click();
  await expect(repair).toContainText("账本与余额一致，无需修复");
  await expect(
    repair.getByRole("button", { name: "确认按原账本修复", exact: true }),
  ).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("point-policy-and-cancel.png"),
    fullPage: true,
  });

  // Configuration alone must not authorize a user application or occupy points.
  const base = "http://localhost:5174/api/v1/admin";
  const headers = { Origin: "http://localhost:5174", "X-Brand-ID": harbor };
  const walletBefore = await page.request.get(`${base}/wallets/${member}`, {
    headers,
  });
  expect(walletBefore.status(), await walletBefore.text()).toBe(200);
  const originalWallet = (await walletBefore.json()).data;
  const createdGame = await page.request.post(`${base}/games`, {
    headers: { ...headers, "Idempotency-Key": crypto.randomUUID() },
    data: {
      code: `withdraw_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`,
      name: "Withdrawal rule browser fixture",
      timezone: "UTC",
      model: {
        model: "DIGITS_0_9",
        length: 3,
        ordered: true,
        allow_repeat: true,
      },
      reason: "isolated withdrawal configuration fixture",
    },
  });
  expect(createdGame.status(), await createdGame.text()).toBe(201);
  const game = (await createdGame.json()).data;
  const wp = page.locator(".withdrawal-policy");
  await expect(
    wp.getByLabel("品牌：默认流水倍数 N", { exact: true }),
  ).toHaveValue("1");
  await wp.getByLabel("品牌：启用提现配置", { exact: true }).check();
  await wp.getByLabel("品牌：最低提现积分", { exact: true }).fill("10");
  await wp
    .getByLabel("品牌：最高提现积分（留空表示无上限）", { exact: true })
    .fill("100");
  await wp.getByLabel("品牌：默认流水倍数 N", { exact: true }).fill("2.5");
  await wp.getByLabel("赠送", { exact: true }).uncheck();
  await wp
    .getByLabel(/^品牌：变更原因/)
    .fill("real withdrawal settings without eligibility execution");
  const writes: { key: string; body: string }[] = [];
  await page.route("**/api/v1/admin/withdrawal-policy", async (route) => {
    if (route.request().method() !== "PUT") return route.continue();
    writes.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postData()!,
    });
    if (writes.length === 1) {
      const committed = await route.fetch();
      expect(committed.status(), await committed.text()).toBe(200);
      await route.abort("failed");
    } else await route.continue();
  });
  await wp
    .getByRole("button", { name: "核对品牌配置并继续", exact: true })
    .click();
  const confirmation = wp.getByRole("region", {
    name: "请核对将要保存的配置",
    exact: true,
  });
  await expect(confirmation).toContainText('"turnover_multiple": "2.5"');
  await expect(
    wp.getByLabel("品牌：默认流水倍数 N", { exact: true }),
  ).toBeDisabled();
  await confirmation
    .getByRole("button", { name: "确认并提交冻结请求", exact: true })
    .click();
  await expect(
    wp.getByRole("alert").filter({ hasText: "此范围有写入结果待确认" }),
  ).toBeVisible();
  await wp.getByRole("button", { name: "只读核对", exact: true }).click();
  await expect(
    wp.getByRole("alert").filter({ hasText: "由于读取结果不含本次请求凭据" }),
  ).toBeVisible();
  await expect(
    wp.getByRole("button", { name: "使用原请求重试", exact: true }),
  ).toBeEnabled();
  await wp.getByRole("button", { name: "使用原请求重试", exact: true }).click();
  await expect(
    wp.getByRole("status").filter({ hasText: "配置已保存" }),
  ).toBeVisible();
  expect(writes).toHaveLength(2);
  expect(writes[0].key).toBe(writes[1].key);
  expect(writes[0].body).toBe(writes[1].body);
  const brandPolicy = await page.request.get(`${base}/withdrawal-policy`, {
    headers,
  });
  expect(brandPolicy.status(), await brandPolicy.text()).toBe(200);
  expect((await brandPolicy.json()).data).toMatchObject({
    version: 2,
    config: {
      enabled: true,
      min_points: "10",
      max_points: "100",
      allowed_sources: ["recharge", "winning"],
      review_mode: "manual",
      turnover_multiple: "2.5",
    },
  });
  await page.unroute("**/api/v1/admin/withdrawal-policy");
  await wp.getByLabel("彩种：直接输入 ID", { exact: true }).fill(game.id);
  await wp.getByRole("button", { name: "读取此彩种策略", exact: true }).click();
  const savedGame = wp.locator(".policy-card").filter({
    has: page.getByRole("heading", { name: "彩种流水倍数覆盖", exact: true }),
  });
  const effectiveN = savedGame
    .locator(".saved-summary div")
    .filter({ has: page.getByText("已保存有效 N", { exact: true }) })
    .locator("dd");
  await expect(effectiveN).toHaveText("2.5");
  await expect(wp.getByLabel("选择彩种", { exact: true })).toHaveValue(game.id);
  await wp.getByLabel("品牌：默认流水倍数 N", { exact: true }).fill("9");
  await expect(effectiveN).toHaveText("2.5"); // Not the unsaved brand draft.
  await wp.getByLabel("品牌：默认流水倍数 N", { exact: true }).fill("2.5");
  await wp.getByLabel(/^彩种：流水倍数 N/).fill("0");
  await wp
    .getByLabel(/^彩种：变更原因/)
    .fill("zero override configuration, not an eligibility decision");
  await wp
    .getByRole("button", { name: "核对彩种配置并继续", exact: true })
    .click();
  await confirmation
    .getByRole("button", { name: "确认并提交冻结请求", exact: true })
    .click();
  await expect(effectiveN).toHaveText("0");
  await expect(savedGame).toContainText("彩种覆盖");
  await wp.getByLabel("品牌：默认流水倍数 N", { exact: true }).fill("3.5");
  await wp
    .getByLabel(/^品牌：变更原因/)
    .fill("new default preserves the saved game override");
  await wp
    .getByRole("button", { name: "核对品牌配置并继续", exact: true })
    .click();
  await confirmation
    .getByRole("button", { name: "确认并提交冻结请求", exact: true })
    .click();
  await expect(
    wp.getByRole("status").filter({ hasText: "配置已保存" }),
  ).toBeVisible();
  await expect(effectiveN).toHaveText("0");
  await wp.getByLabel(/^彩种：流水倍数 N/).fill("");
  await wp
    .getByLabel(/^彩种：变更原因/)
    .fill("restore inheritance using the current persisted brand default");
  await wp
    .getByRole("button", { name: "核对彩种配置并继续", exact: true })
    .click();
  await confirmation
    .getByRole("button", { name: "确认并提交冻结请求", exact: true })
    .click();
  await expect(effectiveN).toHaveText("3.5");
  await expect(savedGame).toContainText("品牌默认");
  const brandHistory = await page.request.get(
    `${base}/withdrawal-policy/history`,
    { headers },
  );
  expect(brandHistory.status(), await brandHistory.text()).toBe(200);
  const brandVersions = (await brandHistory.json()).data.items;
  expect(
    brandVersions.map((entry: { version: number }) => entry.version),
  ).toEqual([3, 2, 1]);
  expect(brandVersions[1].config.turnover_multiple).toBe("2.5");
  expect(brandVersions[2].changed_by).toBe("");
  const gameHistory = await page.request.get(
    `${base}/games/${game.id}/withdrawal-policy/history`,
    { headers },
  );
  expect(gameHistory.status(), await gameHistory.text()).toBe(200);
  const gameVersions = (await gameHistory.json()).data.items;
  expect(
    gameVersions.map((entry: { version: number }) => entry.version),
  ).toEqual([3, 2, 1]);
  expect(gameVersions[0].config.turnover_multiple).toBeNull();
  expect(gameVersions[1].config.turnover_multiple).toBe("0");
  expect(gameVersions[1].changed_by).not.toBe("");
  await wp.getByRole("button", { name: "读取品牌历史", exact: true }).click();
  await expect(
    wp.getByText("real withdrawal settings without eligibility execution", {
      exact: false,
    }),
  ).toBeVisible();
  const walletAfter = await page.request.get(`${base}/wallets/${member}`, {
    headers,
  });
  expect(walletAfter.status(), await walletAfter.text()).toBe(200);
  expect((await walletAfter.json()).data).toEqual(originalWallet);
  const untouchedLedger = await page.request.get(
    `${base}/wallets/${member}/ledger`,
    { headers },
  );
  expect(untouchedLedger.status(), await untouchedLedger.text()).toBe(200);
  expect((await untouchedLedger.json()).data.items).toEqual([]);
  const noSubmission = await page.request.post(
    "http://localhost:5173/api/v1/b/harbor/withdrawals",
    {
      headers: {
        Origin: "http://localhost:5173",
        "Idempotency-Key": crypto.randomUUID(),
      },
      data: { points: "10", source_allocation: [{source:"recharge",state:"available",points:"10"}] },
    },
  );
  // The route now exists, but an administrator cookie is not a member session.
  expect(noSubmission.status()).toBe(401);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await wp.screenshot({
    path: info.outputPath("withdrawal-policy-history.png"),
  });
});
test("repair UI submits reviewed token only and shows immutable evidence (mocked repair response)", async ({
  page,
}, info) => {
  const member = "0199a999-0000-7000-8000-000000000001";
  const buckets = {
    recharge: {
      available: "100",
      manual_frozen: "0",
      system_frozen: "0",
      withdrawal: "0",
    },
    winning: {
      available: "0",
      manual_frozen: "0",
      system_frozen: "0",
      withdrawal: "0",
    },
    gift: {
      available: "0",
      manual_frozen: "0",
      system_frozen: "0",
      withdrawal: "0",
    },
  };
  let repaired = false;
  let submitted: unknown;
  await page.route(
    `**/api/v1/admin/wallets/${member}/repair-preview`,
    async (route) =>
      route.fulfill({
        json: {
          success: true,
          data: {
            account_id: "fixture-account",
            member_id: member,
            version: 1,
            ledger_version: 1,
            actual: {
              ...buckets,
              recharge: {
                ...buckets.recharge,
                available: repaired ? "100" : "999",
              },
            },
            expected: buckets,
            repairable: !repaired,
            consistent: repaired,
            issues: repaired
              ? []
              : ["materialized balance differs from ledger"],
            token: repaired ? "new-token" : "reviewed-token",
          },
        },
      }),
  );
  await page.route(`**/api/v1/admin/wallets/${member}/repairs`, async (route) =>
    route.fulfill({
      json: {
        success: true,
        data: {
          items: repaired
            ? [
                {
                  id: "mock-repair-record",
                  version: 1,
                  before_snapshot: { balance: "999" },
                  after_snapshot: { balance: "100" },
                  reason: "reviewed projection repair",
                  actor_id: "fixture-admin",
                  request_id: "fixture-request",
                  created_at: "2026-10-06T00:00:00Z",
                },
              ]
            : [],
        },
      },
    }),
  );
  await page.route(
    `**/api/v1/admin/wallets/${member}/repair`,
    async (route) => {
      submitted = route.request().postDataJSON();
      expect(route.request().headers()["idempotency-key"]).toBeTruthy();
      repaired = true;
      await route.fulfill({
        json: {
          success: true,
          data: { id: "mock-repair-record", audit_log_id: "mock-audit" },
        },
      });
    },
  );
  await login(page, info.project.name === "mobile");
  const panel = page.locator(".balance-repair");
  await panel.getByLabel("差错会员 UUID", { exact: true }).fill(member);
  await panel
    .getByRole("button", { name: "检查差错与修复记录", exact: true })
    .click();
  await expect(panel).toContainText("账本完整，可以重建余额");
  const apply = panel.getByRole("button", {
    name: "确认按原账本修复",
    exact: true,
  });
  await expect(apply).toBeDisabled();
  await panel
    .getByLabel("余额修复原因", { exact: true })
    .fill("reviewed projection repair");
  await panel.getByRole("checkbox").check();
  await apply.click();
  await expect(panel.getByRole("status")).toContainText("余额重建已提交");
  await expect(panel).toContainText("账本与余额一致，无需修复");
  expect(submitted).toEqual({
    version: 1,
    token: "reviewed-token",
    reason: "reviewed projection repair",
  });
  await panel.locator("details summary").click();
  await expect(panel.locator("details")).toContainText("fixture-request");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
});
