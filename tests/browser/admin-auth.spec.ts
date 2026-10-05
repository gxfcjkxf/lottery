import { test, expect } from "@playwright/test";

test("admin directory writes a real brand member and kick revokes its session", async ({
  page,
}, testInfo) => {
  test.skip(
    !process.env.TEST_ADMIN_USERNAME || !process.env.TEST_ADMIN_PASSWORD,
    "Provide isolated test administrator credentials",
  );
  const name = `ops_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`;
  const registration = await page.request.post(
    "http://localhost:5173/api/v1/auth/register",
    {
      headers: {
        Origin: "http://localhost:5173",
        "Idempotency-Key": crypto.randomUUID(),
      },
      data: {
        username: name,
        password: "test-only-member-password-123",
        privacy_policy_version: "dev-1",
        service_terms_version: "dev-1",
      },
    },
  );
  expect(registration.status()).toBe(201);
  const user = (await registration.json()).data;
  await page.goto("http://localhost:5174");
  if (testInfo.project.name === "mobile")
    await page.locator(".mobile-nav button").nth(1).click();
  else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /用户和成员/ })
      .click();
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.TEST_ADMIN_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.TEST_ADMIN_PASSWORD!);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption("0199a000-0000-7000-8000-000000000001");
  const row = page
    .locator(".member-directory-table tbody tr")
    .filter({ hasText: name });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "编辑", exact: true }).click();
  const dialog = page
    .getByRole("dialog")
    .filter({ has: page.getByRole("heading", { name: "编辑成员状态和备注" }) });
  await dialog.getByRole("combobox").selectOption("frozen");
  await dialog.getByLabel("备注", { exact: true }).fill("browser verification");
  await dialog.getByLabel(/操作原因/).fill("browser freeze verification");
  await dialog.getByRole("button", { name: "保存到后台", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(row).toContainText("冻结");
  await expect(row).toContainText("browser verification");
  await row.getByRole("button", { name: "踢出", exact: true }).click();
  const kick = page
    .getByRole("dialog")
    .filter({ has: page.getByRole("heading", { name: "踢出品牌会话" }) });
  await kick.getByLabel(/原因/).fill("browser kick verification");
  await kick.getByRole("button", { name: /确认踢出/ }).click();
  await expect(kick).not.toBeVisible();
  const me = await page.request.get("http://localhost:5173/api/v1/me", {
    headers: { Authorization: `Bearer ${user.access_token}` },
  });
  expect(me.status()).toBe(401);
  const cross = await page.request.get(
    "http://localhost:5174/api/v1/admin/users",
    { headers: { "X-Brand-ID": "0199a000-0000-7000-8000-000000000002" } },
  );
  expect(cross.status()).toBe(403);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
});
