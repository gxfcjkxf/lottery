import { test, expect, type Page, type TestInfo } from "@playwright/test";
const harbor = "0199a000-0000-7000-8000-000000000002";
async function login(
  page: Page,
  info: TestInfo,
  username: string,
  password: string,
) {
  await page.goto("http://localhost:5174");
  await page.getByLabel("账号", { exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  if (info.project.name === "mobile") {
    await page.locator(".mobile-nav button").last().click();
    await page
      .locator(".mobile-more-menu")
      .getByRole("button", { name: /规则配置/ })
      .click();
  } else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /规则配置/ })
      .click();
}
test("custom multi-tier rules retain creator review controls and allow another authorized reviewer", async ({
  page,
  browser,
}, info) => {
  test.setTimeout(90_000);
  page.setDefaultTimeout(10_000);
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD ||
      !process.env.TEST_RULE_REVIEWER_USERNAME ||
      !process.env.TEST_RULE_REVIEWER_PASSWORD,
    "Requires isolated creator/reviewer credentials",
  );
  await login(
    page,
    info,
    process.env.TEST_HARBOR_ADMIN_USERNAME!,
    process.env.TEST_HARBOR_ADMIN_PASSWORD!,
  );
  const panel = page.locator(".rule-versions");
  const game = panel
    .locator("details")
    .filter({
      has: page.getByText("创建彩种（模板或自定义号码模型）", { exact: true }),
    });
  await game.locator("summary").click();
  await game
    .getByLabel("代码", { exact: true })
    .fill(`visual_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`);
  await game.getByLabel("名称", { exact: true }).fill("Visual four digit game");
  await game.getByLabel(/自定义彩种号码模型（/).check();
  await game
    .locator(".model-editor")
    .getByRole("combobox", { name: "号码模型", exact: true })
    .selectOption("DIGITS_0_9");
  await game.getByLabel(/数字位数 N/).fill("4");
  await game
    .getByLabel("创建原因", { exact: true })
    .fill("isolated custom four-position model");
  const gameReply = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/v1/admin/games") &&
      r.request().method() === "POST",
  );
  await game.getByRole("button", { name: "创建彩种", exact: true }).click();
  const gameResponse = await gameReply;
  expect(gameResponse.status(), await gameResponse.text()).toBe(201);
  const gameData = (await gameResponse.json()).data;
  expect(gameData.model.length).toBe(4);
  const play = panel
    .locator("details")
    .filter({ has: page.getByText("创建玩法", { exact: true }) });
  await play.locator("summary").click();
  await play.getByLabel("彩种 ID", { exact: true }).fill(gameData.id);
  await play.getByLabel("代码", { exact: true }).fill("complex");
  await play.getByLabel("名称", { exact: true }).fill("Exact plus bonus");
  await play.getByLabel("创建原因", { exact: true }).fill("visual custom play");
  const playReply = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/games/${gameData.id}/plays`) &&
      r.request().method() === "POST",
  );
  await play.getByRole("button", { name: "创建玩法", exact: true }).click();
  const playResponse = await playReply;
  expect(playResponse.status(), await playResponse.text()).toBe(201);
  const playData = (await playResponse.json()).data;
  await panel.getByLabel("直接指定玩法 ID", { exact: true }).fill(playData.id);
  await panel
    .getByRole("button", { name: "选择玩法并读取历史", exact: true })
    .click();
  await panel
    .getByRole("button", { name: "新建规则草稿", exact: true })
    .click();
  const editor = panel.locator(".editor");
  await expect(editor.locator(".definition-editor")).toBeVisible();
  await editor.getByLabel("单位积分", { exact: true }).fill("2");
  await editor.getByLabel("最大倍投", { exact: true }).fill("9007199254740993");
  const main = editor.locator(".tier-editor").first();
  await main.getByLabel("奖级赔率（最多六位小数）", { exact: true }).fill("5");
  await main
    .getByLabel("条件运算", { exact: true })
    .first()
    .selectOption("all");
  await main
    .locator(".condition-node")
    .nth(1)
    .getByLabel("比较值（0–20000000）", { exact: true })
    .fill("4");
  await main
    .getByRole("button", { name: "新增子条件", exact: true })
    .first()
    .click();
  await main
    .locator(".condition-node")
    .nth(2)
    .getByLabel("条件运算", { exact: true })
    .first()
    .selectOption("not");
  const negated = main.locator(".condition-node").nth(3);
  await negated
    .getByLabel("判断字段", { exact: true })
    .selectOption("draw_all_same");
  await negated
    .getByLabel("开奖结果部分", { exact: true })
    .selectOption("digits");
  await negated.getByLabel("比较值（0–20000000）", { exact: true }).fill("1");
  await editor.getByRole("button", { name: "新增奖级", exact: true }).click();
  const bonus = editor.locator(".tier-editor").nth(1);
  await bonus.getByLabel("奖级代码", { exact: true }).fill("BONUS");
  await bonus.getByLabel("奖级赔率（最多六位小数）", { exact: true }).fill("2");
  await bonus.getByLabel(/排他奖级/).uncheck();
  await bonus
    .getByLabel("条件运算", { exact: true })
    .first()
    .selectOption("equals");
  await bonus
    .getByLabel("判断字段", { exact: true })
    .selectOption("draw_odd_count");
  await bonus
    .getByLabel("开奖结果部分", { exact: true })
    .selectOption("digits");
  await bonus.getByLabel("比较值（0–20000000）", { exact: true }).fill("2");
  await editor
    .getByRole("combobox", { name: "混合排他/累加策略", exact: true })
    .selectOption("max_exclusive_plus_additive");
  await editor
    .getByLabel("保存原因", { exact: true })
    .fill("independent visual conditions and two tiers");
  const draftReply = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/v1/admin/rule-versions") &&
      r.request().method() === "POST",
  );
  await editor.getByRole("button", { name: "保存新草稿", exact: true }).click();
  const draftResponse = await draftReply;
  expect(draftResponse.status(), await draftResponse.text()).toBe(201);
  const record = (await draftResponse.json()).data;
  expect(record.definition.prize_tiers).toHaveLength(2);
  expect(record.definition.limits.max_multiplier).toBe("9007199254740993");
  expect(record.definition.prize_tiers[0].condition.children[1].op).toBe("not");
  const cases = editor.locator(".rule-cases-editor");
  const first = cases.locator(".case-card").first();
  const validate = editor.getByRole("button", {
    name: "验证已保存草稿并保存报告",
    exact: true,
  });
  // Adding/copying/removing rows must not resurrect a stale valid CSV value.
  await first.getByLabel(/第 1 位候选数字/).fill("bad");
  await cases.getByRole("button", { name: /新增用例/ }).click();
  await expect(validate).toBeDisabled();
  await cases
    .locator(".case-card")
    .nth(1)
    .getByRole("button", { name: "删除用例", exact: true })
    .click();
  await expect(validate).toBeDisabled();
  await first.getByRole("button", { name: "复制用例", exact: true }).click();
  await expect(validate).toBeDisabled();
  await cases
    .locator(".case-card")
    .nth(1)
    .getByRole("button", { name: "删除用例", exact: true })
    .click();
  await first.getByLabel(/第 1 位候选数字/).fill("1");
  const fillCase = async (
    index: number,
    name: string,
    draw: string,
    prize: string,
    won: boolean,
  ) => {
    const card = cases.locator(".case-card").nth(index);
    await card.getByLabel("用例名称", { exact: true }).fill(name);
    for (const [i, n] of [1, 2, 1, 0].entries())
      await card
        .getByLabel(new RegExp(`第 ${i + 1} 位候选数字`))
        .fill(String(n));
    await card.getByLabel(/开奖数字/).fill(draw);
    await card.getByLabel("预期投注积分", { exact: true }).fill("2");
    await card.getByLabel("预期中奖积分", { exact: true }).fill(prize);
    await card.getByLabel("预期中奖", { exact: true }).setChecked(won);
  };
  await fillCase(0, "exact_and_bonus", "1,2,1,0", "14", true);
  await cases.getByRole("button", { name: /新增用例/ }).click();
  await fillCase(1, "loss", "2,2,2,2", "0", false);
  await editor
    .getByLabel("验证原因", { exact: true })
    .fill("winning and losing independent expectations");
  const validationReply = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/rule-versions/${record.id}/validate`) &&
      r.request().method() === "POST",
  );
  await validate.click();
  const validationResponse = await validationReply;
  expect(validationResponse.status(), await validationResponse.text()).toBe(
    200,
  );
  const verified = (await validationResponse.json()).data;
  expect(verified.validation.passed).toBe(true);
  expect(verified.validation.cases).toHaveLength(2);
  expect(verified.validation.cases[0].actual_prize_points).toBe("14");
  await editor
    .getByLabel("提交审核原因", { exact: true })
    .fill("validated custom rule ready for brand review");
  const submitReply = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/rule-versions/${record.id}/submit-review`) &&
      r.request().method() === "POST",
  );
  await editor
    .getByRole("button", { name: "提交审核（需服务端验证通过）", exact: true })
    .click();
  const submitted = await submitReply;
  expect(submitted.status(), await submitted.text()).toBe(200);
  await expect(panel.locator(".review").getByRole("button", { name: "批准该版本", exact: true })).toBeVisible();
  const ctx = await browser.newContext({
    viewport: info.project.use.viewport,
    isMobile: info.project.name === "mobile",
    hasTouch: info.project.name === "mobile",
  });
  try {
    const reviewer = await ctx.newPage();
    reviewer.setDefaultTimeout(10_000);
    await login(
      reviewer,
      info,
      process.env.TEST_RULE_REVIEWER_USERNAME!,
      process.env.TEST_RULE_REVIEWER_PASSWORD!,
    );
    const rp = reviewer.locator(".rule-versions");
    await rp.getByLabel("直接指定玩法 ID", { exact: true }).fill(playData.id);
    await rp
      .getByRole("button", { name: "选择玩法并读取历史", exact: true })
      .click();
    const review = rp.locator(".review");
    await review
      .getByLabel("审核原因（批准和拒绝均必填）", { exact: true })
      .fill("reviewed nested predicates, mixed tiers and two cases");
    await review.getByRole("checkbox").check();
    const approvalReply = reviewer.waitForResponse(
      (r) =>
        r.url().endsWith(`/rule-versions/${record.id}/approve`) &&
        r.request().method() === "POST",
    );
    await review
      .getByRole("button", { name: "批准该版本", exact: true })
      .click();
    const approval = await approvalReply;
    expect(approval.status(), await approval.text()).toBe(200);
    expect((await approval.json()).data.status).toBe("active");
    await rp.locator(".saved-rule-summary summary").click();
    await expect(
      rp
        .locator(".saved-rule-summary")
        .getByLabel("奖级赔率（最多六位小数）")
        .first(),
    ).toHaveValue("5");
    expect(
      await reviewer.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    await reviewer.screenshot({
      path: info.outputPath("custom-rule-editor.png"),
      fullPage: true,
    });
  } finally {
    await ctx.close();
  }
});
