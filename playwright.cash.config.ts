import { defineConfig, devices } from "@playwright/test";
import base from "./playwright.layout.config";
export default defineConfig({
  ...base,
  testMatch: "cash-wallet.spec.ts",
  outputDir: ".local/multi-currency-20261111/browser-results",
  workers: 4,
  projects: [
    {
      name: "android",
      use: { ...devices["Pixel 7"], browserName: "chromium" },
    },
    {
      name: "android-landscape",
      use: { ...devices["Pixel 7 landscape"], browserName: "chromium" },
    },
    { name: "ios", use: { ...devices["iPhone 13"], browserName: "webkit" } },
    {
      name: "ios-landscape",
      use: { ...devices["iPhone 13 landscape"], browserName: "webkit" },
    },
    { name: "chromium-reflow", use: { ...devices["Desktop Chrome"] } },
    { name: "webkit-reflow", use: { ...devices["Desktop Safari"] } },
  ],
});
