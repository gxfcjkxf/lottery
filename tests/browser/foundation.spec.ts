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
    "提现、佣金管理、域名管理及新建品牌尚未实现",
  );
  await expect(page.locator(".banner-copy")).toContainText(
    "各页面会标出真实接口与演示边界",
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

test("legacy demo game and stored demo orders cannot become real bets", async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem(
      "luma-demo-orders",
      JSON.stringify([
        { id: "DEMO-old", points: "123", status: "Placed (demo)" },
      ]),
    );
  });
  const invalidCatalog = page.waitForResponse((r) =>
    r.url().endsWith("/api/v1/games/daily-3"),
  );
  await page.goto("http://localhost:5173/games/daily-3/bet");
  expect((await invalidCatalog).status()).toBe(400);
  await expect(page.getByTestId("bet-selection-page")).toBeVisible();
  await expect(page.getByRole("alert").first()).toBeVisible();
  await expect(page.getByTestId("preview-button")).toHaveCount(0);
  await noHorizontalOverflow(page);
  await page.goto("http://localhost:5173/orders");
  await expect(page.getByText("DEMO-old", { exact: true })).toHaveCount(0);
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("luma-demo-orders") || "[]")[0].points,
    ),
  ).toBe("123");
});
