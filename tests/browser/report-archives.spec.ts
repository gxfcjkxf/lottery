import { createHash } from "node:crypto";
import { expect, test, type APIResponse, type Page } from "@playwright/test";

const origin = process.env.TEST_ARCHIVE_ORIGIN;
const apiOrigin = process.env.TEST_ARCHIVE_API_ORIGIN;
const browserAdmin = `${origin}/api/v1/admin`;
const admin = browserAdmin;
const allowedBrowserOrigin = /^http:\/\/localhost:(?:5174|15292)$/;
const allowedAPIOrigin = /^http:\/\/127\.0\.0\.1:(?:8080|15291)$/;
const brand = "0199a000-0000-7000-8000-000000000002";
const username = process.env.TEST_ARCHIVE_ADMIN_USERNAME;
const password = process.env.TEST_ARCHIVE_ADMIN_PASSWORD;
const viewport = process.env.REPORT_ARCHIVE_VIEWPORT;
const confirmation = process.env.REPORT_ARCHIVE_FIXTURE_CONFIRM;

type Archive = {
  id: string;
  brand_id: string;
  revision: number;
  previous_id: string | null;
  reason: string;
  payload_sha256: string;
  snapshot: { wallet_snapshot: { balances: { available_points: string } } };
};
type Wallet = { available_points: string; frozen_points: string; withdrawal_points: string };
type Ledger = { items: Array<{ id: string }> };

async function data<T>(response: Pick<APIResponse, "text" | "status">, status = 200): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  const envelope = JSON.parse(body);
  expect(envelope.success).toBe(true);
  return envelope.data as T;
}

async function navigate(page: Page, project: string, destination: string): Promise<void> {
  if (project === "mobile" && destination === "工作台|Dashboard") {
    await page.locator(".mobile-nav").getByRole("button", { name: new RegExp(destination) }).click();
  } else if (project === "mobile") {
    await page.locator(".mobile-nav button").nth(4).click();
    await page.locator(".mobile-more-menu").getByRole("button", { name: new RegExp(destination) }).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: new RegExp(destination) }).click();
  }
}

async function confirmArchive(page: Page) {
  const dialog = page.getByTestId("archive-review");
  await expect(dialog).toBeVisible();
  await dialog.getByTestId("archive-confirm").check();
  await dialog.getByTestId("archive-submit").click();
}

test("real report archives recover a lost committed receipt and keep old versions immutable", async ({ page, context }, info) => {
  test.setTimeout(60_000);
  page.setDefaultTimeout(10_000);
  expect(confirmation).toBe("owned_synthetic_database");
  expect(viewport).toBe(info.project.name);
  expect(allowedBrowserOrigin.test(origin ?? "")).toBe(true);
  expect(allowedAPIOrigin.test(apiOrigin ?? "")).toBe(true);
  expect(username).toBe(`archive_browser_${info.project.name}`);
  expect(password).toBeTruthy();

  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  let headers: Record<string, string> = { "X-Brand-ID": brand };
  const get = async <T>(path: string) => data<T>(await page.request.get(`${admin}${path}`, { headers }));
  const post = async <T>(path: string, body: unknown, status = 200) => data<T>(await page.request.post(`${admin}${path}`, {
    headers: { Origin: origin!, ...headers, "Idempotency-Key": crypto.randomUUID() }, data: body,
  }), status);

  await data(await page.request.post(`${browserAdmin}/auth/login`, {
    headers: { Origin: origin!, "Idempotency-Key": crypto.randomUUID() },
    data: { identifier: username, password },
  }));
  // The admin cookie is path-scoped to /api/v1/admin. Use the same-origin
  // browser proxy for all setup requests; never copy it to a separate host.
  expect((await context.cookies(`${browserAdmin}/me`)).some(cookie => cookie.name === "lottery_admin")).toBe(true);
  const { account } = await get<{ account: { id: string; permissions_by_brand: Record<string, string[]> } }>("/me");
  expect(account.permissions_by_brand[brand]).toEqual(expect.arrayContaining([
    "report_archive.view.brand", "report_archive.create.brand", "report_archive.download.brand",
  ]));
  const initialArchives = await get<{ items: Archive[] }>("/report-archives?limit=100&offset=0");
  expect(initialArchives.items).toEqual([]);

  const member = await post<{ member_id: string }>("/users", {
    username: `archive_${info.project.name}_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`,
    password: "owned-report-archive-member-password-2026",
    display_name: "Owned report archive member",
    reason: "Create the real member used by the report archive browser flow",
  }, 201);
  const createRecharge = async (points: string, remark: string) => {
    const pending = await post<{ id: string; version: number }>("/recharges", {
      member_id: member.member_id, points, proof_reference: remark, remark,
      reason: `Create genuine ${points}-point report archive fixture recharge`,
    }, 201);
    await post(`/recharges/${pending.id}/confirm`, {
      version: pending.version, reason: `Confirm genuine ${points}-point report archive fixture recharge`,
    });
  };
  await createRecharge("37", "Initial real report archive funding");
  const wallet = () => get<Wallet>(`/wallets/${member.member_id}`);
  const ledger = () => get<Ledger>(`/wallets/${member.member_id}/ledger?limit=100&offset=0`);
  const economicState = async () => JSON.stringify({ wallet: await wallet(), ledger: await ledger() });
  expect((await wallet()).available_points).toBe("37");
  const beforeFirstArchive = await economicState();

  await page.goto(origin!);
  await page.getByLabel(/选择真实后台品牌|Select live admin brand/, { exact: true }).selectOption(brand);
  await navigate(page, info.project.name, "日月归档|Daily and monthly archives");
  let panel = page.locator(".archives");
  await expect(panel.getByRole("heading", { name: "日月归档", exact: true })).toBeVisible();
  await expect(panel.getByTestId("archive-list")).toBeVisible();

  const periodKey = "2026-09";
  const reason = "Committed monthly snapshot after a genuine lost browser response";
  const writes: Array<{ body: string | null; key: string | undefined; actor: string | undefined }> = [];
  let first!: Archive;
  await page.route("**/api/v1/admin/report-archives", async route => {
    if (route.request().method() !== "POST") return route.continue();
    writes.push({
      body: route.request().postData(),
      key: route.request().headers()["idempotency-key"],
      actor: route.request().headers()["x-report-archive-actor-id"],
    });
    const response = await route.fetch();
    const committed = await data<Archive>(response, 201);
    expect(writes.at(-1)?.actor).toBe(account.id);
    if (writes.length === 1) {
      first = committed;
      await route.abort("failed");
    } else {
      expect(writes[1]).toEqual(writes[0]);
      expect(committed).toEqual(first);
      await route.fulfill({ response });
    }
  });

  const fillCreate = async (expectedRevision: number, actionReason: string) => {
    await panel.getByTestId("archive-kind").selectOption("monthly");
    await panel.getByTestId("archive-period-key").fill(periodKey);
    await panel.getByTestId("archive-reason").fill(actionReason);
    await panel.getByTestId("archive-expected-revision").fill(String(expectedRevision));
    await panel.getByTestId("archive-review-button").click();
  };

  await fillCreate(0, reason);
  await confirmArchive(page);
  await expect(panel.getByRole("alert")).toContainText("写入结果未知");
  await expect(panel.getByTestId("archive-unknown")).toBeVisible();
  expect(writes).toHaveLength(1);
  expect(writes[0]?.key).toMatch(/^[0-9a-f-]{36}$/i);
  expect(JSON.parse(writes[0]!.body!)).toEqual({ kind: "monthly", period_key: periodKey, expected_revision: 0, reason });
  expect(first.revision).toBe(1);
  expect(first.previous_id).toBeNull();
  expect(first.snapshot.wallet_snapshot.balances.available_points).toBe("37");
  expect(await economicState()).toBe(beforeFirstArchive);
  const downloadEvents: string[] = [];
  page.on("download", download => downloadEvents.push(download.suggestedFilename()));

  await panel.getByTestId("archive-refresh").click();
  await expect(panel.getByTestId("archive-unknown")).toBeVisible();
  expect(downloadEvents).toEqual([]);
  await navigate(page, info.project.name, "工作台|Dashboard");
  await navigate(page, info.project.name, "日月归档|Daily and monthly archives");
  panel = page.locator(".archives");
  await expect(panel.getByTestId("archive-replay")).toBeVisible();
  await panel.getByTestId("archive-replay").click();
  await confirmArchive(page);
  await expect(panel.getByTestId("archive-receipt")).toContainText(first.id);
  expect(writes).toHaveLength(2);
  expect(writes[1]).toEqual(writes[0]);
  expect(await economicState()).toBe(beforeFirstArchive);
  await page.unroute("**/api/v1/admin/report-archives");

  await createRecharge("11", "Real additional report archive funding");
  expect((await wallet()).available_points).toBe("48");
  const beforeSecondArchive = await economicState();
  const secondReason = "Second immutable monthly snapshot after genuine recharge";
  await fillCreate(1, secondReason);
  const secondResponse = page.waitForResponse(response => response.request().method() === "POST" && response.url() === `${browserAdmin}/report-archives`);
  await confirmArchive(page);
  const second = await data<Archive>(await secondResponse, 201);
  await expect(panel.getByTestId("archive-receipt")).toContainText(second.id);
  expect(second.revision).toBe(2);
  expect(second.previous_id).toBe(first.id);
  expect(second.snapshot.wallet_snapshot.balances.available_points).toBe("48");
  expect(await economicState()).toBe(beforeSecondArchive);
  await page.unroute("**/api/v1/admin/report-archives");

  await panel.getByTestId("archive-list").getByRole("button", { name: /2026-09 · monthly · v1/ }).click();
  await expect(panel.getByTestId("archive-detail")).toContainText(first.id);
  await expect(panel.getByTestId("archive-detail")).toContainText("37");
  const oldRecord = await get<Archive>(`/report-archives/${first.id}`);
  expect(oldRecord).toEqual(first);
  const bodyPromise = page.waitForResponse(response => response.url() === `${browserAdmin}/report-archives/${first.id}/download`);
  const downloadPromise = page.waitForEvent("download");
  await panel.getByTestId("archive-download").click();
  const [download, downloadResponse] = await Promise.all([downloadPromise, bodyPromise]);
  expect(downloadResponse.status()).toBe(200);
  const bytes: Buffer[] = [];
  const stream = await download.createReadStream();
  expect(stream).not.toBeNull();
  for await (const chunk of stream!) bytes.push(Buffer.from(chunk));
  const payload = Buffer.concat(bytes);
  const digest = createHash("sha256").update(payload).digest("hex");
  const responseHeaders = await downloadResponse.allHeaders();
  expect(digest).toBe(responseHeaders["x-content-sha256"]);
  expect(digest).toBe(first.payload_sha256);
  expect(Number(responseHeaders["content-length"])).toBe(payload.length);
  expect(download.suggestedFilename()).toBe(`report-archive-${first.id}-v1.json`);
  expect(JSON.parse(payload.toString("utf8")).wallet_snapshot.balances.available_points).toBe("37");
  expect(downloadEvents).toHaveLength(1);
  expect(await economicState()).toBe(beforeSecondArchive);

  const configuredWidth = page.viewportSize()!.width;
  await expect.poll(() => page.evaluate(width => document.documentElement.scrollWidth <= width && innerWidth <= width, configuredWidth)).toBe(true);
  await panel.screenshot({ path: info.outputPath(`report-archives-${info.project.name}.png`) });
  await page.screenshot({ path: info.outputPath(`report-archives-${info.project.name}-viewport.png`) });
  await page.getByTestId("admin-language").selectOption("en");
  await expect(panel.getByRole("heading", { name: "Daily and monthly archives", exact: true })).toBeVisible();
  expect(await economicState()).toBe(beforeSecondArchive);
  expect(errors).toEqual([]);
});
