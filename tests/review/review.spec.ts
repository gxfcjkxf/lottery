import { expect, test } from "@playwright/test";

const userOrigin = "http://127.0.0.1:5183";
const adminOrigin = "http://127.0.0.1:5184";
const apiOrigin = "http://127.0.0.1:8085";
const auroraBrandId = "0199a000-0000-7000-8000-000000000001";
const harborBrandId = "0199a000-0000-7000-8000-000000000002";

function reviewPassword() {
  const password = process.env.TEST_REVIEW_ADMIN_PASSWORD;
  if (!password) throw new Error("TEST_REVIEW_ADMIN_PASSWORD is required.");
  return password;
}

test("real registration restores its cookie, starts at zero, and logs out", async ({
  page,
  context,
}) => {
  const username = `review_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`;
  const password = "用户测试密码"; // 18 UTF-8 bytes, fewer than 10 characters.
  await page.goto(`${userOrigin}/register`);
  await page.getByLabel("Choose a username or phone", { exact: true }).fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  expect(await page.getByLabel("Password", { exact: true }).evaluate((input: HTMLInputElement) => input.checkValidity())).toBe(true);
  await page.locator('.auth-form input[type="checkbox"]').nth(0).check();
  await page.locator('.auth-form input[type="checkbox"]').nth(1).check();
  const registrationResponse = page.waitForResponse(
    (response) => response.url() === `${userOrigin}/api/v1/auth/register`,
  );
  await page.getByRole("button", { name: /Continue/ }).click();
  expect((await registrationResponse).status()).toBe(201);
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByLabel("Username", { exact: true })).toHaveValue(username);

  const meBeforeResponse = await page.request.get(`${userOrigin}/api/v1/me`);
  expect(meBeforeResponse.status()).toBe(200);
  const meBefore = (await meBeforeResponse.json()).data;
  const walletResponse = await page.request.get(`${userOrigin}/api/v1/wallet`);
  expect(walletResponse.status()).toBe(200);
  expect((await walletResponse.json()).data.available_points).toBe("0");

  const cookies = await context.cookies();
  expect(cookies.some((cookie) => cookie.name.startsWith("lottery_user_") && cookie.httpOnly)).toBe(true);
  expect(await page.evaluate(() => Object.keys(localStorage).some((key) => /token|session/i.test(key)))).toBe(false);
  await page.reload();
  await expect(page.getByLabel("Username", { exact: true })).toHaveValue(username);
  const meAfterResponse = await page.request.get(`${userOrigin}/api/v1/me`);
  expect(meAfterResponse.status()).toBe(200);
  const meAfter = (await meAfterResponse.json()).data;
  expect(meAfter.user.id).toBe(meBefore.user.id);
  expect(meAfter.member.id).toBe(meBefore.member.id);

  await page.getByRole("button", { name: /Sign out/ }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect((await page.request.get(`${userOrigin}/api/v1/me`)).status()).toBe(401);
  await page.getByLabel("Username or phone", { exact: true }).fill(username);
  await page.getByLabel("Password", { exact: true }).fill(password);
  const loginResponse = page.waitForResponse(response => response.url() === `${userOrigin}/api/v1/auth/login`);
  await page.getByRole("button", { name: /Continue/ }).click();
  expect((await loginResponse).status()).toBe(200);
  await expect(page).toHaveURL(/\/account$/);
  const loggedIn = (await (await page.request.get(`${userOrigin}/api/v1/me`)).json()).data;
  expect(loggedIn.user.id).toBe(meBefore.user.id);
  expect(loggedIn.member.id).toBe(meBefore.member.id);
  await page.getByRole("button", { name: /Sign out/ }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("Aurora operator sees Aurora only and cannot read Harbor members", async ({ page }) => {
  const password = reviewPassword();
  const login = await page.request.post(`${adminOrigin}/api/v1/admin/auth/login`, {
    headers: { Origin: adminOrigin, "Idempotency-Key": crypto.randomUUID() },
    data: { identifier: "review_operator", password },
  });
  expect(login.status(), await login.text()).toBe(200);

  await page.goto(adminOrigin);
  await expect(page.getByLabel("选择真实后台品牌", { exact: true })).toBeVisible();
  await page.getByLabel("选择真实后台品牌", { exact: true }).selectOption(auroraBrandId);
  if (test.info().project.name === "mobile360") {
    await page.locator(".mobile-nav button").nth(1).click();
  } else {
    await page.locator(".side-nav").getByRole("button", { name: /用户和成员/ }).click();
  }
  await expect(page.getByRole("heading", { name: "用户和成员", exact: true })).toBeVisible();
  await expect(page.locator(".directory-brand-bar")).toContainText(auroraBrandId);

  const brandsResponse = await page.request.get(`${adminOrigin}/api/v1/admin/brands`);
  expect(brandsResponse.status()).toBe(200);
  const brands = (await brandsResponse.json()).data.items;
  expect(brands.map((brand: { id: string }) => brand.id)).toEqual([auroraBrandId]);
  expect(brands.map((brand: { id: string }) => brand.id)).not.toContain(harborBrandId);

  const harborMembers = await page.request.get(`${apiOrigin}/api/v1/admin/users?limit=1&offset=0`, {
    headers: { Origin: adminOrigin, "X-Brand-ID": harborBrandId },
  });
  expect(harborMembers.status()).toBe(403);
});
