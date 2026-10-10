import { expect, test, type Route } from "@playwright/test";

const auroraOrigin = "http://localhost:5173";
const harborOrigin = "http://harbor.localhost:5173";
const adminOrigin = "http://localhost:5174";
const auroraBrand = "0199a000-0000-7000-8000-000000000001";
const harborBrand = "0199a000-0000-7000-8000-000000000002";

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required for user brand isolation browser tests.`);
  return value;
}

const auroraAdminUsername = requiredEnv("TEST_ADMIN_USERNAME");
const auroraAdminPassword = requiredEnv("TEST_ADMIN_PASSWORD");
const harborAdminUsername = requiredEnv("TEST_HARBOR_ADMIN_USERNAME");
const harborAdminPassword = requiredEnv("TEST_HARBOR_ADMIN_PASSWORD");
const uid = () => crypto.randomUUID();

type Envelope<T> = { success: boolean; data: T; error?: { code?: string; message?: string } };
type MemberView = {
  user: { id: string };
  member: { id: string; brand_id: string };
};
type Wallet = { brand_id: string; member_id: string; display_points: string; available_points: string };
type Recharge = { id: string; brand_id: string; member_id: string; points: string; version: number | string };

// Playwright's Node fetch cannot resolve *.localhost on every host. Forward
// through localhost while retaining Harbor's real HTTP authority and cookies.
async function fetchHarbor(route: Route) {
  const response = await route.fetch({
    url: route.request().url().replace("harbor.localhost", "localhost"),
    headers: {
      ...(await route.request().allHeaders()),
      host: new URL(harborOrigin).host,
    },
  });
  await route.fulfill({ response });
}

async function harborFetch<T>(page: import("@playwright/test").Page, path: string, headers: Record<string, string> = {}) {
  return page.evaluate(async ({ path, headers }) => {
    const response = await fetch(path, { credentials: "include", headers });
    return { status: response.status, body: await response.json() as Envelope<T> };
  }, { path, headers });
}

test("one global user has isolated Aurora and Harbor memberships, wallets, and recharge records", async ({ browser, playwright }) => {
  test.setTimeout(60_000);
  const mobile = test.info().project.name === "mobile";
  const options = {
    viewport: mobile ? { width: 360, height: 800 } : { width: 1440, height: 900 },
    ...(mobile ? { isMobile: true, hasTouch: true } : {}),
  };
  const auroraContext = await browser.newContext(options);
  const harborContext = await browser.newContext(options);
  const auroraAdmin = await playwright.request.newContext();
  const harborAdmin = await playwright.request.newContext();

  try {
    await auroraContext.addInitScript(() => localStorage.setItem("luma-language", "en"));
    await harborContext.addInitScript(() => localStorage.setItem("luma-language", "en"));
    const auroraPage = await auroraContext.newPage();
    const harborPage = await harborContext.newPage();
    await harborPage.route("**/api/**", fetchHarbor);

    const username = `brand_iso_${uid().replaceAll("-", "").slice(0, 16)}`;
    const password = `Isolate-${uid()}-Pass2026!`;

    await auroraPage.goto(`${auroraOrigin}/register`);
    await auroraPage.getByLabel("Choose a username or phone", { exact: true }).fill(username);
    await auroraPage.getByLabel("Password", { exact: true }).fill(password);
    await auroraPage.locator('.auth-form input[type="checkbox"]').nth(0).check();
    await auroraPage.locator('.auth-form input[type="checkbox"]').nth(1).check();
    const registration = auroraPage.waitForResponse(response =>
      response.url() === `${auroraOrigin}/api/v1/auth/register` && response.request().method() === "POST",
    );
    await auroraPage.getByRole("button", { name: /Continue/ }).click();
    expect((await registration).status()).toBe(201);
    await expect(auroraPage).toHaveURL(/\/account$/);

    const auroraMeResponse = await auroraPage.request.get(`${auroraOrigin}/api/v1/me`);
    expect(auroraMeResponse.status()).toBe(200);
    const auroraMe = (await auroraMeResponse.json() as Envelope<MemberView>).data;
    expect(auroraMe.member.brand_id).toBe(auroraBrand);

    await harborPage.goto(`${harborOrigin}/login`);
    await harborPage.getByLabel("Username or phone", { exact: true }).fill(username);
    await harborPage.getByLabel("Password", { exact: true }).fill(password);
    const initialLogin = harborPage.waitForResponse(response =>
      response.url().endsWith("/api/v1/auth/login") && response.request().method() === "POST" && response.status() === 409,
    );
    await harborPage.getByRole("button", { name: /Continue/ }).click();
    await initialLogin;
    const joinTerms = harborPage.locator(".join-terms");
    await expect(joinTerms).toContainText("This account already exists. Joining this brand requires accepting its terms:");
    await joinTerms.locator('input[type="checkbox"]').check();
    const joined = harborPage.waitForResponse(response =>
      response.url().endsWith("/api/v1/auth/login") && response.request().method() === "POST" && response.status() === 200,
    );
    await joinTerms.getByRole("button", { name: /Accept and join brand/ }).click();
    expect((await joined).status()).toBe(200);
    await expect(harborPage).toHaveURL(/\/account$/);

    const harborMeResult = await harborFetch<MemberView>(harborPage, "/api/v1/me");
    expect(harborMeResult.status).toBe(200);
    const harborMe = harborMeResult.body.data;
    expect(harborMe.user.id).toBe(auroraMe.user.id);
    expect(harborMe.member.brand_id).toBe(harborBrand);
    expect(harborMe.member.id).not.toBe(auroraMe.member.id);

    const auroraAdminLogin = await auroraAdmin.post(`${adminOrigin}/api/v1/admin/auth/login`, {
      headers: {
        Origin: adminOrigin,
        "X-Brand-ID": auroraBrand,
        "Idempotency-Key": uid(),
      },
      data: { identifier: auroraAdminUsername, password: auroraAdminPassword },
    });
    expect(auroraAdminLogin.status(), await auroraAdminLogin.text()).toBe(200);
    const harborAdminLogin = await harborAdmin.post(`${adminOrigin}/api/v1/admin/auth/login`, {
      headers: {
        Origin: adminOrigin,
        "X-Brand-ID": harborBrand,
        "Idempotency-Key": uid(),
      },
      data: { identifier: harborAdminUsername, password: harborAdminPassword },
    });
    expect(harborAdminLogin.status(), await harborAdminLogin.text()).toBe(200);
    const adminHeaders = { Origin: adminOrigin, "X-Brand-ID": auroraBrand };
    const createdResponse = await auroraAdmin.post(`${adminOrigin}/api/v1/admin/recharges`, {
      headers: { ...adminHeaders, "Idempotency-Key": uid() },
      data: {
        member_id: auroraMe.member.id,
        points: "17",
        proof_reference: `browser-isolation-${uid()}`,
        remark: "isolated browser test funding",
        reason: "Fund Aurora membership for cross-brand isolation verification",
      },
    });
    expect(createdResponse.status(), await createdResponse.text()).toBe(201);
    const recharge = (await createdResponse.json() as Envelope<Recharge>).data;
    const confirmedResponse = await auroraAdmin.post(`${adminOrigin}/api/v1/admin/recharges/${recharge.id}/confirm`, {
      headers: { ...adminHeaders, "Idempotency-Key": uid() },
      data: { version: recharge.version, reason: "Confirm isolated Aurora browser test funding" },
    });
    expect(confirmedResponse.status(), await confirmedResponse.text()).toBe(200);

    const auroraWalletResponse = await auroraPage.request.get(`${auroraOrigin}/api/v1/wallet`);
    expect(auroraWalletResponse.status()).toBe(200);
    const auroraWallet = (await auroraWalletResponse.json() as Envelope<Wallet>).data;
    expect(auroraWallet).toMatchObject({ brand_id: auroraBrand, member_id: auroraMe.member.id, display_points: "17", available_points: "17" });
    const harborWalletResult = await harborFetch<Wallet>(harborPage, "/api/v1/wallet", { "X-Brand-ID": auroraBrand });
    expect(harborWalletResult.status).toBe(200);
    expect(harborWalletResult.body.data).toMatchObject({ brand_id: harborBrand, member_id: harborMe.member.id, display_points: "0", available_points: "0" });

    await auroraPage.goto(auroraOrigin);
    await expect(auroraPage.getByTestId("home-wallet-balance")).toHaveText("17 pts");
    await harborPage.goto(harborOrigin);
    await expect(harborPage.getByTestId("home-wallet-balance")).toHaveText("0 pts");
    await expect(auroraPage.locator("body")).not.toContainText("12,840");
    await expect(harborPage.locator("body")).not.toContainText("12,840");

    await auroraPage.goto(`${auroraOrigin}/recharge`);
    const auroraRecords = auroraPage.locator(".recharge-page");
    await expect(auroraRecords.locator(".recharge-record")).toHaveCount(1);
    await expect(auroraRecords.locator(".recharge-record").first()).toContainText(recharge.id);
    await expect(auroraRecords.locator(".recharge-record").first()).toContainText("17");

    await harborPage.goto(`${harborOrigin}/recharge`);
    const harborRecords = harborPage.locator(".recharge-page");
    await expect(harborRecords).toContainText("No recharge records yet.");
    await expect(harborRecords.locator(".recharge-record")).toHaveCount(0);
    const harborList = await harborFetch<{ items: Recharge[]; total_count: string }>(
      harborPage,
      "/api/v1/recharges?limit=20&offset=0",
      { "X-Brand-ID": auroraBrand },
    );
    expect(harborList.status).toBe(200);
    expect(harborList.body.data.items).toEqual([]);
    expect(harborList.body.data.total_count).toBe("0");
    const crossBrandDetail = await harborFetch<unknown>(harborPage, `/api/v1/recharges/${recharge.id}`, { "X-Brand-ID": auroraBrand });
    expect(crossBrandDetail.status).toBe(404);
    expect(crossBrandDetail.body.success).toBe(false);

    const harborAdminHeaders = { Origin: adminOrigin, "X-Brand-ID": harborBrand };
    const harborAdminList = await harborAdmin.get(`${adminOrigin}/api/v1/admin/recharges?member_id=${harborMe.member.id}&limit=20&offset=0`, { headers: harborAdminHeaders });
    expect(harborAdminList.status()).toBe(200);
    expect((await harborAdminList.json() as Envelope<{ items: unknown[] }>).data.items).toEqual([]);
    const auroraMemberFromHarbor = await harborAdmin.get(`${adminOrigin}/api/v1/admin/wallets/${auroraMe.member.id}`, { headers: harborAdminHeaders });
    expect(auroraMemberFromHarbor.status()).toBe(404);

    const auroraListResponse = await auroraPage.request.get(`${auroraOrigin}/api/v1/recharges?limit=20&offset=0`);
    expect(auroraListResponse.status()).toBe(200);
    const auroraList = (await auroraListResponse.json() as Envelope<{ items: Recharge[]; total_count: string }>).data;
    expect(auroraList.items.map(item => item.id)).toEqual([recharge.id]);
    expect(auroraList.total_count).toBe("1");
    const finalAuroraWallet = await auroraPage.request.get(`${auroraOrigin}/api/v1/wallet`);
    expect(finalAuroraWallet.status()).toBe(200);
    expect((await finalAuroraWallet.json() as Envelope<Wallet>).data).toEqual(auroraWallet);
    const finalHarborWallet = await harborFetch<Wallet>(harborPage, "/api/v1/wallet");
    expect(finalHarborWallet.status).toBe(200);
    expect(finalHarborWallet.body.data).toEqual(harborWalletResult.body.data);
    for (const page of [auroraPage, harborPage]) {
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    }
  } finally {
    await harborAdmin.dispose();
    await auroraAdmin.dispose();
    await harborContext.close();
    await auroraContext.close();
  }
});
