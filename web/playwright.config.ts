import { defineConfig, devices } from '@playwright/test'

const baseURL = process.env.PT_E2E_BASE_URL
if (!baseURL) {
  throw new Error('PT_E2E_BASE_URL is required; run go run ./tools/ptdev e2e')
}
const target = new URL(baseURL)
if (target.protocol !== 'http:' || target.hostname !== '127.0.0.1') {
  throw new Error('E2E requires an isolated HTTP loopback stack')
}
const outputDir = process.env.PT_E2E_RESULTS_DIR ?? './test-results/manual'

export default defineConfig({
  testDir: './e2e',
  outputDir,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 30_000,
  globalTimeout: 600_000,
  expect: { timeout: 5_000 },
  forbidOnly: true,
  // Compact console output (failures in full); per-test durations go to report.json.
  reporter: [['dot'], ['json', { outputFile: `${outputDir}/report.json` }]],
  use: {
    baseURL,
    locale: 'en-US',
    colorScheme: 'light',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'setup-chromium',
      testMatch: '**/setup.spec.ts',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'desktop-chromium',
      testIgnore:
        process.env.PT_E2E_PERF === '1'
          ? '**/setup.spec.ts'
          : ['**/setup.spec.ts', '**/performance.spec.ts'],
      dependencies: ['setup-chromium'],
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'mobile-chromium',
      testIgnore: [
        '**/setup.spec.ts',
        '**/performance.spec.ts',
        '**/profile.spec.ts',
        '**/fields-acceptance.spec.ts',
      ],
      dependencies: ['setup-chromium'],
      use: { ...devices['Pixel 7'], defaultBrowserType: 'chromium' },
    },
  ],
})
