import { test, expect, type Page, type TestInfo } from "@playwright/test";

const brandId = "0199a000-0000-7000-8000-000000000001";
async function navigate(page: Page, info: TestInfo, name: string) {
  if (info.project.name === "mobile") {
    if (name === "用户和成员")
      await page.locator(".mobile-nav button").nth(1).click();
    else {
      await page.locator(".mobile-nav button").last().click();
      await page
        .getByRole("navigation", { name: "全部管理页面" })
        .getByRole("button", { name: new RegExp(name) })
        .click();
    }
  } else
    await page
      .locator(".side-nav")
      .getByRole("button", { name: new RegExp(name) })
      .click();
}
async function login(page: Page, info: TestInfo) {
  await page.goto("http://localhost:5174");
  await page
    .getByLabel("账号", { exact: true })
    .fill(process.env.TEST_ADMIN_USERNAME!);
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.TEST_ADMIN_PASSWORD!);
  await page
    .getByRole("button", { name: "登录并加载真实成员", exact: true })
    .click();
  await navigate(page, info, "用户和成员");
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(brandId);
}
function unique() {
  return crypto.randomUUID().replaceAll("-", "").slice(0, 12);
}
async function fits(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth + 1,
    ),
  ).toBe(true);
}
test.beforeEach(() => {
  test.skip(
    !process.env.TEST_ADMIN_USERNAME || !process.env.TEST_ADMIN_PASSWORD,
    "Provide isolated test administrator credentials",
  );
});

test("operator creates real member; user personally accepts current brand terms", async ({
  page,
  context,
}, info) => {
  const name = `new_${unique()}`;
  await login(page, info);
  const provision = page.locator(".provision");
  await provision.getByLabel("用户名", { exact: false }).fill(name);
  await provision.getByLabel(/初始密码/).fill("member-test-only-password-2026");
  await provision
    .getByLabel(/创建原因/)
    .fill("browser provisioning verification");
  await provision
    .getByRole("button", { name: "创建成员", exact: true })
    .click();
  await expect(provision.getByRole("status")).toContainText("成员已创建");
  await expect(provision.getByRole("status")).toContainText("条款已接受: 否");
  await expect(provision.getByLabel(/初始密码/)).toHaveValue("");
  await fits(page);
  const user = await context.newPage();
  await user.goto("http://localhost:5173/login");
  await user.getByLabel("Username or phone", { exact: true }).fill(name);
  await user
    .getByLabel("Password", { exact: true })
    .fill("member-test-only-password-2026");
  await user.getByRole("button", { name: /Continue/ }).click();
  await expect(user.locator(".join-terms")).toBeVisible();
  await user.locator(".join-terms input[type=checkbox]").check();
  await user.getByRole("button", { name: /^Accept and join brand/ }).click();
  await expect(user).toHaveURL(/\/account$/);
  await expect(user.getByLabel("Username", { exact: true })).toHaveValue(name);
  await fits(user);
});

test("real role and account creation persist; new account has only assigned grants", async ({
  page,
  context,
}, info) => {
  const suffix = unique(),
    role = `Read only ${suffix}`,
    username = `staff_${suffix}`;
  await login(page, info);
  await navigate(page, info, "账号与权限");
  await page.getByLabel(/角色代码/).fill(`reader_${suffix}`);
  await page.getByLabel(/显示名称/).fill(role);
  await page.getByLabel("user.view.brand", { exact: true }).check();
  await page.getByLabel("brand.view.brand", { exact: true }).check();
  await page
    .getByLabel("变更原因", { exact: true })
    .fill("browser role creation");
  await page.getByRole("button", { name: "创建角色", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("角色已创建");
  await page.reload();
  // The shell does not persist the selected page; return to the live role directory.
  await page
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(brandId);
  await navigate(page, info, "账号与权限");
  await expect(
    page.locator(".access-page").getByText(role, { exact: true }).first(),
  ).toBeVisible();
  await page.getByRole("tab", { name: "管理员账号", exact: true }).click();
  await page.getByLabel(/用户名（最多/).fill(username);
  await page.getByLabel(/初始密码（16/).fill("staff-test-only-password-2026");
  await page.getByLabel(new RegExp(role)).check();
  await page
    .getByLabel("变更原因", { exact: true })
    .fill("browser account creation");
  await page.getByRole("button", { name: "创建管理员", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("管理员账号已创建");
  await expect(page.locator(".page-footer")).toContainText(
    "账号与角色变更为真实操作",
  );
  await fits(page);
  await page.screenshot({
    path: info.outputPath("account-management.png"),
    fullPage: true,
  });
  const auth = await page.request.post(
    "http://localhost:5174/api/v1/admin/auth/login",
    {
      headers: {
        Origin: "http://localhost:5174",
        "Idempotency-Key": crypto.randomUUID(),
      },
      data: { identifier: username, password: "staff-test-only-password-2026" },
    },
  );
  expect(auth.status()).toBe(200);
  const me = await page.request.get("http://localhost:5174/api/v1/admin/me");
  const account = (await me.json()).data.account;
  expect(account.permissions_by_brand[brandId].sort()).toEqual([
    "brand.view.brand",
    "user.view.brand",
  ]);
  const denied = await page.request.get(
    "http://localhost:5174/api/v1/admin/roles",
    { headers: { "X-Brand-ID": brandId } },
  );
  expect(denied.status()).toBe(403);
  // A fresh page loads the newly authenticated read-only account from its cookie.
  const staff = await context.newPage();
  await staff.goto("http://localhost:5174");
  await navigate(staff, info, "用户和成员");
  await staff
    .getByLabel("选择真实后台品牌", { exact: true })
    .selectOption(brandId);
  await expect(staff.locator(".provision")).toContainText(
    "没有此品牌的成员创建权限",
  );
  await fits(staff);
});
