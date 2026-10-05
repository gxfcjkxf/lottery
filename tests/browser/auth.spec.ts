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
  await page.getByRole("button", { name: /Sign out/ }).click();
  await expect(page).toHaveURL(/\/login$/);
  const me = await page.request.get("http://localhost:5173/api/v1/me");
  expect(me.status()).toBe(401);
});
