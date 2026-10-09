import { test, expect } from "@playwright/test";

// Only desktop writes this shared Harbor fixture. Mobile verifies the same live
// editor's responsive layout without racing its configuration versions.
test("Harbor authentication editor loads real settings; desktop enables captcha", async ({
  page,
  context,
}, info) => {
  test.skip(
    !process.env.TEST_HARBOR_ADMIN_USERNAME ||
      !process.env.TEST_HARBOR_ADMIN_PASSWORD,
    "Provide isolated Harbor test administrator credentials",
  );
  await page.goto("http://localhost:5174");
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.TEST_HARBOR_ADMIN_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.TEST_HARBOR_ADMIN_PASSWORD!);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  const brandId = "0199a000-0000-7000-8000-000000000002";
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(brandId);
  if (info.project.name === "mobile") {
    await page.locator(".mobile-nav button").last().click();
    await page
      .getByRole("navigation", { name: "全部管理页面" })
      .getByRole("button", { name: /品牌和域名/ })
      .click();
  } else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: /品牌和域名/ })
      .click();
  const settings = page.locator(".auth-settings");
  await expect(settings.locator(".version")).toContainText("配置版本");
  await expect(settings.locator(".legal")).toContainText("dev-1");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
  if (info.project.name === "mobile") return;
  const original = await settings.getByLabel(/启用验证码/).isChecked();
  try {
    await settings.getByLabel(/启用验证码/).check();
    await settings
      .getByLabel("变更原因", { exact: true })
      .fill("browser captcha enable verification");
    await settings
      .getByRole("button", { name: "保存并立即生效", exact: true })
      .click();
    await expect(settings.getByRole("status")).toContainText(
      "配置已保存并立即生效",
    );
    await expect(settings.locator(".legal")).toContainText("dev-1");
    const user = await context.newPage();
    await user.goto("http://harbor.localhost:5173/login");
    await expect(user.locator(".brand").first()).toContainText("Harbor");
    await expect(
      user.getByAltText("Verification code image", { exact: true }),
    ).toBeVisible();
    await expect(
      user.getByLabel("Enter the characters shown", { exact: true }),
    ).toBeVisible();
    // Chromium resolves *.localhost to loopback; Node's API client does not.
    // Use the page's actual same-origin browser request, without editing DNS.
    const denied = await user.evaluate(async () => {
      const response = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": crypto.randomUUID(),
        },
        body: JSON.stringify({
          identifier: "not_a_real_user",
          password: "test-only-invalid-password-2026",
        }),
      });
      return { status: response.status, body: await response.json() };
    });
    expect(denied.status).toBe(400);
    expect(denied.body.error.code).toBe("CAPTCHA_INVALID");
    await user.close();
  } finally {
    await settings
      .getByRole("button", { name: "重新读取", exact: true })
      .click();
    await expect(settings.locator(".form")).toBeVisible();
    await settings.getByLabel(/启用验证码/).setChecked(original);
    await settings
      .getByLabel("变更原因", { exact: true })
      .fill("restore isolated browser fixture");
    await settings
      .getByRole("button", { name: "保存并立即生效", exact: true })
      .click();
    await expect(settings.getByRole("status")).toContainText(
      "配置已保存并立即生效",
    );
  }
});
