import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  fullyParallel: false,
  timeout: 90000,
  expect: { timeout: 10000 },
  reporter: "list",
  outputDir: "../../workspace/browser-results",
  use: {
    baseURL: process.env.ISLET_TEST_URL || "http://127.0.0.1:28091",
    channel: "msedge",
    headless: true,
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
    screenshot: "off",
    trace: "off",
  },
});
