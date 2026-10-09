import { defineConfig, devices } from "@playwright/test";

const password = process.env.TEST_REVIEW_ADMIN_PASSWORD;
if (!password) {
  throw new Error(
    "TEST_REVIEW_ADMIN_PASSWORD is required to run the real review smoke suite.",
  );
}

const executablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
if (!executablePath) {
  throw new Error(
    "PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH is required to run the review smoke suite.",
  );
}

export default defineConfig({
  testDir: "./tests/review",
  fullyParallel: false,
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: "http://127.0.0.1:5183",
    storageState: {
      cookies: [],
      origins: [
        {
          origin: "http://127.0.0.1:5184",
          localStorage: [{ name: "lottery.admin.locale", value: "zh-CN" }],
        },
      ],
    },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: { executablePath },
  },
  projects: [
    {
      name: "desktop1440",
      use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } },
    },
    {
      name: "mobile360",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 360, height: 800 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
});
