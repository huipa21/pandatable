import { defineConfig, devices } from '@playwright/test'

const baseURL = process.env.PT_E2E_BASE_URL
if (!baseURL) {
  throw new Error('PT_E2E_BASE_URL is required; run go run ./tools/ptdev e2e')
}
const target = new URL(baseURL)
if (target.protocol !== 'http:' || target.hostname !== '127.0.0.1') {
  throw new Error('E2E requires an isolated HTTP loopback stack')
}

export default defineConfig({
  testDir: './e2e',
  outputDir: process.env.PT_E2E_RESULTS_DIR ?? './test-results/manual',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  globalTimeout: 120_000,
  expect: { timeout: 5_000 },
  forbidOnly: true,
  reporter: [['list']],
  use: {
    baseURL,
    locale: 'en-US',
    colorScheme: 'light',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'desktop-chromium', use: { ...devices['Desktop Chrome'] } },
    {
      name: 'mobile-chromium',
      use: { ...devices['Pixel 7'], defaultBrowserType: 'chromium' },
    },
  ],
})
