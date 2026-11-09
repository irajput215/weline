import { defineConfig, devices } from '@playwright/test'

// Playwright journeys for Weline modules (frontend/src/modules/<module>/e2e/*.e2e.ts).
// Kept apart from upstream's e2e suite so neither runner picks up the other's files.
// Needs the app running with migrations applied (default http://localhost:8080):
//   npx playwright test -c src/modules/playwright.config.ts
export default defineConfig({
  testDir: '.',
  testMatch: '**/e2e/*.e2e.ts',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    trace: 'off',
    screenshot: 'only-on-failure',
    // Optional: a Chromium other than the one this Playwright version downloads.
    launchOptions: process.env.PW_CHROMIUM_PATH ? { executablePath: process.env.PW_CHROMIUM_PATH } : {}
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
})
