// Playwright settings of the end-to-end journey (T-020). scripts/e2e.sh starts the API and the web server and sets E2E_BASE_URL and E2E_OUT.
import { defineConfig } from "@playwright/test";
import { assertSafeTarget } from "./guard.mjs";

assertSafeTarget(); // local base URL and a throwaway database only (T-087)

const out = process.env.E2E_OUT ?? "out";

export default defineConfig({
  testDir: ".",
  testMatch: [
    "journey.spec.mjs",
    "h2-gallery.spec.mjs",
    "class.spec.mjs",
    "design.spec.mjs",
    "editor-shell.spec.mjs",
    "editor-gestures.spec.mjs",
    "editor-text.spec.mjs",
    "editor-palette.spec.mjs",
    "editor-history.spec.mjs",
    "editor-navy.spec.mjs",
    "editor-flow.spec.mjs",
    "editor-flow-qa.spec.mjs",
    "editor-flow-stale-qa.spec.mjs",
    "editor-templates.spec.mjs",
    "editor-save-template.spec.mjs",
    "editor-phone.spec.mjs",
    "editor-phone-qa.spec.mjs",
    "editor-keyboard.spec.mjs",
    "editor-a11y.spec.mjs",
    "app-a11y.spec.mjs",
    "stage2-journey.spec.mjs",
    "stage2-budgets.spec.mjs",
  ],
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
  projects: [
    { name: "chromium", testIgnore: ["stage2-journey.spec.mjs", "stage2-budgets.spec.mjs"], use: { browserName: "chromium" } },
    // T-183: the stage 2 story once on a desktop and once on a tablet (1024 px, touch); the spec reads the project name for its viewport
    { name: "journey-desktop", testMatch: ["stage2-journey.spec.mjs"], use: { browserName: "chromium" } },
    // the budgets (STAGE2_BUDGETS=1 only, see the spec)
    { name: "budgets", testMatch: ["stage2-budgets.spec.mjs"], use: { browserName: "chromium" } },
    { name: "journey-tablet", testMatch: ["stage2-journey.spec.mjs"], use: { browserName: "chromium" } },
  ],
});
