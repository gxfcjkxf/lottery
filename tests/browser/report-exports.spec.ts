import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";

const harbor = "0199a000-0000-7000-8000-000000000002";
const adminBase = "http://localhost:5174/api/v1/admin";
const uiBase = "http://localhost:5174";
const username = process.env.TEST_EXPORT_ADMIN_USERNAME ?? process.env.TEST_HARBOR_ADMIN_USERNAME;
const password = process.env.TEST_EXPORT_ADMIN_PASSWORD ?? process.env.TEST_HARBOR_ADMIN_PASSWORD;
const platformUsername = process.env.TEST_EXPORT_PLATFORM_USERNAME;
const platformPassword = process.env.TEST_EXPORT_PLATFORM_PASSWORD;

type ExportFixture = { memberId: string; headers: Record<string, string> };

function formatDateTimeLocal(timestamp: number): string {
  const date = new Date(timestamp);
  const pad = (value: number) => String(value).padStart(2, "0");
  const minuteValue = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  return date.getSeconds() === 0 ? minuteValue : `${minuteValue}:${pad(date.getSeconds())}`;
}

async function signIn(page: Page, identifier = username, secret = password) {
  test.skip(!identifier || !secret, "Provide isolated report export administrator credentials");
  const response = await page.request.post(`${adminBase}/auth/login`, {
    headers: { Origin: uiBase, "X-Brand-ID": harbor, "Idempotency-Key": crypto.randomUUID() },
    data: { identifier, password: secret },
  });
  const body = await response.text();
  expect(response.status(), body).toBe(200);
}

async function createFixture(page: Page): Promise<ExportFixture> {
  const headers = { Origin: uiBase, "X-Brand-ID": harbor };
  const post = async (path: string, data: unknown, expectedStatus = 200) => {
    const response = await page.request.post(`${adminBase}${path}`, {
      headers: { ...headers, "Idempotency-Key": crypto.randomUUID() }, data,
    });
    const text = await response.text();
    expect(response.status(), text).toBe(expectedStatus);
    return JSON.parse(text).data;
  };
  const member = await post("/users", {
    username: `report_export_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`,
    password: "report-export-test-only-password-2026",
    reason: "isolated report export browser fixture",
  }, 201);
  const pending = await post("/recharges", {
    member_id: member.member_id, points: "25", proof_reference: "isolated report export test",
    remark: "report export fixture", reason: "fund report export fixture",
  }, 201);
  await post(`/recharges/${pending.id}/confirm`, { version: pending.version, reason: "confirm report export fixture" });
  await post(`/wallets/${member.member_id}/freeze`, { points: "5", reason: "report export balance fixture" });
  return { memberId: member.member_id, headers };
}

async function openReports(page: Page, projectName: string) {
  await page.goto(uiBase);
  await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(harbor);
  if (projectName === "mobile") {
    await page.locator(".mobile-nav button").nth(4).click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: /报表和对账/ }).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: /报表和对账/ }).click();
  }
  return page.locator(".reports-management");
}

async function queryLedger(panel: ReturnType<Page["locator"]>, memberId: string) {
  await panel.getByRole("button", { name: "账本报表", exact: true }).click();
  await panel.getByLabel("会员筛选 UUID", { exact: true }).fill(memberId);
  await panel.getByLabel("账本分组", { exact: true }).selectOption("entry_type");
  // Keep the bound in the browser's local timezone, not the Node worker's.
  const upperBound = await panel.evaluate(() => Date.now() + 60 * 60 * 1000);
  const to = await panel.page().evaluate(formatDateTimeLocal, upperBound);
  await panel.getByLabel("报表结束时间", { exact: true }).fill(to);
  await panel.getByRole("button", { name: "查询报表", exact: true }).click();
  const ledger = panel.getByRole("region", { name: "账本报表", exact: true });
  await expect(ledger.locator(".reports-summary").first()).toBeVisible();
  return ledger;
}

test("datetime-local report bounds omit only zero seconds", async ({ page }) => {
  // A scratch control checks the browser parser deterministically without
  // changing any report, clock, authentication or persisted financial data.
  await page.setContent('<label>Report bound<input type="datetime-local" step="1"></label>');
  const input = page.getByLabel("Report bound", { exact: true });
  for (const seconds of [0, 1]) {
    const timestamp = await page.evaluate(value => new Date(2026, 9, 7, 4, 15, value).getTime(), seconds);
    const value = await page.evaluate(formatDateTimeLocal, timestamp);
    expect(value).toBe(seconds === 0 ? "2026-10-07T04:15" : "2026-10-07T04:15:01");
    await input.fill(value);
    await expect(input).toHaveValue(value);
    expect(await input.evaluate(element => (element as HTMLInputElement).valueAsNumber)).toBe(Date.UTC(2026, 9, 7, 4, 15, seconds));
  }
});

function parseFixtureCsv(bytes: Buffer): string[][] {
  expect([...bytes.subarray(0, 3)]).toEqual([0xef, 0xbb, 0xbf]);
  return bytes.subarray(3).toString("utf8").trimEnd().split("\n").map((line) => line.split(","));
}

test("real all-group ledger CSV matches the committed Harbor report on desktop and mobile", async ({ page }, info) => {
  test.skip(!username || !password, "Provide TEST_EXPORT_ADMIN credentials or TEST_HARBOR fallback credentials");
  await signIn(page);
  const fixture = await createFixture(page);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const panel = await openReports(page, info.project.name);
  const reportRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/admin/reports/ledger?")) reportRequests.push(request.url());
  });
  const ledger = await queryLedger(panel, fixture.memberId);
  const displayedKeys = (await ledger.locator(".reports-table tbody tr td:first-child").allTextContents()).map((v) => v.trim()).sort();
  expect(displayedKeys).toEqual(["freeze", "recharge"]);
  await expect(ledger.locator(".reports-summary").first()).toContainText("25");
  const committedRequest = new URL(reportRequests.at(-1)!);
  const committedParams = committedRequest.searchParams;
  expect(committedParams.get("group_by")).toBe("entry_type");
  expect(committedParams.get("member_id")).toBe(fixture.memberId);

  // A draft edit is deliberately left unsubmitted: exporting must keep the displayed request scope.
  await panel.getByLabel("会员筛选 UUID", { exact: true }).fill("0199a000-0000-7000-8000-000000000001");
  const downloadResponsePromise = page.waitForResponse((r) => r.url().includes("/api/v1/admin/reports/ledger/export?"));
  const downloadPromise = page.waitForEvent("download");
  await ledger.getByRole("button", { name: "导出全部分组 CSV", exact: true }).click();
  const [download, exportResponse] = await Promise.all([downloadPromise, downloadResponsePromise]);
  expect(exportResponse.status()).toBe(200);
  const exportURL = new URL(exportResponse.url());
  expect(Object.fromEntries(exportURL.searchParams)).toEqual({
    from: committedParams.get("from"), to: committedParams.get("to"), group_by: "entry_type", member_id: fixture.memberId,
  });
  expect(exportResponse.request().headers()["x-brand-id"]).toBe(harbor);
  const responseHeaders = await exportResponse.allHeaders();
  expect(responseHeaders["x-report-kind"]).toBe("ledger");
  expect(responseHeaders["x-report-brand-id"]).toBe(harbor);
  expect(responseHeaders["x-report-group-count"]).toBe("2");

  const filePath = await download.path();
  expect(filePath).not.toBeNull();
  const bytes = await readFile(filePath!);
  expect(download.suggestedFilename()).toMatch(/^lottery-ledger-0199a000-0000-7000-8000-000000000002-\d{8}T\d{6}Z\.csv$/);
  expect(createHash("sha256").update(bytes).digest("hex")).toBe(responseHeaders["x-report-sha256"]);
  expect(Number(responseHeaders["content-length"])).toBe(bytes.length);

  const rows = parseFixtureCsv(bytes);
  const headers = ["record_type", "brand_id", "snapshot_at", "timezone", "from", "to", "group_by", "game_id", "member_id", "key", "label", "entry_count", "net_points", "recharge_points", "prize_credit_points", "prize_reversal_points", "refund_points", "account_count", "available_points", "frozen_points", "withdrawal_points", "total_points"];
  expect(rows).toHaveLength(5); // header, summary, balances, and both complete groups
  expect(rows[0]).toEqual(headers);
  expect(rows[1]![0]).toBe("summary");
  expect(rows[1]![11]).toBe("2");
  expect(rows[1]![12]).toBe("25");
  expect(rows[1]![13]).toBe("25");
  expect(rows[2]![0]).toBe("balances");
  expect(rows[2]![18]).toBe("20");
  expect(rows[2]![19]).toBe("5");
  expect(rows[2]![21]).toBe("25");
  expect(rows.slice(3).map((row) => row[9])).toEqual(displayedKeys);
  for (const row of rows.slice(1)) {
    expect(row![1]).toBe(harbor);
    expect(row![2]).toBe(responseHeaders["x-report-snapshot-at"]);
    expect(row![3]).toBeTruthy();
    expect(Date.parse(row![4]!)).toBe(Date.parse(committedParams.get("from")!));
    expect(Date.parse(row![5]!)).toBe(Date.parse(committedParams.get("to")!));
    expect(row![6]).toBe("entry_type");
    expect(row![8]).toBe(fixture.memberId);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await ledger.screenshot({ path: info.outputPath(`s7k-real-ledger-export-${info.project.name}.png`) });

  await panel.getByRole("button", { name: "投注报表", exact: true }).click();
  const betting = panel.getByRole("region", { name: "投注报表", exact: true });
  await expect(betting.locator(".reports-summary").first()).toBeVisible();
  const bettingResponsePromise = page.waitForResponse((r) => r.url().includes("/api/v1/admin/reports/betting/export?"));
  const bettingDownloadPromise = page.waitForEvent("download");
  await betting.getByRole("button", { name: "导出全部分组 CSV", exact: true }).click();
  const [bettingDownload, bettingResponse] = await Promise.all([bettingDownloadPromise, bettingResponsePromise]);
  expect(bettingResponse.status()).toBe(200);
  const bettingURL = new URL(bettingResponse.url());
  expect(Object.fromEntries(bettingURL.searchParams)).toEqual({
    from: committedParams.get("from"), to: committedParams.get("to"), group_by: "day", member_id: fixture.memberId,
  });
  const bettingHeaders = await bettingResponse.allHeaders();
  expect(bettingHeaders["x-report-kind"]).toBe("betting");
  expect(bettingHeaders["x-report-group-count"]).toBe("0");
  const bettingPath = await bettingDownload.path();
  const bettingBytes = await readFile(bettingPath!);
  expect(createHash("sha256").update(bettingBytes).digest("hex")).toBe(bettingHeaders["x-report-sha256"]);
  const bettingRows = parseFixtureCsv(bettingBytes);
  expect(bettingRows).toHaveLength(2); // Header and summary; no bets or pagination rows.
  expect(bettingRows[0]).toHaveLength(24);
  expect(bettingRows[1]![0]).toBe("summary");
  expect(bettingRows[1]![8]).toBe(fixture.memberId);
  expect(bettingRows[1]![11]).toBe("0");
  expect(errors).toEqual([]);
});

test("late real CSV response cannot download after tab round-trip, brand switch or logout", async ({ page }, info) => {
  test.skip(!username || !password || !platformUsername || !platformPassword, "Provide isolated brand and platform report export credentials");
  await signIn(page);
  const fixture = await createFixture(page);
  const logout = await page.request.post(`${adminBase}/auth/logout`, {
    headers: { Origin: uiBase, "Idempotency-Key": crypto.randomUUID() }, data: {},
  });
  expect(logout.status(), await logout.text()).toBe(200);
  // Only the ordinary brand operator funds the fixture. The genuine platform
  // reader below selects both brands without any user or financial write.
  await signIn(page, platformUsername, platformPassword);
  let panel = await openReports(page, info.project.name);
  let ledger = await queryLedger(panel, fixture.memberId);
  const brandSelect = page.getByLabel("选择真实后台品牌", { exact: true });
  const alternate = await brandSelect.locator("option").evaluateAll((options, excluded) => options.map((option) => (option as HTMLOptionElement).value).find((value) => value && value !== excluded), harbor);
  expect(alternate).toBeTruthy();
  const downloads: string[] = [];
  page.on("download", (download) => downloads.push(download.suggestedFilename()));
  await page.evaluate(() => {
    const pageWindow = window as Window & { __reportCsvDigestCount?: number };
    pageWindow.__reportCsvDigestCount = 0;
    const subtle = window.crypto.subtle;
    const originalDigest = subtle.digest.bind(subtle);
    Object.defineProperty(subtle, "digest", {
      configurable: true,
      value: async (algorithm: AlgorithmIdentifier, data: BufferSource) => {
        try { return await originalDigest(algorithm, data); }
        finally { pageWindow.__reportCsvDigestCount = (pageWindow.__reportCsvDigestCount ?? 0) + 1; }
      },
    });
  });

  async function delayNextExport() {
    let enter!: () => void;
    let release!: () => void;
    const reached = new Promise<void>((resolve) => { enter = resolve; });
    const held = new Promise<void>((resolve) => { release = resolve; });
    await page.route("**/api/v1/admin/reports/ledger/export?**", async (route) => {
      const actualResponse = await route.fetch();
      expect(actualResponse.status()).toBe(200);
      enter();
      await held;
      await route.fulfill({ response: actualResponse });
    }, { times: 1 });
    const response = page.waitForResponse((r) => r.url().includes("/api/v1/admin/reports/ledger/export?"));
    await ledger.getByRole("button", { name: "导出全部分组 CSV", exact: true }).click();
    await reached;
    return { release, response, digestCount: await page.evaluate(() => (window as Window & { __reportCsvDigestCount?: number }).__reportCsvDigestCount ?? 0) };
  }

  async function assertDiscarded(responsePromise: Promise<import("@playwright/test").Response>, digestCount: number) {
    expect((await responsePromise).status()).toBe(200);
    await expect.poll(() => page.evaluate(() => (window as Window & { __reportCsvDigestCount?: number }).__reportCsvDigestCount ?? 0)).toBe(digestCount + 1);
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(resolve, 0)))));
    expect(downloads).toEqual([]);
  }

  const tabRoundTrip = await delayNextExport();
  await panel.getByRole("button", { name: "投注报表", exact: true }).click();
  await panel.getByRole("button", { name: "账本报表", exact: true }).click();
  tabRoundTrip.release();
  await assertDiscarded(tabRoundTrip.response, tabRoundTrip.digestCount);

  const first = await delayNextExport();
  {
    await brandSelect.selectOption(alternate!);
    first.release();
    await assertDiscarded(first.response, first.digestCount);

    await brandSelect.selectOption(harbor);
    panel = page.locator(".reports-management");
    ledger = await queryLedger(panel, fixture.memberId);
    await page.unroute("**/api/v1/admin/reports/ledger/export?**");
    const second = await delayNextExport();
    await page.getByRole("button", { name: "退出登录", exact: true }).last().click();
    second.release();
    await assertDiscarded(second.response, second.digestCount);
  }
});
