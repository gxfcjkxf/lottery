import { test, expect } from "@playwright/test";
import { rememberAdminSession, restoreAdminSession } from "./support/admin-session";

test("translated operational console preserves rule and report drafts without business writes", async ({ page, context }, info) => {
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD, "Provide an owned Harbor administrator");
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  const origin = process.env.TEST_ADMIN_ORIGIN ?? "http://localhost:5174", brand = "0199a000-0000-7000-8000-000000000002";
  const username = process.env.TEST_HARBOR_ADMIN_USERNAME!, headers = { Origin: origin, "X-Brand-ID": brand };
  if (!await restoreAdminSession(context, username, brand, origin)) {
    const response = await page.request.post(`${origin}/api/v1/admin/auth/login`, { headers: { ...headers, "Idempotency-Key": crypto.randomUUID() }, data: { identifier: username, password: process.env.TEST_HARBOR_ADMIN_PASSWORD } });
    expect(response.status()).toBe(200);
  }
  await rememberAdminSession(context, username, origin);
  const writes: string[] = [], errors: string[] = [];
  page.on("request", request => { if (new URL(request.url()).pathname.startsWith("/api/") && !["GET", "HEAD", "OPTIONS"].includes(request.method())) writes.push(`${request.method()} ${new URL(request.url()).pathname}`); });
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(origin);
  await page.getByTestId("admin-language").selectOption("zh-CN");
  await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(brand);
  const navigate = async (name: string) => {
    await page.getByTestId("admin-language").selectOption("zh-CN");
    if (info.project.name === "mobile") {
      if (name === "期次和开奖") { await page.locator(".mobile-nav button").nth(2).click(); return; }
      await page.locator(".mobile-nav button").nth(4).click();
      await page.locator(".mobile-more-menu").getByRole("button", { name: new RegExp(`${name}\\s*›$`) }).click();
    } else await page.locator(".side-nav").getByRole("button", { name: new RegExp(`${name}$`) }).click();
  };
  for (const item of [
    ["品牌和域名", ".brand-domains", "Brand domains"],
    ["代理树", ".agent-management", "Agent management"],
    ["加入码", ".join-codes", "Join code management"],
    ["期次和开奖", ".draw-management", "Draw management"],
    ["注单和异常", ".bet-orders", "Bet order management"],
    ["通知投递", ".notification-deliveries", "Notification deliveries"],
    ["通知模板", ".nt-page", "Notification templates"],
    ["风控与合规", ".compliance", "Risk and compliance"],
  ]) {
    await navigate(item[0]); const panel = page.locator(item[1]); await expect(panel).toBeVisible();
    await page.getByTestId("admin-language").selectOption("en");
    await expect(panel.getByRole("heading", { name: item[2], exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), item[0]).toBe(true);
  }
  await navigate("规则配置"); const simulator = page.locator(".rule-simulator");
  await simulator.getByLabel("特别号候选（最多 49）", { exact: true }).fill("7,19,31,43");
  const draft = await simulator.locator("input, select, textarea").evaluateAll(elements => elements.map(el => (el as HTMLInputElement).value));
  await page.getByTestId("admin-language").selectOption("en");
  await expect(simulator.getByRole("heading", { name: "Rule simulator", exact: true })).toBeVisible();
  const templateOptions = simulator.locator(".simulator-form select").first().locator("option");
  await expect(templateOptions).toHaveCount(6);
  const templateLabels = await templateOptions.allTextContents();
  expect(templateLabels).toHaveLength(6); expect(templateLabels.join(" ")).not.toMatch(/\p{Script=Han}/u);
  expect(templateLabels[0].trim()).toBe("Special-number match");
  await expect(simulator.getByLabel("Special-number candidates (up to 49)", { exact: true })).toHaveValue("7,19,31,43");
  expect(await simulator.locator("input, select, textarea").evaluateAll(elements => elements.map(el => (el as HTMLInputElement).value))).toEqual(draft);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await simulator.screenshot({ path: info.outputPath("english-rule-simulator.png") });
  await navigate("报表和对账"); const reports = page.locator(".reports-management");
  await reports.getByLabel("会员筛选 UUID", { exact: true }).fill("unsubmitted-customer-draft");
  await page.getByTestId("admin-language").selectOption("en");
  await expect(reports.getByRole("heading", { name: "Operational reports", exact: true })).toBeVisible();
  await expect(reports.getByLabel("Member UUID filter", { exact: true })).toHaveValue("unsubmitted-customer-draft");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await reports.screenshot({ path: info.outputPath("english-operational-reports.png") });
  expect(writes).toEqual([]); expect(errors).toEqual([]);
});
