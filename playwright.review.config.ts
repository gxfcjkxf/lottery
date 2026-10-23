import { assertReviewIsolation } from './scripts/review-isolation.mjs';
import { defineConfig, devices } from "@playwright/test";

assertReviewIsolation(process.env);

const password = process.env.TEST_REVIEW_ADMIN_PASSWORD;
if (!password) {
  throw new Error(
    "TEST_REVIEW_ADMIN_PASSWORD is required to run the real review smoke suite.",
  );
}

for (const name of ['TEST_PLATFORM_ADMIN_USERNAME', 'TEST_PLATFORM_ADMIN_PASSWORD', 'TEST_PLATFORM_ADMIN_ORIGIN']) {
  if (!process.env[name]) throw new Error(`${name} is required for the complete review suite; brand creation must not be skipped.`);
}

if (process.env.TEST_PLATFORM_ADMIN_USERNAME?.trim().toLowerCase() === 'admin') {
  throw new Error('Use dedicated review accounts; shared admin login replaces the developer session.');
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
  workers: 2,
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
      name: "mobilepixel7",
      use: {
        ...devices["Pixel 7"],
        contextOptions: { screen: devices["Pixel 7"].screen },
      },
    },
  ],
});
