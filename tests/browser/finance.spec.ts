import { test, expect } from "@playwright/test";

test("manual recharge, freeze, original-source unfreeze and adjustment reconcile with user wallet", async ({
  page,
  context,
}, info) => {
  test.skip(
    !process.env.TEST_ADMIN_USERNAME || !process.env.TEST_ADMIN_PASSWORD,
    "Provide isolated test administrator credentials",
  );
  const username = `cash_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`;
  const registered = await page.request.post(
    "http://localhost:5173/api/v1/auth/register",
    {
      headers: {
        Origin: "http://localhost:5173",
        "Idempotency-Key": crypto.randomUUID(),
      },
      data: {
        username,
        password: "cash-test-only-password-2026",
        privacy_policy_version: "dev-1",
        service_terms_version: "dev-1",
      },
    },
  );
  expect(registered.status()).toBe(201);
  const registration = (await registered.json()).data;
  const member = registration.member.id;
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
    .fill(process.env.TEST_ADMIN_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.TEST_ADMIN_PASSWORD!);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption("0199a000-0000-7000-8000-000000000001");
  if (info.project.name === "mobile")
    await page.locator(".mobile-nav button").nth(3).click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /资金与账本/ })
      .click();
  const finance = page.locator(".finance-management");
  await finance.getByLabel("会员 UUID", { exact: true }).fill(member);
  await finance.getByRole("button", { name: "查询会员", exact: true }).click();
  await expect(finance.locator(".wallet-panel")).toBeVisible();
  const recharge = finance
    .locator(".form-panel")
    .filter({ has: page.getByRole("heading", { name: /创建.*充值/ }) });
  await recharge.getByLabel(/充值积分/).fill("100");
  await recharge
    .getByLabel("原因", { exact: true })
    .fill("browser recharge create");
  await recharge.getByRole("button", { name: /创建待确认充值单/ }).click();
  await expect(finance.getByRole("status")).toContainText("充值单已创建");
  const order = finance.locator(".recharge-row").first();
  await expect(order).toContainText("待确认");
  await expect(
    finance.locator(".wallet-panel .total.prominent strong"),
  ).toHaveText("0");
  await order
    .getByLabel("确认原因", { exact: true })
    .fill("browser recharge confirm");
  await order.getByRole("button", { name: "确认并入账", exact: true }).click();
  await expect(order).toContainText("已确认");
  await expect(
    finance.locator(".wallet-panel .total.prominent strong"),
  ).toHaveText("100");
  const freeze = finance
    .locator("form")
    .filter({
      has: page.getByRole("heading", { name: "人工冻结", exact: true }),
    });
  await freeze.getByLabel(/冻结积分（正整数）/).fill("60");
  await freeze.getByLabel("原因", { exact: true }).fill("browser freeze test");
  await freeze.getByRole("button", { name: "冻结积分", exact: true }).click();
  await expect(finance.getByRole("status")).toContainText("人工冻结请求已成功");
  await expect(
    finance.locator(".wallet-panel .total.prominent strong"),
  ).toHaveText("100");
  const unfreeze = finance.locator(".unfreeze-row").first();
  await unfreeze
    .getByLabel("整笔原路解冻原因", { exact: true })
    .fill("browser unfreeze test");
  await unfreeze
    .getByRole("button", { name: "整笔原路解冻", exact: true })
    .click();
  await expect(finance.getByRole("status")).toContainText(
    "整笔原路解冻请求已成功",
  );
  await expect(finance.locator(".unfreeze-row")).toHaveCount(0);
  const adjust = finance
    .locator("form")
    .filter({
      has: page.getByRole("heading", { name: "来源积分调整", exact: true }),
    });
  await adjust.getByRole("combobox", { name: "积分来源", exact: true }).selectOption("gift");
  await adjust.getByLabel(/调整额/).fill("25");
  await adjust
    .getByLabel("原因", { exact: true })
    .fill("browser gift adjustment");
  await adjust
    .getByRole("button", { name: "提交来源调整", exact: true })
    .click();
  await expect(finance.getByRole("status")).toContainText("积分调整请求已成功");
  await expect(
    finance.locator(".wallet-panel .total.prominent strong"),
  ).toHaveText("125");
  await expect(finance.locator(".status.good")).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("finance-management.png"),
    fullPage: true,
  });
  const user = await context.newPage();
  await user.goto("http://localhost:5173/wallet");
  await expect(
    user.locator(".wallet-summary .total-primary strong"),
  ).toHaveText("125");
  await expect(user.locator(".wallet-summary .ledger-entry")).toHaveCount(4);
  await expect(user.locator(".wallet-summary")).toContainText(
    "browser gift adjustment",
  );
  await expect(user.locator(".wallet-summary")).toContainText("Before");
  expect(
    await user.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await user.screenshot({
    path: info.outputPath("wallet-summary.png"),
    fullPage: true,
  });
});
