import { test, expect } from "@playwright/test";
import { restoreAdminSession } from "./support/admin-session";

const harbor = "0199a000-0000-7000-8000-000000000002";
const origin = "http://localhost:5174";
const api = "http://localhost:5173/api/v1";
const admin = `${origin}/api/v1/admin`;
const unique = () => crypto.randomUUID();

type Envelope<T> = { success: boolean; data: T; error?: unknown };
type BrandOperation = { brand_id: string; version: number; name: string; status: "active" | "paused"; updated_at: string; audit_log_id?: string };
type Revision = { id: string; brand_id: string; version: number; previous_status: "active" | "paused"; status: "active" | "paused"; changed_by: string; reason: string; audit_log_id: string; created_at: string };

async function envelope<T>(response: Awaited<ReturnType<import("@playwright/test").APIRequestContext["get"]>>, label: string): Promise<T> {
  const text = await response.text();
  expect(response.status(), `${label}: ${text}`).toBe(200);
  const result = JSON.parse(text) as Envelope<T>;
  expect(result.success, `${label}: ${text}`).toBe(true);
  return result.data;
}

test("Harbor operation pause survives a dropped acknowledgment, records once, and resumes without blocking member reads", async ({ page, context }, info) => {
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD, "Provide isolated Harbor administrator credentials");
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  const headers = { Origin: origin, "X-Brand-ID": harbor };
  if (!await restoreAdminSession(context, process.env.TEST_HARBOR_ADMIN_USERNAME!, harbor)) {
    const login = await page.request.post(`${admin}/auth/login`, {
      headers: { ...headers, "Idempotency-Key": unique() },
      data: { identifier: process.env.TEST_HARBOR_ADMIN_USERNAME, password: process.env.TEST_HARBOR_ADMIN_PASSWORD },
    });
    const text = await login.text();
    expect(login.status(), `Harbor admin login: ${text}`).toBe(200);
  }

  const getContext = async (host: string) => envelope<{ brand: { id: string; name: string; status: string } }>(
    await page.request.get(`${api}/context`, { headers: { host } }), `GET context (${host})`,
  );
  const harborContext = await getContext("harbor.localhost:5173");
  const auroraContext = await getContext("localhost:5173");
  expect(harborContext.brand).toMatchObject({ id: harbor, status: "active" });
  expect(auroraContext.brand).toMatchObject({ name: "Aurora", status: "active" });

  // Create a real, isolated member before pausing, then authenticate it while
  // paused. No balance, limits, orders, draws, or shared fixture state change.
  const username = `brandop_${unique().replaceAll("-", "").slice(0, 12)}`;
  const password = `BrandOp-${unique()}-test`;
  const contextResponse = await page.request.get(`${api}/b/harbor/context`);
  const authContext = await envelope<{ terms: { privacy_policy_version: string; service_terms_version: string }; auth?: { captcha_enabled?: boolean } }>(contextResponse, "GET Harbor public context");
  let captcha: { captcha_id: string; captcha_answer: string } | Record<string, never> = {};
  if (authContext.auth?.captcha_enabled) {
    const challenge = await envelope<{ id: string; svg: string }>(await page.request.get(`${api}/b/harbor/auth/challenge`), "GET registration CAPTCHA");
    captcha = { captcha_id: challenge.id, captcha_answer: challenge.svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? "" };
    expect(captcha.captcha_answer).toMatch(/^[A-Za-z0-9]+$/);
  }
  const registration = await page.request.post(`${api}/b/harbor/auth/register`, {
    headers: { Origin: "http://localhost:5173", "Idempotency-Key": unique() },
    data: { username, password, ...authContext.terms, ...captcha },
  });
  const registrationText = await registration.text();
  expect(registration.status(), `Harbor member registration: ${registrationText}`).toBe(201);
  expect((JSON.parse(registrationText) as Envelope<{ member: { id: string } }>).success).toBe(true);

  const operation = async () => envelope<BrandOperation>(await page.request.get(`${admin}/brand-operation`, { headers }), "GET brand operation");
  const history = async () => envelope<{ items: Revision[]; limit: number; offset: number }>(
    await page.request.get(`${admin}/brand-operation/history?limit=20&offset=0`, { headers }), "GET brand-operation history",
  );
  let originalFailure: unknown;
  let initial: BrandOperation | null = null;
  let ownPauseVersion: number | null = null;
  try {
    initial = await operation();
    const initialHistory = await history();
    expect(initial).toMatchObject({ brand_id: harbor, status: "active" });
    expect(initial.version).toBeGreaterThan(0);
    expect(initialHistory).toMatchObject({ limit: 20, offset: 0 });

    const writes: Array<{ method: string; url: string; body: string | null; key: string | undefined }> = [];
    let firstStatus: number | undefined;
    const pageErrors: string[] = [];
    page.on("pageerror", (error) => pageErrors.push(error.message));
    // Keep this interception installed through page teardown. Removing the
    // pattern while a background GET is in flight can leave it unhandled.
    await page.route("**/api/v1/admin/brand-operation", async (route) => {
      if (route.request().method() !== "PATCH") {
        await route.continue();
        return;
      }
      const request = route.request();
      const requestHeaders = await request.allHeaders();
      const originalURL = new URL(request.url());
      writes.push({ method: request.method(), url: request.url(), body: request.postData(), key: requestHeaders["idempotency-key"] });
      expect(requestHeaders.origin).toBe(origin);
      // Chromium does not expose the Host header through allHeaders. The
      // observed URL provides the original authority forwarded to the API.
      expect(originalURL.host).toBe("localhost:5174");
      expect(requestHeaders.cookie, "the authenticated browser cookie must be forwarded to the API").toBeTruthy();
      expect(requestHeaders["x-brand-id"]).toBe(harbor);
      const response = await route.fetch({
        url: `http://127.0.0.1:8080${originalURL.pathname}${originalURL.search}`,
        method: request.method(),
        headers: { ...requestHeaders, host: originalURL.host },
        postData: request.postData() ?? "",
      });
      if (writes.length === 1) {
        firstStatus = response.status();
        expect(firstStatus, "the real pause request must commit successfully before its acknowledgment is dropped").toBe(200);
        const committed = await response.json() as Envelope<BrandOperation>;
        expect(committed.success).toBe(true);
        expect(committed.data).toMatchObject({brand_id:harbor,status:"paused",version:initial!.version+1});
        ownPauseVersion = committed.data.version;
        await route.abort("failed"); // The write completed; only its acknowledgment is lost.
      } else {
        await route.fulfill({ response });
      }
    });

    await page.goto(origin);
    await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(harbor);
    if (info.project.name === "mobile") {
      await page.locator(".mobile-nav button").nth(4).click();
      await page.locator(".mobile-more-menu").getByRole("button", { name: /品牌和域名/ }).click();
    } else {
      await page.locator(".side-nav").getByRole("button", { name: /品牌和域名/ }).click();
    }
    await page.getByRole("heading", { name: "品牌和域名", exact: true }).waitFor();
    const panel = page.locator(".brand-operation");
    await expect(panel.getByRole("heading", { name: "品牌运行状态", exact: true })).toBeVisible();
    await panel.getByLabel("目标状态", { exact: true }).selectOption("paused");
    await panel.getByLabel("操作原因", { exact: true }).fill("Verify Harbor pause and same-key retry after a lost acknowledgment");
    await panel.getByRole("button", { name: "核对状态变更", exact: true }).click();
    await panel.getByRole("button", { name: "确认提交", exact: true }).click();
    // The unknown-intent control appears as soon as a write starts. Wait for
    // actual server acceptance and the deliberately lost ACK before retrying.
    await expect.poll(() => firstStatus).toBe(200);
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeVisible();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeEnabled();
    expect(firstStatus).toBe(200);
    const uncertainRevision = await operation();
    expect(uncertainRevision).toMatchObject({ brand_id: harbor, status: "paused", version: initial.version + 1 });
    const pausedHistory = await history();
    const pauseEvents = pausedHistory.items.filter((item) => item.version === uncertainRevision.version);
    expect(pauseEvents).toHaveLength(1);
    expect(pauseEvents[0]).toMatchObject({ previous_status: "active", status: "paused" });

    await panel.getByRole("button", { name: "重新读取状态", exact: true }).click();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeVisible();
    await expect(panel.getByLabel("目标状态", { exact: true })).toBeDisabled();
    await panel.getByRole("button", { name: "刷新记录", exact: true }).click();
    await expect(panel.locator(".brand-operation__history-list")).toContainText("Verify Harbor pause and same-key retry after a lost acknowledgment");
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeVisible();
    await expect(panel.getByLabel("目标状态", { exact: true })).toBeDisabled();

    if (info.project.name === "mobile") {
      // Members is a primary mobile navigation item, not a More-menu item.
      await page.locator(".mobile-nav button").nth(1).click();
    } else {
      await page.locator(".side-nav").getByRole("button", { name: /用户和成员/ }).click();
    }
    await expect(page.getByRole("heading", { name: "用户和成员", exact: true })).toBeVisible();
    if (info.project.name === "mobile") {
      await page.locator(".mobile-nav button").last().click();
      await page.locator(".mobile-more-menu").getByRole("button", { name: /品牌和域名/ }).click();
    } else {
      await page.locator(".side-nav").getByRole("button", { name: /品牌和域名/ }).click();
    }
    await expect(page.getByRole("heading", { name: "品牌和域名", exact: true })).toBeVisible();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeVisible();
    await expect(panel.getByLabel("目标状态", { exact: true })).toBeDisabled();
    await expect(panel.locator(".brand-operation__history-list")).toContainText("Verify Harbor pause and same-key retry after a lost acknowledgment");

    await panel.getByRole("button", { name: "使用原键重试", exact: true }).click();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toHaveCount(0);
    // The replay receipt clears the frozen intent before the component's
    // independent status and history reads finish. Wait for its busy guard to
    // release before starting another operation.
    await expect(panel.getByLabel("目标状态", { exact: true })).toBeEnabled();
    expect(writes).toHaveLength(2);
    expect(writes[0]).toEqual(writes[1]);
    expect((await getContext("harbor.localhost:5173")).brand).toMatchObject({ id: harbor, status: "paused" });
    expect((await getContext("localhost:5173")).brand).toMatchObject({ name: "Aurora", status: "active" });

    let loginCaptcha: { captcha_id: string; captcha_answer: string } | Record<string, never> = {};
    if (authContext.auth?.captcha_enabled) {
      const challenge = await envelope<{ id: string; svg: string }>(await page.request.get(`${api}/b/harbor/auth/challenge`), "GET login CAPTCHA");
      loginCaptcha = { captcha_id: challenge.id, captcha_answer: challenge.svg.match(/<text\b[^>]*>([^<]+)<\/text>/)?.[1] ?? "" };
      expect(loginCaptcha.captcha_answer).toMatch(/^[A-Za-z0-9]+$/);
    }
    const userLogin = await page.request.post(`${api}/b/harbor/auth/login`, {
      headers: { Origin: "http://localhost:5173", "Idempotency-Key": unique() },
      data: { identifier: username, password, ...loginCaptcha },
    });
    const userLoginText = await userLogin.text();
    expect(userLogin.status(), `Harbor user login while paused: ${userLoginText}`).toBe(200);
    const userData = (JSON.parse(userLoginText) as Envelope<{ access_token: string }>).data;
    expect(userData.access_token).toBeTruthy();
    const me = await page.request.get(`${api}/b/harbor/me`, { headers: { Origin: "http://localhost:5173", Authorization: `Bearer ${userData.access_token}` } });
    const meData = await envelope<{ member: { brand_id: string } }>(me, "GET Harbor /me while paused");
    expect(meData.member.brand_id).toBe(harbor);
    const wallet = await page.request.get(`${api}/b/harbor/wallet`, { headers: { Origin: "http://localhost:5173", Authorization: `Bearer ${userData.access_token}` } });
    expect(await envelope<Record<string, unknown>>(wallet, "GET Harbor wallet while paused")).toBeTruthy();

    const pausedOperation = await operation();
    await panel.getByLabel("目标状态", { exact: true }).selectOption("active");
    await panel.getByLabel("操作原因", { exact: true }).fill("Restore Harbor after pause verification");
    await panel.getByRole("button", { name: "核对状态变更", exact: true }).click();
    const resumeResponse = page.waitForResponse((response) => {
      const request = response.request();
      return request.method() === "PATCH" && new URL(response.url()).pathname === "/api/v1/admin/brand-operation" && response.status() === 200;
    });
    await panel.getByRole("button", { name: "确认提交", exact: true }).click();
    await resumeResponse;
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toHaveCount(0);
    const resumed = await operation();
    expect(resumed).toMatchObject({ brand_id: harbor, status: "active", version: pausedOperation.version + 1 });
    const resumedHistory = await history();
    const resumeEvents = resumedHistory.items.filter((item) => item.version === resumed.version);
    expect(resumeEvents).toHaveLength(1);
    expect(resumeEvents[0]).toMatchObject({ previous_status: "paused", status: "active" });
    expect((await getContext("harbor.localhost:5173")).brand).toMatchObject({ id: harbor, status: "active" });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    expect(pageErrors).toEqual([]);
    await page.screenshot({ path: info.outputPath(`s7b-brand-operation-${info.project.name}.png`), fullPage: true });
  } catch (cause) {
    originalFailure = cause;
    throw cause;
  } finally {
    try {
      // Harbor is the isolated brand under test. Recover an interrupted run
      // too, while leaving all unrelated fixtures and financial state alone.
      const current = await operation();
      if (initial?.status === "active" && ownPauseVersion !== null && current.brand_id === harbor && current.status === "paused" &&
        current.version === ownPauseVersion && current.version === initial.version + 1) {
        const cleanup = await page.request.patch(`${admin}/brand-operation`, {
          headers: { ...headers, "Idempotency-Key": unique() },
          data: {
            version: current.version,
            status: initial.status,
            reason: "Restore Harbor to its initial active state after brand-operation browser test (automatic cleanup)",
          },
        });
        const cleanupText = await cleanup.text();
        expect(cleanup.status(), `Harbor brand-operation cleanup: ${cleanupText}`).toBe(200);
        const result = JSON.parse(cleanupText) as Envelope<BrandOperation>;
        expect(result.success, `Harbor brand-operation cleanup: ${cleanupText}`).toBe(true);
      }
    } catch (cleanupFailure) {
      if (originalFailure !== undefined) {
        console.error("Harbor brand-operation cleanup also failed after the original test failure:", cleanupFailure);
      } else {
        throw cleanupFailure;
      }
    }
  }
});
