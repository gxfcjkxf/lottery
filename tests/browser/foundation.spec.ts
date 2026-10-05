import { test, expect, type Page } from "@playwright/test";

async function noHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1,
    ),
  ).toBe(true);
}

test("user shell loads live persisted brand context and fits viewport", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("http://localhost:5173");
  await expect(page.locator(".footer-online")).toHaveText("Service connected");
  await expect(page.locator(".page-footer")).toContainText(
    "Prototype · demo data",
  );
  await expect(page.locator(".brand").first()).toContainText("Aurora");
  await noHorizontalOverflow(page);
  expect(errors).toEqual([]);
});

test("admin shell distinguishes live context from demo data", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("http://localhost:5174");
  await expect(page.locator(".context-strip")).toContainText("Aurora · aurora");
  await expect(
    page.getByRole("heading", { name: /早上好，林岚/ }),
  ).toBeVisible();
  await expect(page.locator(".banner-copy")).toContainText(
    "提现、域名主题及标为“演示”的页面不会写入后台。",
  );
  await expect(page.locator(".banner-copy")).toContainText(
    "规则版本流程已接入真实",
  );
  await noHorizontalOverflow(page);
  expect(errors).toEqual([]);
});

test("API brands remain isolated and spoofed brand header is ignored", async ({
  request,
}) => {
  const origin = "http://localhost:5173";
  const main = await request.get(`${origin}/api/v1/context`, {
    headers: { "X-Brand-ID": "harbor" },
  });
  expect(main.ok()).toBe(true);
  const a = (await main.json()).data.brand;
  expect(a.code).toBe("aurora");
  const other = await request.get(`${origin}/api/v1/b/harbor/context`);
  expect(other.ok()).toBe(true);
  const b = (await other.json()).data.brand;
  expect(b.code).toBe("harbor");
  expect(b.id).not.toBe(a.id);
  const missing = await request.get(`${origin}/api/v1/b/unknown/context`);
  expect(missing.status()).toBe(404);
});

test("repeated positional digits survive demo confirmation and cancellation", async ({
  page,
}) => {
  await page.goto("http://localhost:5173/games/daily-3/bet");
  await page
    .getByRole("button", { name: "Choose digit 1 for position 1", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Choose digit 2 for position 2", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Choose digit 1 for position 3", exact: true })
    .click();
  await noHorizontalOverflow(page);
  await page.getByRole("button", { name: /Review selection/ }).click();
  await expect(page).toHaveURL(/\/bet\/confirm$/);
  await expect(page.locator(".confirm-balls span")).toHaveText(["1", "2", "1"]);
  await page.getByRole("button", { name: /Confirm picks/ }).click();
  await expect(page).toHaveURL(/\/orders\/DEMO-/);
  await page
    .getByRole("button", { name: "Cancel demo order", exact: true })
    .click();
  await expect(
    page.getByText("Cancelled (demo)", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("luma-demo-orders") || "[]")[0].points,
    ),
  ).toBe("1");
});
