import { test, expect } from "@playwright/test";
const harbor = "0199a000-0000-7000-8000-000000000002";
test("six operator templates use the real rule engine without financial mutation", async ({
  page,
}, info) => {
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Provide isolated Harbor administrator credentials",
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
  if (info.project.name === "mobile") {
    await page.locator(".mobile-nav button").nth(4).click();
    await page
      .locator(".mobile-more-menu")
      .getByRole("button", { name: /规则配置/ })
      .click();
  } else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /规则配置/ })
      .click();
  const simulator = page.locator(".rule-simulator");
  await expect(simulator).toContainText("不会扣款、投注、发布或审核规则");
  async function run(combos: number, prize: string) {
    const reply = page.waitForResponse(
      (r) =>
        r.url().endsWith("/api/v1/admin/rule-simulations") &&
        r.request().method() === "POST",
    );
    await simulator
      .getByRole("button", { name: "运行真实规则模拟", exact: true })
      .click();
    const response = await reply;
    expect(response.status(), await response.text()).toBe(200);
    const data = (await response.json()).data;
    expect(data.combination_count).toBe(combos);
    expect(data.prize_points).toBe(prize);
    await expect(
      simulator.getByRole("heading", { name: "模拟命中", exact: true }),
    ).toBeVisible();
    return data;
  }
  await run(4, "35");
  await simulator
    .getByRole("combobox", { name: "玩法模板", exact: true })
    .selectOption("digits");
  await simulator.getByLabel(/^第 1 位候选/).fill("1");
  await simulator.getByLabel(/^第 2 位候选/).fill("2");
  await simulator.getByLabel(/^第 3 位候选/).fill("1");
  await simulator.getByLabel("三位开奖结果", { exact: true }).fill("1,2,1");
  await run(1, "35");
  await simulator
    .getByRole("combobox", { name: "玩法模板", exact: true })
    .selectOption("features");
  await simulator.getByLabel(/^三同号/).fill("1");
  await simulator.getByLabel(/^首尾同号/).fill("1");
  await simulator.getByLabel(/^奇数个数/).fill("3");
  await simulator.getByLabel(/^和值/).fill("3");
  await simulator.getByLabel("三位开奖结果", { exact: true }).fill("1,1,1");
  await run(1, "35");
  await simulator
    .getByRole("combobox", { name: "玩法模板", exact: true })
    .selectOption("exclude");
  await run(1, "35");
  await simulator
    .getByRole("combobox", { name: "玩法模板", exact: true })
    .selectOption("attributes");
  await run(2, "35");
  await simulator
    .getByRole("combobox", { name: "玩法模板", exact: true })
    .selectOption("m-select-n");
  await simulator.getByLabel("所选特别号", { exact: true }).fill("7");
  await run(1, "35");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("rule-simulator.png"),
    fullPage: true,
  });
});
