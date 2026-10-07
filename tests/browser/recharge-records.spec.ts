import { test, expect } from "@playwright/test";
import { rememberAdminSession, restoreAdminSession } from "./support/admin-session";

test("real member recharge records reflect manual confirmation and cancellation without exposing staff data or submitting payments", async ({ page, context }, info) => {
  test.skip(!process.env.TEST_HARBOR_ADMIN_USERNAME || !process.env.TEST_HARBOR_ADMIN_PASSWORD, "Provide an owned Harbor administrator");
  test.setTimeout(60_000); page.setDefaultTimeout(10_000);
  const adminOrigin = process.env.TEST_ADMIN_ORIGIN ?? "http://localhost:5174", userOrigin = process.env.TEST_USER_ORIGIN ?? "http://harbor.localhost:5173";
  const brand = "0199a000-0000-7000-8000-000000000002", username = process.env.TEST_HARBOR_ADMIN_USERNAME!, uid = () => crypto.randomUUID();
  const headers = { Origin: adminOrigin, "X-Brand-ID": brand };
  if (!await restoreAdminSession(context, username, brand, adminOrigin)) {
    const response = await page.request.post(`${adminOrigin}/api/v1/admin/auth/login`, { headers: { ...headers, "Idempotency-Key": uid() }, data: { identifier: username, password: process.env.TEST_HARBOR_ADMIN_PASSWORD } });
    expect(response.status()).toBe(200);
  }
  rememberAdminSession(username, await context.cookies(`${adminOrigin}/api/v1/admin/me`), adminOrigin);
  const admin = async (path: string, body?: unknown, status = 200) => {
    const response = await page.request.fetch(`${adminOrigin}/api/v1/admin${path}`, { method: body === undefined ? "GET" : "POST", headers: { ...headers, ...(body === undefined ? {} : { "Idempotency-Key": uid() }) }, ...(body === undefined ? {} : { data: body }) });
    expect(response.status(), await response.text()).toBe(status); return (await response.json()).data;
  };
  await page.addInitScript(() => localStorage.setItem("luma-language", "en"));
  await page.route(`${userOrigin}/api/**`, async route => {
    const response = await route.fetch({ url: route.request().url().replace("harbor.localhost", "localhost"), headers: { ...(await route.request().allHeaders()), host: new URL(userOrigin).host } });
    await route.fulfill({ response });
  });
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  await page.goto(`${userOrigin}/register`);
  await page.getByLabel("Choose a username or phone", { exact: true }).fill(`recharge_${uid().replaceAll("-", "").slice(0, 12)}`);
  await page.getByLabel("Password", { exact: true }).fill("recharge-owned-user-password-2026");
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check(); await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registered = page.waitForResponse(response => response.url().endsWith("/auth/register") && response.request().method() === "POST");
  await page.getByRole("button", { name: /Continue/ }).click(); const response = await registered; expect(response.status()).toBe(201);
  const member = (await response.json()).data.member;
  await expect(page).toHaveURL(/\/account$/);
  await page.goto(`${userOrigin}/recharge`);
  const panel = page.locator(".recharge-page");
  await expect(panel).toContainText("No recharge records yet.");
  await expect(panel.getByRole("button", { name: /Submit|Pay|Request a top-up/ })).toHaveCount(0);
  const create = (points: string) => admin("/recharges", { member_id: member.id, points, proof_reference: "private-staff-proof", remark: "private-staff-remark", reason: "owned member recharge browser fixture" }, 201);
  const pending = await create("35"), confirmed = await create("14"), cancelled = await create("20");
  await admin(`/recharges/${confirmed.id}/confirm`, { version: confirmed.version, reason: "private confirmation reason" });
  await admin(`/recharges/${cancelled.id}/cancel`, { version: cancelled.version, reason: "private cancellation reason" });
  const beforeWallet = await admin(`/wallets/${member.id}`), beforeLedger = await admin(`/wallets/${member.id}/ledger`);
  expect(beforeWallet.recharge_points).toBe("14"); expect(beforeLedger.items).toHaveLength(1);
  const businessWrites: string[] = [];
  page.on("request", request => { if (new URL(request.url()).pathname.startsWith("/api/") && !["GET", "HEAD", "OPTIONS"].includes(request.method())) businessWrites.push(request.method()); });
  await panel.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(panel.locator(".recharge-record")).toHaveCount(3);
  await expect(panel).toContainText("Pending"); await expect(panel).toContainText("Confirmed"); await expect(panel).toContainText("Cancelled");
  await panel.locator(".recharge-record").filter({ hasText: confirmed.id }).getByRole("button", { name: "Details", exact: true }).click();
  await expect(panel.locator(".recharge-detail")).toContainText(confirmed.id);
  const safe = await page.evaluate(async () => { const r = await fetch("/api/v1/recharges", { credentials: "include" }); return { status: r.status, body: await r.json() }; });
  expect(safe.status).toBe(200); expect(safe.body.data.member_id).toBe(member.id);
  expect(safe.body.data.items.map((item: { id: string }) => item.id).sort()).toEqual([pending.id, confirmed.id, cancelled.id].sort());
  const wire = JSON.stringify(safe.body);
  for (const privateField of ["account_id", "created_by", "confirmed_by", "audit_log_id", "proof_reference", "remark", "private-staff-proof", "private-staff-remark"]) expect(wire).not.toContain(privateField);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await panel.screenshot({ path: info.outputPath("member-recharge-records.png") });
  expect(businessWrites).toEqual([]);
  expect(await admin(`/wallets/${member.id}`)).toEqual(beforeWallet); expect(await admin(`/wallets/${member.id}/ledger`)).toEqual(beforeLedger);
  // A new browser sign-in cookie must not let the old mounted member scope
  // display the new account's data, even when both belong to the same brand.
  const switched = await page.evaluate(async (identifier: string) => {
    const r = await fetch("/api/v1/auth/register", { method: "POST", credentials: "include", headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() }, body: JSON.stringify({ username: identifier, password: "recharge-switch-password-2026", privacy_policy_version: "dev-1", service_terms_version: "dev-1" }) });
    return { status: r.status, body: await r.json() };
  }, `recharge_other_${uid().replaceAll("-", "").slice(0, 10)}`);
  expect(switched.status).toBe(201); expect(switched.body.data.member.id).not.toBe(member.id);
  await panel.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(panel).toContainText("Sign in to view your recharge records."); await expect(panel.locator(".recharge-record")).toHaveCount(0);
  await expect(panel).not.toContainText(confirmed.id);
  await page.reload(); await expect(panel).toContainText("No recharge records yet."); await expect(panel).not.toContainText(confirmed.id);
  expect(errors).toEqual([]); await page.unrouteAll({ behavior: "wait" });
});
