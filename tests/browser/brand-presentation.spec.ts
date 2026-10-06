import { test, expect, type APIRequestContext, type BrowserContext, type Locator } from "@playwright/test";
import { restoreAdminSession } from "./support/admin-session";

const harbor = "0199a000-0000-7000-8000-000000000002";
const adminOrigin = "http://localhost:5174";
const admin = `${adminOrigin}/api/v1/admin`;
const unique = () => crypto.randomUUID();
const announcement = '<img src=x onerror="window.__brandPresentationXss=1">';

type Envelope<T> = { success: boolean; data: T; error?: unknown };
type BrandPresentationConfig = Record<string, unknown> & {
  display_name: string | null;
  logo_text: string | null;
  primary_color: string | null;
  accent_color: string | null;
  font_family: string | null;
  radius: string | null;
  default_locale: "en" | "zh-CN" | null;
  available_locales: string[] | null;
  content: { en: { tagline: string | null; announcement: string | null }; "zh-CN": { tagline: string | null; announcement: string | null } } | null;
};
type BrandPresentationRecord = {
  brand_id: string;
  version: number;
  status: "active" | "paused" | "disabled";
  base_name: string;
  config: BrandPresentationConfig;
  effective: Record<string, unknown> & { display_name: string; logo_text: string; primary_color: string; accent_color: string; font_family: string; radius: string; default_locale: "en" | "zh-CN"; available_locales: string[]; content: Record<string, { tagline: string; announcement: string }> };
  updated_at: string;
  audit_log_id?: string;
};
type BrandPresentationRevision = { version: number; reason: string; audit_log_id: string; config: BrandPresentationConfig; effective: BrandPresentationRecord["effective"] };
type BrandPresentationHistory = { items: BrandPresentationRevision[]; limit: number; offset: number };

async function data<T>(response: Awaited<ReturnType<APIRequestContext["get"]>>, label: string, status = 200): Promise<T> {
  const text = await response.text();
  expect(response.status(), `${label}: ${text}`).toBe(status);
  const envelope = JSON.parse(text) as Envelope<T>;
  expect(envelope.success, `${label}: ${text}`).toBe(true);
  return envelope.data;
}

async function ensureAdmin(context: BrowserContext, page: import("@playwright/test").Page): Promise<void> {
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

async function readPresentation(page: import("@playwright/test").Page): Promise<BrandPresentationRecord> {
  return data<BrandPresentationRecord>(await page.request.get(`${admin}/brand-presentation`, {
    headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
  }), "GET Harbor brand presentation");
}

async function restorePresentation(page: import("@playwright/test").Page, original: BrandPresentationConfig): Promise<void> {
  for (let conflict = 0; conflict < 4; conflict++) {
    const current = await readPresentation(page);
    if (JSON.stringify(current.config) === JSON.stringify(original)) return;
    const key = unique();
    const body = { version: current.version, config: original, reason: "Restore Harbor brand presentation after browser verification" };
    const send = () => page.request.put(`${admin}/brand-presentation`, {
      headers: { Origin: adminOrigin, "X-Brand-ID": harbor, "Idempotency-Key": key }, data: body,
    });
    let response;
    let replayedUnknown = false;
    try {
      response = await send();
    } catch {
      // A transport failure leaves the outcome unknown. Replay this exact
      // request once with its original key; do not create a second intent.
      response = await send();
      replayedUnknown = true;
    }
    if (response.status() >= 500 && !replayedUnknown) {
      response = await send();
      replayedUnknown = true;
    }
    if (response.status() >= 500) throw new Error(`Restore Harbor brand presentation remained uncertain after one same-key replay (${response.status()})`);
    if (response.status() === 409 && conflict < 3) continue;
    const receipt = await data<BrandPresentationRecord>(response, "Restore Harbor brand presentation");
    expect(receipt.config).toEqual(original);
    return;
  }
  throw new Error("Could not restore Harbor brand presentation after concurrent version changes");
}

async function openBrandSettings(page: import("@playwright/test").Page, projectName: string) {
  await page.goto(adminOrigin);
  await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(harbor);
  if (projectName === "mobile") {
    await page.locator(".mobile-nav button").nth(4).click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: /品牌和域名/ }).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: /品牌和域名/ }).click();
  }
  await expect(page.getByRole("heading", { name: "品牌和域名", exact: true })).toBeVisible();
  const panel = page.locator(".brand-presentation");
  await expect(panel.getByRole("heading", { name: "品牌展示配置", exact: true })).toBeVisible();
  return panel;
}

async function inherit(panel: Locator, label: string, field: Locator): Promise<void> {
  const checkbox = panel.getByLabel(label, { exact: true });
  if (await checkbox.isChecked()) {
    await expect(field).toBeDisabled();
    await checkbox.uncheck();
  }
  await expect(checkbox).not.toBeChecked();
  await expect(field).toBeEnabled();
}

async function leaveAndReturn(page: import("@playwright/test").Page, projectName: string): Promise<void> {
  if (projectName === "mobile") {
    await page.locator(".mobile-nav button").nth(1).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: /用户和成员/ }).click();
  }
  await expect(page.getByRole("heading", { name: "用户和成员", exact: true })).toBeVisible();
  if (projectName === "mobile") {
    await page.locator(".mobile-nav button").last().click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: /品牌和域名/ }).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: /品牌和域名/ }).click();
  }
}

test("Harbor presentation survives a lost acknowledgment, renders safely, and restores its original overrides", async ({ page, context }, info) => {
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD, "Provide isolated Harbor administrator credentials");
  test.setTimeout(90_000);
  page.setDefaultTimeout(10_000);
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await ensureAdmin(context, page);

  const original = await readPresentation(page);
  expect(original).toMatchObject({ brand_id: harbor, version: expect.any(Number), config: expect.any(Object), effective: expect.any(Object) });
  const panel = await openBrandSettings(page, info.project.name);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({ path: info.outputPath(`s7c-brand-presentation-admin-${info.project.name}.png`), fullPage: true });

  test.skip(original.status === "disabled", "Disabled brands are read-only and cannot run the real-write scenario");

  const updated: BrandPresentationConfig = {
    ...original.config,
    display_name: "Harbor Brand Preview",
    logo_text: "HARBOR PREVIEW",
    logo_url: null,
    favicon_url: null,
    primary_color: "#174a7e",
    accent_color: "#e7a93b",
    font_family: "serif",
    radius: "round",
    default_locale: "zh-CN",
    available_locales: ["en", "zh-CN"],
    content: {
      en: { tagline: "Harbor English preview", announcement: "Harbor presentation browser verification" },
      "zh-CN": { tagline: "港湾品牌展示验证", announcement },
    },
  };
  let firstStatus: number | undefined;
  const writes: Array<{ method: string; url: string; body: string | null; key: string | undefined }> = [];
  const routePattern = "**/api/v1/admin/brand-presentation";
  await page.route(routePattern, async (route) => {
    if (route.request().method() !== "PUT") { await route.continue(); return; }
    const request = route.request();
    const requestHeaders = await request.allHeaders();
    const originalURL = new URL(request.url());
    writes.push({ method: request.method(), url: request.url(), body: request.postData(), key: requestHeaders["idempotency-key"] });
    expect(requestHeaders.origin).toBe(adminOrigin);
    // Chromium omits Host from allHeaders; retain the original URL authority.
    expect(originalURL.host).toBe("localhost:5174");
    expect(requestHeaders.cookie, "the real authenticated admin cookie must accompany the write").toBeTruthy();
    expect(requestHeaders["x-brand-id"]).toBe(harbor);
    const response = await route.fetch({
      url: `http://127.0.0.1:8080${originalURL.pathname}${originalURL.search}`,
      method: request.method(),
      headers: { ...requestHeaders, host: originalURL.host },
      postData: request.postData() ?? "",
    });
    if (writes.length === 1) {
      firstStatus = response.status();
      expect(firstStatus, "the real presentation update must commit before its acknowledgment is dropped").toBe(200);
      await route.abort("failed");
    } else {
      await route.fulfill({ response });
    }
  });

  try {
    const localizedField = (label: string) => panel.locator(".localized-field").filter({ hasText: label }).locator("textarea");
    const overrides: Array<[string, Locator]> = [
      ["展示名称继承默认", panel.getByLabel("展示名称", { exact: true })],
      ["标志文字继承默认", panel.getByLabel("标志文字", { exact: true })],
      ["主色继承默认", panel.getByLabel("主色", { exact: true })],
      ["辅助色继承默认", panel.getByLabel("辅助色", { exact: true })],
      ["字体预设继承默认", panel.getByLabel("字体预设", { exact: true })],
      ["圆角预设继承默认", panel.getByLabel("圆角预设", { exact: true })],
      ["默认语言继承默认", panel.getByLabel("默认语言", { exact: true })],
      ["可用语言继承默认", panel.getByLabel("启用English", { exact: true })],
      ["文案继承默认", localizedField("英文标语")], ["英文标语继承默认", localizedField("英文标语")],
      ["英文公告继承默认", localizedField("英文公告")], ["中文标语继承默认", localizedField("中文标语")],
      ["中文公告继承默认", localizedField("中文公告")],
    ];
    for (const [label, field] of overrides) await inherit(panel, label, field);
    for (const [label, field] of [
      ["Logo地址继承默认", panel.getByLabel("Logo地址", { exact: true })],
      ["favicon地址继承默认", panel.getByLabel("favicon地址", { exact: true })],
    ] as Array<[string, Locator]>) {
      const checkbox = panel.getByLabel(label, { exact: true });
      if (!await checkbox.isChecked()) await checkbox.check();
      await expect(checkbox).toBeChecked();
      await expect(field).toBeDisabled();
    }
    await panel.getByLabel("展示名称", { exact: true }).fill(updated.display_name!);
    await panel.getByLabel("标志文字", { exact: true }).fill(updated.logo_text!);
    await panel.getByLabel("主色", { exact: true }).fill(updated.primary_color!);
    await panel.getByLabel("辅助色", { exact: true }).fill(updated.accent_color!);
    await panel.getByLabel("默认语言", { exact: true }).selectOption("zh-CN");
    await panel.getByLabel("字体预设", { exact: true }).selectOption("serif");
    await panel.getByLabel("圆角预设", { exact: true }).selectOption("round");
    await panel.getByLabel("启用English", { exact: true }).check();
    await panel.getByLabel("启用简体中文", { exact: true }).check();
    await panel.getByLabel("英文标语", { exact: true }).fill(updated.content!.en.tagline!);
    await panel.getByLabel("英文公告", { exact: true }).fill(updated.content!.en.announcement!);
    await panel.getByLabel("中文标语", { exact: true }).fill(updated.content!["zh-CN"].tagline!);
    await panel.getByLabel("中文公告", { exact: true }).fill(announcement);
    await panel.getByLabel("操作原因", { exact: true }).fill("Verify Harbor presentation and same-key retry after lost acknowledgment");
    await panel.getByRole("button", { name: "核对展示配置", exact: true }).click();
    await panel.getByRole("button", { name: "确认提交", exact: true }).click();
    await expect.poll(() => firstStatus).toBe(200);
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeVisible();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeEnabled();
    await expect(panel.getByLabel("展示名称", { exact: true })).toBeDisabled();
    await expect(panel.getByLabel("操作原因", { exact: true })).toBeDisabled();

    const committed = await readPresentation(page);
    const submittedVersion = (JSON.parse(writes[0].body ?? "{}") as { version?: number }).version;
    expect(submittedVersion).toEqual(expect.any(Number));
    expect(committed).toMatchObject({
      brand_id: harbor, status: original.status, version: submittedVersion! + 1, config: updated,
      effective: {
        display_name: updated.display_name, logo_text: updated.logo_text,
        logo_url: null, favicon_url: null,
        primary_color: updated.primary_color, accent_color: updated.accent_color,
        font_family: updated.font_family, radius: updated.radius, default_locale: "zh-CN",
        available_locales: ["en", "zh-CN"],
        content: { en: updated.content!.en, "zh-CN": updated.content!["zh-CN"] },
      },
    });
    const history = await data<BrandPresentationHistory>(await page.request.get(`${admin}/brand-presentation/history?limit=20&offset=0`, {
      headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
    }), "GET Harbor presentation history");
    const firstRevision = history.items.filter((item) => item.version === committed.version);
    expect(firstRevision).toHaveLength(1);
    expect(firstRevision[0]).toMatchObject({ reason: "Verify Harbor presentation and same-key retry after lost acknowledgment", config: updated });

    // A read after an uncertain acknowledgment and a leave/back cycle must
    // preserve the exact pending intent until its original key is retried.
    await panel.getByRole("button", { name: "重新读取配置", exact: true }).click();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeEnabled();
    await panel.getByRole("button", { name: "刷新记录", exact: true }).click();
    await expect(panel).toContainText("Verify Harbor presentation and same-key retry after lost acknowledgment");
    await expect(panel.getByLabel("展示名称", { exact: true })).toBeDisabled();
    await leaveAndReturn(page, info.project.name);
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toBeEnabled();
    await expect(panel.getByLabel("展示名称", { exact: true })).toBeDisabled();
    await panel.getByRole("button", { name: "使用原键重试", exact: true }).click();
    await expect(panel.getByRole("button", { name: "使用原键重试", exact: true })).toHaveCount(0);
    expect(writes).toHaveLength(2);
    expect(writes[0]).toEqual(writes[1]);

    const current = await readPresentation(page);
    expect(current.version).toBeGreaterThanOrEqual(committed.version);
    expect(current.config).toEqual(updated);
    await expect(page.locator(".sidebar .brand-lockup")).toContainText(updated.display_name!);
    await expect.poll(() => page.locator(".app-shell").evaluate((root) => getComputedStyle(root).getPropertyValue("--primary").trim())).toBe(updated.primary_color);
    await page.screenshot({ path: info.outputPath(`s7c-brand-presentation-admin-published-${info.project.name}.png`), fullPage: true });
    const replayHistory = await data<BrandPresentationHistory>(await page.request.get(`${admin}/brand-presentation/history?limit=20&offset=0`, {
      headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
    }), "GET Harbor presentation history after retry");
    expect(replayHistory.items.filter((item) => item.version === current.version)).toHaveLength(1);

    const user = await context.newPage();
    const userErrors: string[] = [];
    user.on("pageerror", (error) => userErrors.push(error.message));
    await user.goto("http://harbor.localhost:5173/");
    await expect(user.locator(".hero h1")).toHaveText("港湾品牌展示验证");
    await expect(user.locator(".announcement h3")).toHaveText(announcement);
    await expect(user.locator(".announcement img, .announcement script")).toHaveCount(0);
    expect(await user.evaluate(() => (window as Window & { __brandPresentationXss?: unknown }).__brandPresentationXss)).toBeUndefined();
    const languageSwitch = user.getByRole("button", { name: "Switch to English", exact: true });
    await expect(languageSwitch).toBeVisible();
    await languageSwitch.click();
    await expect(user.locator(".hero h1")).toHaveText("Harbor English preview");
    await expect(user.locator(".announcement h3")).toHaveText("Harbor presentation browser verification");
    await user.getByRole("button", { name: "Switch to Chinese", exact: true }).click();
    await expect(user.locator(".hero h1")).toHaveText("港湾品牌展示验证");
    await expect(user.locator(".announcement h3")).toHaveText(announcement);
    await expect(user.locator(".announcement img, .announcement script")).toHaveCount(0);
    const brandLink = user.locator(".sidebar .brand");
    if (info.project.name === "mobile") {
      await user.getByRole("button", { name: "Open navigation", exact: true }).click();
    }
    await expect(brandLink).toBeVisible();
    await expect(brandLink).toContainText("Harbor Brand Preview");
    const logoMark = brandLink.locator(".brand-mark");
    await expect(logoMark.locator("img")).toHaveCount(0);
    await expect(logoMark).toHaveText("HARBOR PREVIEW");
    if (info.project.name === "mobile") {
      const navigation = user.getByRole("button", { name: "Open navigation", exact: true });
      await user.getByRole("button", { name: "Close navigation", exact: true }).click();
      await expect(navigation).toHaveAttribute("aria-expanded", "false");
      await expect.poll(() => user.locator(".sidebar").evaluate((drawer) => drawer.getBoundingClientRect().right)).toBeLessThanOrEqual(0);
    }

    // This repository has no allowed raster file in public/; serve a tiny PNG
    // for the accepted same-origin relative asset path and verify both logo
    // rendering and the page's branded favicon link against that response.
    const logoAssetPath = `/brand-assets/brand-presentation-${unique()}.png`;
    await user.route(`http://harbor.localhost:5173${logoAssetPath}`, (route) => route.fulfill({
      status: 200,
      contentType: "image/png",
      body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jPZkAAAAASUVORK5CYII=", "base64"),
    }));

    const assetInherit = [
      ["Logo地址继承默认", panel.getByLabel("Logo地址", { exact: true })],
      ["favicon地址继承默认", panel.getByLabel("favicon地址", { exact: true })],
    ] as Array<[string, Locator]>;
    for (const [label, field] of assetInherit) await inherit(panel, label, field);
    await panel.getByLabel("Logo地址", { exact: true }).fill(logoAssetPath);
    await panel.getByLabel("favicon地址", { exact: true }).fill(logoAssetPath);
    await panel.getByLabel("操作原因", { exact: true }).fill("Verify same-origin relative logo and favicon rendering");
    await panel.getByRole("button", { name: "核对展示配置", exact: true }).click();
    const assetPublish = page.waitForResponse((response) => response.request().method() === "PUT" &&
      new URL(response.url()).pathname === "/api/v1/admin/brand-presentation" && response.status() === 200);
    await panel.getByRole("button", { name: "确认提交", exact: true }).click();
    await assetPublish;
    expect(writes).toHaveLength(3);
    expect(JSON.parse(writes[2].body ?? "{}")).toMatchObject({ config: { ...updated, logo_url: logoAssetPath, favicon_url: logoAssetPath } });
    expect(writes[2].key).toBeTruthy();
    expect(writes[2].key).not.toBe(writes[0].key);
    const assetRecord = await readPresentation(page);
    expect(assetRecord).toMatchObject({ config: { ...updated, logo_url: logoAssetPath, favicon_url: logoAssetPath } });
    const assetHistory = await data<BrandPresentationHistory>(await page.request.get(`${admin}/brand-presentation/history?limit=20&offset=0`, {
      headers: { Origin: adminOrigin, "X-Brand-ID": harbor },
    }), "GET Harbor presentation history after relative asset publish");
    const assetRevision = assetHistory.items.filter((item) => item.version === assetRecord.version);
    expect(assetRevision).toHaveLength(1);
    expect(assetRevision[0]).toMatchObject({ reason: "Verify same-origin relative logo and favicon rendering" });
    await page.unroute(routePattern);

    await user.reload();
    if (info.project.name === "mobile") {
      await user.getByRole("button", { name: "Open navigation", exact: true }).click();
    }
    const renderedLogo = user.locator(".sidebar .brand .brand-mark img");
    await expect(renderedLogo).toHaveAttribute("src", logoAssetPath);
    await expect(renderedLogo).toHaveAttribute("alt", "HARBOR PREVIEW");
    await expect.poll(() => renderedLogo.evaluate((image: HTMLImageElement) => image.naturalWidth)).toBe(1);
    const favicon = user.locator('link[data-brand-favicon]');
    await expect(favicon).toHaveAttribute("href", logoAssetPath);
    expect(await favicon.evaluate((link: HTMLLinkElement) => link.href)).toBe(new URL(logoAssetPath, user.url()).href);
    if (info.project.name === "mobile") {
      const navigation = user.getByRole("button", { name: "Open navigation", exact: true });
      await user.getByRole("button", { name: "Close navigation", exact: true }).click();
      await expect(navigation).toHaveAttribute("aria-expanded", "false");
      await expect.poll(() => user.locator(".sidebar").evaluate((drawer) => drawer.getBoundingClientRect().right)).toBeLessThanOrEqual(0);
    }
    const rendered = await user.evaluate(() => ({
      primary: getComputedStyle(document.documentElement).getPropertyValue("--primary").trim(),
      accent: getComputedStyle(document.documentElement).getPropertyValue("--accent").trim(),
      fontFamily: getComputedStyle(document.documentElement).getPropertyValue("--font-family").trim(),
      brand: document.querySelector(".sidebar .brand")?.textContent ?? "",
    }));
    expect(rendered).toMatchObject({ primary: "#174a7e", accent: "#e7a93b" });
    expect(rendered.fontFamily).toContain("Georgia");
    expect(rendered.brand).toContain("Harbor Brand Preview");
    expect(await user.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    await user.screenshot({ path: info.outputPath(`s7c-brand-presentation-public-${info.project.name}.png`), fullPage: true });
    await user.close();
    expect(userErrors).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    expect(pageErrors).toEqual([]);
  } finally {
    await page.unroute(routePattern).catch(() => undefined);
    await restorePresentation(page, original.config);
  }
});
