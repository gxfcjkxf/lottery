import { test, expect, type APIRequestContext, type BrowserContext, type Locator, type Page } from "@playwright/test";
import { restoreAdminSession } from "./support/admin-session";

const harbor = "0199a000-0000-7000-8000-000000000002";
const adminOrigin = "http://localhost:5174";
const apiOrigin = "http://127.0.0.1:8080";
const admin = `${adminOrigin}/api/v1/admin`;
const unique = () => crypto.randomUUID();

type Envelope<T> = { success: boolean; data: T; error?: { code?: string; message?: string } };
type BrandDomain = { id: string; domain: string; enabled: boolean; is_primary: boolean };
type BrandDomainRecord = {
  brand_id: string;
  version: number;
  status: "active" | "paused" | "disabled";
  domains: BrandDomain[];
  updated_at: string;
  audit_log_id?: string;
};
type BrandDomainRevision = {
  id: string;
  brand_id: string;
  version: number;
  changed_by: string;
  reason: string;
  audit_log_id: string;
  created_at: string;
  before_domains: BrandDomain[];
  domains: BrandDomain[];
};

async function readData<T>(response: Awaited<ReturnType<APIRequestContext["get"]>>, label: string, status = 200): Promise<T> {
  const text = await response.text();
  expect(response.status(), `${label}: ${text}`).toBe(status);
  const envelope = JSON.parse(text) as Envelope<T>;
  expect(envelope.success, `${label}: ${text}`).toBe(true);
  return envelope.data;
}

async function ensureAdmin(context: BrowserContext, page: Page): Promise<void> {
  const username = process.env.TEST_HARBOR_ADMIN_USERNAME!;
  const password = process.env.TEST_HARBOR_ADMIN_PASSWORD!;
  if (await restoreAdminSession(context, username, harbor)) return;
  const response = await page.request.post(`${admin}/auth/login`, {
    headers: { Origin: adminOrigin, "X-Brand-ID": harbor, "Idempotency-Key": unique() },
    data: { identifier: username, password },
  });
  const text = await response.text();
  expect(response.status(), `Harbor admin login: ${text}`).toBe(200);
  expect((JSON.parse(text) as Envelope<unknown>).success).toBe(true);
}

async function getDomains(page: Page): Promise<BrandDomainRecord> {
  return readData<BrandDomainRecord>(await page.request.get(`${admin}/brand-domains`, {
    headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
  }), "GET Harbor domains");
}

async function patchDomain(page: Page, id: string, version: number, values: Pick<BrandDomain, "enabled" | "is_primary">, reason: string): Promise<BrandDomainRecord> {
  const path = `${admin}/brand-domains/${id}`;
  const sendIntent = async (body: { version: number; enabled: boolean; is_primary: boolean; reason: string }, key: string) => {
    const frozenBody = JSON.stringify(body);
    const send = () => page.request.patch(path, {
      headers: { Origin: adminOrigin, "X-Brand-ID": harbor, "Idempotency-Key": key, "Content-Type": "application/json" },
      data: frozenBody,
    });
    let response: Awaited<ReturnType<APIRequestContext["patch"]>>;
    for (let attempt = 0; attempt < 3; attempt++) {
      try { response = await send(); }
      catch (cause) { if (attempt === 2) throw cause; else continue; }
      if (response.status() < 500 || attempt === 2) return response;
    }
    throw new Error("Domain PATCH replay loop ended unexpectedly");
  };
  const firstBody = { version, ...values, reason };
  let response = await sendIntent(firstBody, unique());
  if (response.status() === 409) {
    const latest = await getDomains(page);
    const latestDomain = latest.domains.find((domain) => domain.id === id);
    if (!latestDomain) throw new Error(`Harbor domain ${id} disappeared during cleanup conflict recovery`);
    if (latestDomain.enabled === values.enabled && latestDomain.is_primary === values.is_primary) return latest;
    response = await sendIntent({ version: latest.version, ...values, reason }, unique());
  }
  return readData<BrandDomainRecord>(response, `PATCH Harbor domain ${id}`);
}

async function restoreDomains(page: Page, original: BrandDomain[], createdID?: string, restoreReason?: string): Promise<void> {
  let current = await getDomains(page);
  const primary = original.find((domain) => domain.is_primary);
  if (primary) {
    const currentPrimaryTarget = current.domains.find((domain) => domain.id === primary.id);
    if (!currentPrimaryTarget) throw new Error(`Original Harbor domain ${primary.id} disappeared`);
    if (!currentPrimaryTarget.is_primary || !currentPrimaryTarget.enabled) {
      if (!primary.enabled) throw new Error("Stored Harbor primary domain is disabled; cannot safely restore it as primary");
      current = await patchDomain(page, primary.id, current.version, { enabled: true, is_primary: true }, restoreReason ?? "Restore Harbor primary domain after browser verification");
    }
  }

  current = await getDomains(page);
  for (const before of original) {
    const now = current.domains.find((domain) => domain.id === before.id);
    if (!now) throw new Error(`Original Harbor domain ${before.id} disappeared`);
    if (now.enabled !== before.enabled || now.is_primary !== before.is_primary) {
      if (before.is_primary && !before.enabled) throw new Error("Stored Harbor primary domain is disabled; cannot safely restore it as primary");
      current = await patchDomain(page, before.id, current.version, { enabled: before.enabled || before.is_primary, is_primary: before.is_primary }, "Restore Harbor domain bindings after browser verification");
    }
  }

  current = await getDomains(page);
  if (createdID) {
    const created = current.domains.find((domain) => domain.id === createdID);
    if (created && (created.enabled || created.is_primary)) {
      current = await patchDomain(page, created.id, current.version, { enabled: false, is_primary: false }, "Disable temporary Harbor domain after browser verification");
    }
  }
  current = await getDomains(page);
  if (!original.every((before) => current.domains.some((domain) => domain.id === before.id && domain.enabled === before.enabled && domain.is_primary === before.is_primary))) {
    throw new Error("Harbor domain state did not match its pre-test snapshot after the audited restore");
  }
  if (createdID && !current.domains.some((domain) => domain.id === createdID && !domain.enabled && !domain.is_primary)) {
    throw new Error("Temporary Harbor domain is not retained as a disabled, non-primary binding");
  }
}

async function assertHostResolves(page: Page, host: string): Promise<void> {
  const response = await page.request.get(`${apiOrigin}/api/v1/context`, { headers: { Host: host } });
  const text = await response.text();
  expect(response.status(), `GET context for ${host}: ${text}`).toBe(200);
  const envelope = JSON.parse(text) as Envelope<{ brand: { id: string } }>;
  expect(envelope.success).toBe(true);
  expect(envelope.data.brand.id).toBe(harbor);
}

async function assertHostDoesNotResolve(page: Page, host: string): Promise<void> {
  const response = await page.request.get(`${apiOrigin}/api/v1/context`, { headers: { Host: host } });
  const text = await response.text();
  expect(response.status(), `GET context for disabled ${host}: ${text}`).toBe(404);
  const envelope = JSON.parse(text) as Envelope<never>;
  expect(envelope.success).toBe(false);
  expect(envelope.error?.code).toBe("BRAND_NOT_FOUND");
}

async function openBrandSettings(page: Page, projectName: string) {
  await page.goto(adminOrigin);
  await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(harbor);
  if (projectName === "mobile") {
    await page.locator(".mobile-nav button").nth(4).click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: /品牌和域名/ }).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: /品牌和域名/ }).click();
  }
  await expect(page.locator(".brand-domains").getByRole("heading", { name: "Brand domains", exact: true })).toBeVisible();
  return page.locator(".brand-domains");
}

async function leaveAndReturn(page: Page, projectName: string): Promise<void> {
  if (projectName === "mobile") await page.locator(".mobile-nav button").nth(1).click();
  else await page.locator(".side-nav").getByRole("button", { name: /用户和成员/ }).click();
  await expect(page.getByRole("heading", { name: "用户和成员", exact: true })).toBeVisible();
  if (projectName === "mobile") {
    await page.locator(".mobile-nav button").last().click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: /品牌和域名/ }).click();
  } else await page.locator(".side-nav").getByRole("button", { name: /品牌和域名/ }).click();
}

async function editFromUI(page: Page, panel: Locator, hostname: string, values: { enabled: boolean; primary: boolean; reason: string }): Promise<void> {
  const row = panel.locator(".domain-row").filter({ hasText: hostname });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Edit", exact: true }).click();
  const enabled = panel.getByLabel("Enabled", { exact: true });
  if (await enabled.isChecked() !== values.enabled) {
    if (values.enabled) await enabled.check(); else await enabled.uncheck();
  }
  const primary = panel.getByLabel("Primary domain", { exact: true });
  if (await primary.isChecked() !== values.primary) {
    if (values.primary) await primary.check(); else await primary.uncheck();
  }
  await panel.getByLabel("Reason", { exact: true }).fill(values.reason);
  await panel.getByRole("button", { name: "Review change", exact: true }).click();
  await panel.getByRole("heading", { name: "Review domain change", exact: true }).waitFor();
  const response = pageWaitForDomainPatch(page);
  await panel.getByRole("button", { name: "Confirm and submit", exact: true }).click();
  await response;
}

function pageWaitForDomainPatch(page: Page) {
  return page.waitForResponse((response) => response.request().method() === "PATCH" &&
    new URL(response.url()).pathname.startsWith("/api/v1/admin/brand-domains/") && response.status() === 200);
}

test("Harbor domain writes preserve the frozen intent, primary binding, resolution, and audit history", async ({ page, context }, info) => {
	page.setDefaultTimeout(10_000);
	const pageErrors:string[]=[];
	page.on("pageerror",error=>pageErrors.push(error.message));
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD, "Provide isolated Harbor administrator credentials");
  test.setTimeout(60_000);
  await ensureAdmin(context, page);

  const original = await getDomains(page);
  expect(original.brand_id).toBe(harbor);
  expect(original.version).toBeGreaterThan(0);
  expect(original.status).not.toBe("disabled");
  const originalBindings = original.domains.map((domain) => ({ ...domain }));
  const originalPrimary = originalBindings.find((domain) => domain.is_primary);
  expect(originalPrimary, "Harbor starts with an enabled primary binding").toMatchObject({ enabled: true, is_primary: true });
  const hostname = `${unique().replaceAll("-", "").slice(0, 18)}.example.test`;
  const reason = "Verify Harbor domain binding, frozen retry, primary switch, and restore";
  const restoreReason = `Restore Harbor primary domain after browser verification ${hostname}`;
  let firstStatus: number | undefined;
  const writes: Array<{ method: string; url: string; body: string | null; key: string | undefined }> = [];
  let createdID: string | undefined;

  const panel = await openBrandSettings(page, info.project.name);
  await page.route("**/api/v1/admin/brand-domains", async (route) => {
    if (route.request().method() !== "POST") { await route.continue(); return; }
    const request = route.request();
    const headers = await request.allHeaders();
    const originalURL = new URL(request.url());
    writes.push({ method: request.method(), url: request.url(), body: request.postData(), key: headers["idempotency-key"] });
    expect(headers.origin).toBe(adminOrigin);
    expect(originalURL.host).toBe("localhost:5174");
    expect(headers.cookie, "the authenticated Harbor admin cookie must accompany the write").toBeTruthy();
    expect(headers["x-brand-id"]).toBe(harbor);
    const response = await route.fetch({
      url: `${apiOrigin}${originalURL.pathname}${originalURL.search}`,
      method: request.method(),
      headers: { ...headers, host: originalURL.host },
      postData: request.postData() ?? "",
    });
    if (writes.length === 1) {
      firstStatus = response.status();
      expect(firstStatus, "the binding must commit before its acknowledgment is dropped").toBe(201);
      const envelope = JSON.parse(await response.text()) as Envelope<BrandDomainRecord>;
      expect(envelope.success).toBe(true);
      createdID = envelope.data.domains.find((domain) => domain.domain === hostname)?.id;
      expect(createdID).toBeTruthy();
      await route.abort("failed");
    } else {
      await route.fulfill({ response });
    }
  });

  try {
    await panel.getByRole("button", { name: "Add domain", exact: true }).click();
    await panel.getByLabel("Domain hostname", { exact: true }).fill(hostname);
    await panel.getByLabel("Enabled", { exact: true }).uncheck();
    await panel.getByLabel("Reason", { exact: true }).fill(reason);
    await panel.getByRole("button", { name: "Review change", exact: true }).click();
    await panel.getByRole("heading", { name: "Review domain change", exact: true }).waitFor();
    await panel.getByRole("button", { name: "Confirm and submit", exact: true }).click();
    await expect.poll(() => firstStatus).toBe(201);
    expect(firstStatus).toBe(201);
    await expect(panel.getByRole("heading", { name: "Request outcome unknown", exact: true })).toBeVisible();
    await expect(panel.getByRole("button", { name: "Retry exact request", exact: true })).toBeEnabled();
    const committedBeforeReplay = await getDomains(page);
    expect(committedBeforeReplay.domains.some((domain) => domain.id === createdID && domain.domain === hostname && !domain.enabled && !domain.is_primary)).toBe(true);

    await leaveAndReturn(page, info.project.name);
    await expect(panel.getByRole("heading", { name: "Request outcome unknown", exact: true })).toBeVisible();
    await expect(panel.getByRole("button", { name: "Retry exact request", exact: true })).toBeEnabled();
    const retryResponse = page.waitForResponse((response) => response.request().method() === "POST" && new URL(response.url()).pathname === "/api/v1/admin/brand-domains" && response.status() === 201);
    await panel.getByRole("button", { name: "Retry exact request", exact: true }).click();
    const replay = await retryResponse;
    const replayEnvelope = JSON.parse(await replay.text()) as Envelope<BrandDomainRecord>;
    expect(replayEnvelope.success).toBe(true);
    expect(replayEnvelope.data.audit_log_id).toBeTruthy();
    expect(writes).toHaveLength(2);
    expect(writes[0]).toEqual(writes[1]);
    expect(createdID).toBeTruthy();

    let current = await getDomains(page);
    let added = current.domains.find((domain) => domain.id === createdID);
    expect(added).toMatchObject({ domain: hostname, enabled: false, is_primary: false });
    expect(current.domains.filter((domain) => domain.is_primary)).toEqual([expect.objectContaining({ id: originalPrimary!.id })]);

    await editFromUI(page, panel, hostname, { enabled: true, primary: true, reason: "Temporarily promote Harbor domain for resolution verification" });
    current = await getDomains(page);
    added = current.domains.find((domain) => domain.id === createdID);
    expect(added).toMatchObject({ enabled: true, is_primary: true });
    expect(current.domains.find((domain) => domain.id === originalPrimary!.id)).toMatchObject({ enabled: true, is_primary: false });
    expect(current.audit_log_id).toBeTruthy();
    await assertHostResolves(page, hostname);

    await editFromUI(page, panel, hostname, { enabled: false, primary: false, reason: "Disable temporary Harbor domain after resolution verification" });
    current = await getDomains(page);
    expect(current.domains.find((domain) => domain.id === createdID)).toMatchObject({ enabled: false, is_primary: false });
    await assertHostDoesNotResolve(page, hostname);

    const historyEnvelope = await readData<{ items: BrandDomainRevision[]; limit: number; offset: number }>(await page.request.get(`${admin}/brand-domains/history?limit=20&offset=0`, {
      headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
    }), "GET Harbor domain history");
    expect(historyEnvelope).toMatchObject({ limit: 20, offset: 0 });
    const revisions = historyEnvelope.items.filter((entry) => entry.domains.some((domain) => domain.id === createdID));
    expect(revisions.length).toBeGreaterThanOrEqual(3);
    expect(revisions.every((entry) => entry.audit_log_id && entry.reason)).toBe(true);
    expect(revisions.some((entry) => entry.domains.some((domain) => domain.domain === hostname && domain.enabled && domain.is_primary))).toBe(true);
  } finally {
    await restoreDomains(page, originalBindings, createdID, restoreReason);
  }

  const restored = await getDomains(page);
  expect(restored.domains.filter(({ id }) => originalBindings.some((before) => before.id === id)).map(({ id, enabled, is_primary }) => ({ id, enabled, is_primary })))
    .toEqual(originalBindings.map(({ id, enabled, is_primary }) => ({ id, enabled, is_primary })));
  expect(restored.domains.find((domain) => domain.id === createdID)).toMatchObject({ domain: hostname, enabled: false, is_primary: false });
  const finalHistory = await readData<{ items: BrandDomainRevision[]; limit: number; offset: number }>(await page.request.get(`${admin}/brand-domains/history?limit=20&offset=0`, {
    headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
  }), "GET Harbor domain history after audited restore");
  const restoreRevision = finalHistory.items.find((entry) => entry.reason === restoreReason);
  expect(restoreRevision?.audit_log_id).toBeTruthy();
  expect(restoreRevision?.before_domains.find((domain) => domain.id === originalPrimary!.id)).toMatchObject({ is_primary: false });
  expect(restoreRevision?.domains.find((domain) => domain.id === originalPrimary!.id)).toMatchObject({ enabled: true, is_primary: true });
  expect(restoreRevision?.domains.find((domain) => domain.id === createdID)).toMatchObject({ domain: hostname, enabled: false, is_primary: false });
	await panel.getByRole("button",{name:"Reload list",exact:true}).click();
	await expect(panel.locator(".domain-row").filter({hasText:originalPrimary!.domain}).locator(".primary-badge")).toHaveText("Primary");
	await panel.getByRole("button",{name:"Refresh history",exact:true}).click();
	await expect(panel.locator(".history")).toContainText(restoreReason);
	expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
	expect(pageErrors).toEqual([]);
	await panel.screenshot({path:info.outputPath(`s7d-brand-domains-${info.project.name}.png`)});
});
