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
