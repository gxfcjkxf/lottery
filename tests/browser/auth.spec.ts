import { test, expect } from "@playwright/test";

test("real registration, cookie restoration, one-time profile fill and logout", async ({
  page,
  context,
}) => {
  const username = `web_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`;
  await page.goto("http://localhost:5173/register");
  await page
    .getByLabel("Choose a username or phone", { exact: true })
    .fill(username);
  await page
    .getByLabel("Password", { exact: true })
    .fill("test-only-password-123");
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  await page.getByRole("button", { name: /Continue/ }).click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByLabel("Username", { exact: true })).toHaveValue(
    username,
  );
  const cookies = await context.cookies();
  expect(
    cookies.some(
      (cookie) =>
        cookie.name.startsWith("lottery_user_") &&
        cookie.httpOnly &&
        cookie.sameSite === "Strict",
    ),
  ).toBe(true);
  expect(
    await page.evaluate(() =>
      Object.keys(localStorage).some((key) => /token|session/i.test(key)),
    ),
  ).toBe(false);
  await page.reload();
  await expect(page.getByLabel("Username", { exact: true })).toHaveValue(
    username,
  );
  await page
    .getByLabel("Phone", { exact: true })
    .fill(`+63${Date.now().toString().slice(-10)}`);
  await page.getByRole("button", { name: /Save profile/ }).click();
  await expect(page.getByLabel("Phone", { exact: true })).toBeDisabled();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  // The worker materializes exactly one real welcome event for registration.
  await expect.poll(async () => {
    const response = await page.request.get("http://localhost:5173/api/v1/notifications?limit=20&offset=0");
    expect(response.status()).toBe(200);
    const payload = await response.json();
    return payload.data.items.length;
  }).toBe(1);
  await page.goto("http://localhost:5173/notifications");
  const inbox = page.locator(".notifications-panel");
  await expect(inbox.getByRole("heading", {name:"Welcome", exact:true})).toBeVisible();
  await expect(page.locator('.nav-dot[aria-label="1 unread notifications"]')).toBeVisible();
  const intents: Array<{key:string,body:string|null}> = [];
  await page.route("**/notifications/read", async route => {
    intents.push({key:route.request().headers()["idempotency-key"]!,body:route.request().postData()});
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    if (intents.length === 1) await route.abort("failed"); // commit succeeded, receipt was lost
    else await route.fulfill({response});
  });
  await inbox.getByRole("button", {name:"Mark this page read",exact:true}).click();
  await expect(inbox.getByRole("button",{name:"Retry the same request",exact:true})).toBeVisible();
  await inbox.getByRole("button",{name:"Reload",exact:true}).click();
  await expect(inbox.locator(".notification-card.read-card")).toHaveCount(1);
  // A read-only refresh is not evidence that the earlier user intent was acknowledged.
  await expect(inbox.getByRole("button",{name:"Retry the same request",exact:true})).toBeVisible();
  await inbox.getByRole("button",{name:"Retry the same request",exact:true}).click();
  await expect(inbox.getByRole("button",{name:"Retry the same request",exact:true})).toHaveCount(0);
  expect(intents).toHaveLength(2);
  expect(intents[0]).toEqual(intents[1]);
  await page.unroute("**/notifications/read");
  await page.reload();
  await expect(inbox.locator(".notification-card.read-card")).toHaveCount(1);
  await expect(page.locator(".nav-dot")).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  await page.screenshot({path: test.info().outputPath("notifications.png"), fullPage:true});
  await page.goto("http://localhost:5173/account");
  await page.getByRole("button", { name: /Sign out/ }).click();
  await expect(page).toHaveURL(/\/login$/);
  const me = await page.request.get("http://localhost:5173/api/v1/me");
  expect(me.status()).toBe(401);
  await page.goto("http://localhost:5173/notifications");
  await expect(inbox.getByRole("alert")).toContainText("Please sign in again");
  await expect(inbox.locator(".notification-card")).toHaveCount(0);
  await expect(page.locator(".nav-dot")).toHaveCount(0);
});
