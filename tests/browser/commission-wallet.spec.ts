import { expect, test, type APIResponse, type BrowserContext, type Page, type TestInfo } from "@playwright/test";
import { rememberAdminSession, restoreAdminSession } from "./support/admin-session";

const harbor = "0199a000-0000-7000-8000-000000000002";
const userOrigin = process.env.TEST_USER_ORIGIN ?? "http://localhost:5173";
const adminOrigin = process.env.TEST_ADMIN_ORIGIN ?? "http://localhost:5174";
const adminApi = `${adminOrigin}/api/v1/admin`;
const userUrl = new URL(userOrigin);
const platformUserHost = userUrl.hostname.replace(/^harbor\.localhost$/, "localhost");
const platformUserOrigin = `${userUrl.protocol}//${platformUserHost}${userUrl.port ? `:${userUrl.port}` : ""}`;
const harborUserHost = userUrl.hostname === "localhost" ? "harbor.localhost" : userUrl.hostname;
const harborUserOrigin = `${userUrl.protocol}//${harborUserHost}${userUrl.port ? `:${userUrl.port}` : ""}`;
const userApi = `${platformUserOrigin}/api/v1/b/harbor`;
const unique = () => crypto.randomUUID();
const headers = (origin: string) => ({ Origin: origin, "X-Brand-ID": harbor });

async function data<T>(response: APIResponse, label: string, status = 200): Promise<T> {
  const text = await response.text();
  expect(response.status(), label).toBe(status);
  const envelope = JSON.parse(text) as { success: boolean; data: T };
  expect(envelope.success, label).toBe(true);
  return envelope.data;
}

async function loginAdminHarbor(page: Page, context: BrowserContext) {
  const username = process.env.TEST_HARBOR_ADMIN_USERNAME!;
  const password = process.env.TEST_HARBOR_ADMIN_PASSWORD!;
  if (!await restoreAdminSession(context, username, harbor, adminOrigin)) {
    await data(await page.request.post(`${adminApi}/auth/login`, {
      headers: { ...headers(adminOrigin), "Idempotency-Key": unique() },
      data: { identifier: username, password },
    }), "Harbor admin login");
  }
  rememberAdminSession(username, await context.cookies(`${adminApi}/me`), adminOrigin);
  await page.goto(adminOrigin);
  await page.locator(".directory-brand-bar select").selectOption(harbor);
}

async function registerHarborMember(page: Page, context: BrowserContext) {
  const auth = await data<{ terms: { privacy_policy_version: string; service_terms_version: string }; auth?: { captcha_enabled?: boolean } }>(
    await page.request.get(`${userApi}/context`), "Harbor public context");
  let captcha: { captcha_id: string; captcha_answer: string } | Record<string, never> = {};
  if (auth.auth?.captcha_enabled) {
    const challenge = await data<{ id: string; svg: string }>(await page.request.get(`${userApi}/auth/challenge`), "Harbor registration CAPTCHA");
    captcha = { captcha_id: challenge.id, captcha_answer: challenge.svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? "" };
    expect(captcha.captcha_answer).toMatch(/^[A-Za-z0-9]+$/);
  }
  const username = `commission_${unique().replaceAll("-", "").slice(0, 14)}`;
  const password = `Commission-${unique()}-test`;
  const registration = await data<{ member: { id: string } }>(await page.request.post(`${userApi}/auth/register`, {
    headers: { Origin: platformUserOrigin, "Idempotency-Key": unique() },
    data: { username, password, ...auth.terms, ...captcha },
  }), "Harbor public member registration", 201);
  const cookieName = `lottery_user_${harbor.replaceAll("-", "")}`;
  const userCookies = (await context.cookies(userApi)).filter((cookie) => cookie.name === cookieName);
  expect(userCookies, "registration should issue the Harbor member cookie").toHaveLength(1);
  await context.addCookies(userCookies.map((cookie) => ({ ...cookie, domain: harborUserHost })));
  return registration.member.id;
}

async function adminFinance(page: Page, info: TestInfo, memberId: string) {
  if (info.project.name === "mobile") await page.locator(".mobile-nav button").nth(3).click();
  else await page.locator(".side-nav").getByRole("button", { name: /资金与账本|Funds.*ledger/ }).click();
  const panel = page.locator(".finance-management");
  await panel.getByLabel(/会员 UUID|Member UUID/, { exact: true }).fill(memberId);
  await panel.getByRole("button", { name: /查询会员|Load wallet/ }).click();
  await expect(panel.locator(".wallet-panel")).toBeVisible();
  return panel;
}

async function fits(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}

test("Harbor commission wallet source is manual, visible, frozen in debit order, and restored by its original entry", async ({ page, context }, info) => {
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD, "Provide isolated Harbor administrator credentials");
  test.setTimeout(90_000);
  page.setDefaultTimeout(12_000);
  await loginAdminHarbor(page, context);
  const memberId = await registerHarborMember(page, context);

  const adminCall = async <T>(path: string, method: "GET" | "POST", body?: unknown) => data<T>(await page.request.fetch(`${adminApi}${path}`, {
    method,
    headers: { ...headers(adminOrigin), ...(body === undefined ? {} : { "Content-Type": "application/json", "Idempotency-Key": unique() }) },
    ...(body === undefined ? {} : { data: body }),
  }), `${method} ${path}`);
  const adjust = async (source: "recharge" | "winning" | "gift" | "commission", points: string) => adminCall<Record<string, unknown>>(
    `/wallets/${memberId}/adjust`, "POST", { source, delta: points, reason: `synthetic manual wallet fixture (${source}); not automatic commission payout` });

  // These are explicit operator adjustments used only as test funding. No commission accrual or payout worker is exercised.
  await adjust("recharge", "2");
  await adjust("winning", "4");
  await adjust("gift", "3");
  await adjust("commission", "9");
  const before = await adminCall<{ available_points: string; display_points: string; by_source: Record<string, Record<string, string>> }>(
    `/wallets/${memberId}`, "GET");
  expect(before).toMatchObject({ available_points: "18", display_points: "18", by_source: {
    recharge: { available: "2" }, winning: { available: "4" }, gift: { available: "3" }, commission: { available: "9" },
  } });

  const finance = await adminFinance(page, info, memberId);
  await expect(finance.locator(".bucket-row")).toHaveCount(16);
  await expect(finance.locator(".bucket-row").filter({ hasText: /佣金|Commission/ }).first()).toContainText("9");
  await fits(page);

  const user = await context.newPage();
  await user.route(`${harborUserOrigin}/api/**`, async (route) => {
    const requestUrl = new URL(route.request().url());
    requestUrl.hostname = platformUserHost;
    const response = await route.fetch({
      url: requestUrl.toString(),
      headers: { ...(await route.request().allHeaders()), host: `${harborUserHost}${userUrl.port ? `:${userUrl.port}` : ""}` },
    });
    await route.fulfill({ response });
  });
  await user.goto(`${harborUserOrigin}/wallet`);
  const walletPanel = user.locator(".wallet-summary");
  await expect(walletPanel.locator(".bucket-source")).toHaveCount(4);
  const commissionBucket = walletPanel.locator(".bucket-source").filter({ has: user.getByRole("heading", { name: /佣金|Commission/ }) });
  await expect(commissionBucket).toBeVisible();
  await expect(commissionBucket.locator("dl div").filter({ hasText: /可用|Available/ }).locator("dd")).toHaveText("9");
  await expect(walletPanel).toContainText(/16 个积分桶|16 points buckets/);
  await fits(user);

  const ledgerBefore = await adminCall<{ items: Array<{ id: string; entry_type: string; reference_type: string; actor_type: string; reason: string }> }>(`/wallets/${memberId}/ledger?limit=100&offset=0`, "GET");
  expect(ledgerBefore.items).toHaveLength(4);
  expect(ledgerBefore.items.every((entry) => entry.entry_type === "adjust" && entry.reference_type === "manual" && entry.actor_type === "admin" && entry.reason.includes("synthetic manual wallet fixture"))).toBe(true);
  const freezeBody = { points: "18", reason: "Harbor four-source commission wallet integration freeze" };
  const freezeKey = unique();
  const firstFreeze = await data<{
    id: string; source_allocation: Array<{ source: string; state: string; points: string }>;
    before_snapshot: Record<string, Record<string, string>>; after_snapshot: Record<string, Record<string, string>>;
  }>(await page.request.post(`${adminApi}/wallets/${memberId}/freeze`, {
    headers: { ...headers(adminOrigin), "Content-Type": "application/json", "Idempotency-Key": freezeKey }, data: freezeBody,
  }), "Manual freeze");
  const replayedFreeze = await data<{ id: string }>(await page.request.post(`${adminApi}/wallets/${memberId}/freeze`, {
    headers: { ...headers(adminOrigin), "Content-Type": "application/json", "Idempotency-Key": freezeKey }, data: freezeBody,
  }), "Replay identical frozen request");
  expect(replayedFreeze.id).toBe(firstFreeze.id);
  expect(firstFreeze.source_allocation).toEqual([
    { source: "recharge", state: "available", points: "2" },
    { source: "winning", state: "available", points: "4" },
    { source: "commission", state: "available", points: "9" },
    { source: "gift", state: "available", points: "3" },
  ]);
  expect(Object.keys(firstFreeze.before_snapshot).sort()).toEqual(["commission", "gift", "recharge", "winning"]);
  expect(Object.keys(firstFreeze.after_snapshot).sort()).toEqual(["commission", "gift", "recharge", "winning"]);
  for (const snapshot of [firstFreeze.before_snapshot, firstFreeze.after_snapshot]) {
    expect(Object.values(snapshot)).toHaveLength(4);
    for (const buckets of Object.values(snapshot)) expect(Object.keys(buckets)).toHaveLength(4);
  }
  expect(firstFreeze.before_snapshot.commission?.available).toBe("9");
  expect(firstFreeze.after_snapshot.commission?.manual_frozen).toBe("9");
  const ledgerFrozen = await adminCall<{ items: Array<{ id: string }> }>(`/wallets/${memberId}/ledger?limit=100&offset=0`, "GET");
  expect(ledgerFrozen.items).toHaveLength(ledgerBefore.items.length + 1);
  expect(ledgerFrozen.items.filter((entry) => entry.id === firstFreeze.id)).toHaveLength(1);

  await walletPanel.getByRole("button", { name: /刷新|Refresh/ }).click();
  const userFreezeRow = walletPanel.locator(".ledger-entry").first();
  await expect(userFreezeRow.locator(".snapshot-bucket")).toHaveCount(16);
  await expect(userFreezeRow).toContainText(/佣金|commission/i);

  await data(await page.request.post(`${adminApi}/wallets/${memberId}/unfreeze`, {
    headers: { ...headers(adminOrigin), "Content-Type": "application/json", "Idempotency-Key": unique() },
    data: { entry_id: firstFreeze.id, reason: "Restore original manual freeze allocation" },
  }), "Unfreeze by original ledger entry");
  const restored = await adminCall<{ by_source: Record<string, Record<string, string>>; available_points: string; frozen_points: string }>(`/wallets/${memberId}`, "GET");
  expect(restored).toMatchObject({ available_points: "18", frozen_points: "0", by_source: {
    recharge: { available: "2", manual_frozen: "0" }, winning: { available: "4", manual_frozen: "0" },
    gift: { available: "3", manual_frozen: "0" }, commission: { available: "9", manual_frozen: "0" },
  } });
  await walletPanel.getByRole("button", { name: /刷新|Refresh/ }).click();
  await expect(walletPanel.locator(".bucket-source")).toHaveCount(4);
  await expect(walletPanel.locator(".bucket-source").filter({ has: user.getByRole("heading", { name: /佣金|Commission/ }) }).locator("dl div").filter({ hasText: /可用|Available/ }).locator("dd")).toHaveText("9");
  await fits(user);
  await finance.locator(".wallet-panel").screenshot({ path: info.outputPath("four-source-admin-wallet.png") });
  await walletPanel.locator(".bucket-grid").screenshot({ path: info.outputPath("four-source-user-wallet.png") });
});
