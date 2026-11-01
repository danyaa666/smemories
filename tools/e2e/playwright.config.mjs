// Playwright settings of the end-to-end journey (T-020). scripts/e2e.sh starts the API and the web server and sets E2E_BASE_URL and E2E_OUT.
import { defineConfig } from "@playwright/test";
import { assertSafeTarget } from "./guard.mjs";

assertSafeTarget(); // local base URL and a throwaway database only (T-087)

const out = process.env.E2E_OUT ?? "out";

export default defineConfig({
  testDir: ".",
  testMatch: ["journey.spec.mjs", "h2-gallery.spec.mjs", "class.spec.mjs", "design.spec.mjs", "editor-shell.spec.mjs", "editor-gestures.spec.mjs", "editor-text.spec.mjs", "editor-palette.spec.mjs"],
  outputDir: `${out}/results`, // traces and screenshots of a failed run; CI uploads the folder
  timeout: 240_000, // the whole journey; the budget in CI is 5 minutes including start-up
  expect: { timeout: 15_000 },
  retries: 0, // a flaky journey must be fixed, not retried
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: process.env.E2E_BASE_URL,
    locale: "en-US",
    actionTimeout: 15_000,
    navigationTimeout: 30_000,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
});
