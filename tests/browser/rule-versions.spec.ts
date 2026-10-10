import { test, expect, type Page, type TestInfo } from "@playwright/test";
import { rememberAdminSession, restoreAdminSession } from "./support/admin-session";

const harbor = "0199a000-0000-7000-8000-000000000002";
async function login(
  page: Page,
  info: TestInfo,
  username: string,
  password: string,
) {
  const restored = await restoreAdminSession(page.context(), username, harbor);
  await page.goto("http://localhost:5174");
  if (!restored) {
  await page.getByLabel("账号", { exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  }
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  await rememberAdminSession(page.context(), username);
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
test("persisted rules validate and allow creator review with audited immediate and next-period approval", async ({
  page,
  browser,
}, info) => {
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Provide an isolated brand administrator with creation and review permissions",
  );
  const code = `book_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`;
  await login(
    page,
    info,
    process.env.TEST_HARBOR_ADMIN_USERNAME!,
    process.env.TEST_HARBOR_ADMIN_PASSWORD!,
  );
  const panel = page.locator(".rule-versions");
  await expect(
    panel.getByRole("heading", { name: "彩种、玩法与规则审批", exact: true }),
  ).toBeVisible();
  const game = panel.locator("details").filter({
    has: page.getByText("创建彩种（模板或自定义号码模型）", { exact: true }),
  });
  await game.locator("summary").click();
  await game.getByLabel("代码", { exact: true }).fill(code);
  await game.getByLabel("名称", { exact: true }).fill("Browser special game");
  await game
    .getByLabel("创建原因", { exact: true })
    .fill("browser catalog setup");
  const gameReply = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/v1/admin/games") &&
      r.request().method() === "POST",
  );
  await game.getByRole("button", { name: "创建彩种", exact: true }).click();
  const gameResponse = await gameReply;
  expect(gameResponse.status(), await gameResponse.text()).toBe(201);
  const gameId = (await gameResponse.json()).data.id;
  const play = panel
    .locator("details")
    .filter({ has: page.getByText("创建玩法", { exact: true }) });
  await play.locator("summary").click();
  await play.getByLabel("彩种 ID", { exact: true }).fill(gameId);
  await play.getByLabel("代码", { exact: true }).fill("special");
  await play.getByLabel("名称", { exact: true }).fill("Special candidate");
  await play.getByLabel("创建原因", { exact: true }).fill("browser play setup");
  const playReply = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/games/${gameId}/plays`) &&
      r.request().method() === "POST",
  );
  await play.getByRole("button", { name: "创建玩法", exact: true }).click();
  const playResponse = await playReply;
  expect(playResponse.status(), await playResponse.text()).toBe(201);
  const playId = (await playResponse.json()).data.id;
  await panel.getByLabel("直接指定玩法 ID", { exact: true }).fill(playId);
  await panel
    .getByRole("button", { name: "选择玩法并读取历史", exact: true })
    .click();
  await panel
    .getByRole("button", { name: "新建规则草稿", exact: true })
    .click();
  const editor = panel.locator(".editor");
  await editor
    .getByRole("combobox", { name: "生效方式", exact: true })
    .selectOption("immediate");
  await editor
    .getByLabel("保存原因", { exact: true })
    .fill("browser draft specification");
  const createReply = page.waitForResponse(
    (r) =>
      r.url().endsWith("/api/v1/admin/rule-versions") &&
      r.request().method() === "POST",
  );
  await editor.getByRole("button", { name: "保存新草稿", exact: true }).click();
  const createResponse = await createReply;
  expect(createResponse.status(), await createResponse.text()).toBe(201);
  const ruleId = (await createResponse.json()).data.id;
  await editor
    .getByLabel("用例名称", { exact: true })
    .fill("special four candidates");
  await editor.getByLabel("预期投注积分", { exact: true }).fill("4");
  await editor.getByLabel("预期中奖积分", { exact: true }).fill("35");
  await editor
    .getByRole("combobox", { name: "预期是否中奖", exact: true })
    .selectOption({ label: "中奖" });
  await editor
    .getByLabel("验证原因", { exact: true })
    .fill("independently checked sample");
  const validationReply = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/rule-versions/${ruleId}/validate`) &&
      r.request().method() === "POST",
  );
  await editor
    .getByRole("button", { name: "验证已保存草稿并保存报告", exact: true })
    .click();
  const validation = await validationReply;
  expect(validation.status(), await validation.text()).toBe(200);
  expect((await validation.json()).data.validation.passed).toBe(true);
  await expect(
    panel.getByRole("heading", { name: "服务端验证：通过", exact: true }),
  ).toBeVisible();
  await editor
    .getByLabel("提交审核原因", { exact: true })
    .fill("ready for single-administrator review");
  const submitReply = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/rule-versions/${ruleId}/submit-review`) &&
      r.request().method() === "POST",
  );
  await editor
    .getByRole("button", { name: "提交审核（需服务端验证通过）", exact: true })
    .click();
  expect((await submitReply).status()).toBe(200);
  await expect(panel.locator(".review")).not.toContainText("不能批准或拒绝自己的版本");
  await expect(
    panel.getByRole("button", { name: "批准该版本", exact: true }),
  ).toBeVisible();
  await page.reload();
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
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(harbor);
  await panel.getByLabel("直接指定玩法 ID", { exact: true }).fill(playId);
  await panel
    .getByRole("button", { name: "选择玩法并读取历史", exact: true })
    .click();
  await panel
    .locator(".version-list")
    .getByRole("button", { name: /第 1 版/ })
    .click();
  await expect(panel.getByRole("button", { name: "批准该版本", exact: true })).toBeVisible();
  const reviewContext = await browser.newContext({
    viewport: info.project.use.viewport,
    isMobile: info.project.name === "mobile",
    hasTouch: info.project.name === "mobile",
  });
  try {
    const reviewer = await reviewContext.newPage();
    await login(
      reviewer,
      info,
      process.env.TEST_HARBOR_ADMIN_USERNAME!,
      process.env.TEST_HARBOR_ADMIN_PASSWORD!,
    );
    const reviewPanel = reviewer.locator(".rule-versions");
    await reviewPanel
      .getByLabel("直接指定玩法 ID", { exact: true })
      .fill(playId);
    await reviewPanel
      .getByRole("button", { name: "选择玩法并读取历史", exact: true })
      .click();
    await reviewPanel
      .locator(".version-list")
      .getByRole("button", { name: /第 1 版/ })
      .click();
    const card = reviewPanel.locator(".review");
    await card
      .getByLabel("审核原因（批准和拒绝均必填）", { exact: true })
      .fill("reviewed cases, odds and scope");
    await card.getByRole("checkbox").check();
    const approveReply = reviewer.waitForResponse(
      (r) =>
        r.url().endsWith(`/rule-versions/${ruleId}/approve`) &&
        r.request().method() === "POST",
    );
    await card.getByRole("button", { name: "批准该版本", exact: true }).click();
    const approved = await approveReply;
    expect(approved.status(), await approved.text()).toBe(200);
    expect((await approved.json()).data.status).toBe("active");
    await expect(reviewPanel.locator(".history")).toContainText("生效中");
    await expect(reviewPanel.locator(".editor")).toHaveCount(0);
    expect(
      await reviewer.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    ).toBe(true);
    await reviewer.screenshot({
      path: info.outputPath("rule-review.png"),
      fullPage: true,
    });
    // A second approved version must remain queued, not replace the active one.
    await panel
      .getByRole("button", { name: "刷新版本历史", exact: true })
      .click();
    await panel
      .getByRole("button", { name: "新建规则草稿", exact: true })
      .click();
    await editor
      .getByRole("combobox", { name: "生效方式", exact: true })
      .selectOption("next_period");
    await editor
      .getByLabel("保存原因", { exact: true })
      .fill("new rule only for the next betting period");
    const nextCreate = page.waitForResponse(
      (r) =>
        r.url().endsWith("/api/v1/admin/rule-versions") &&
        r.request().method() === "POST",
    );
    await editor
      .getByRole("button", { name: "保存新草稿", exact: true })
      .click();
    const nextResponse = await nextCreate;
    expect(nextResponse.status(), await nextResponse.text()).toBe(201);
    const nextId = (await nextResponse.json()).data.id;
    await editor
      .getByLabel("用例名称", { exact: true })
      .fill("next period special");
    await editor.getByLabel("预期投注积分", { exact: true }).fill("4");
    await editor.getByLabel("预期中奖积分", { exact: true }).fill("35");
    await editor
      .getByRole("combobox", { name: "预期是否中奖", exact: true })
      .selectOption({ label: "中奖" });
    await editor
      .getByLabel("验证原因", { exact: true })
      .fill("independent expected values");
    const nextValidate = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/rule-versions/${nextId}/validate`) &&
        r.request().method() === "POST",
    );
    await editor
      .getByRole("button", { name: "验证已保存草稿并保存报告", exact: true })
      .click();
    expect((await nextValidate).status()).toBe(200);
    await editor
      .getByLabel("提交审核原因", { exact: true })
      .fill("queue for next period");
    const nextSubmit = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/rule-versions/${nextId}/submit-review`) &&
        r.request().method() === "POST",
    );
    await editor
      .getByRole("button", {
        name: "提交审核（需服务端验证通过）",
        exact: true,
      })
      .click();
    expect((await nextSubmit).status()).toBe(200);
    await reviewPanel
      .getByRole("button", { name: "刷新版本历史", exact: true })
      .click();
    await reviewPanel
      .locator(".version-list")
      .getByRole("button", { name: /第 2 版/ })
      .click();
    await card
      .getByLabel("审核原因（批准和拒绝均必填）", { exact: true })
      .fill("reviewed next-period scope");
    await card.getByRole("checkbox").check();
    const queuedReply = reviewer.waitForResponse(
      (r) =>
        r.url().endsWith(`/rule-versions/${nextId}/approve`) &&
        r.request().method() === "POST",
    );
    await card.getByRole("button", { name: "批准该版本", exact: true }).click();
    const queued = await queuedReply;
    expect(queued.status(), await queued.text()).toBe(200);
    const queuedVersion = (await queued.json()).data;
    expect(queuedVersion.status).toBe("approved");
    expect(queuedVersion.effective_sequence).toBe(1);
    expect(queuedVersion.effective_at).toBeUndefined();
    const currentPlays = await reviewer.request.get(
      `http://localhost:5174/api/v1/admin/games/${gameId}/plays`,
      { headers: { "X-Brand-ID": harbor } },
    );
    expect(currentPlays.status()).toBe(200);
    expect(
      (await currentPlays.json()).data.plays.find(
        (p: { id: string }) => p.id === playId,
      ).active_version_id,
    ).toBe(ruleId);
    await expect(reviewPanel.locator(".history")).toContainText("已批准");
    await reviewer.screenshot({
      path: info.outputPath("rule-review-next-period.png"),
      fullPage: true,
    });
  } finally {
    await reviewContext.close();
  }
});
