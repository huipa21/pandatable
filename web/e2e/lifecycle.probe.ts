import { renameSync, writeFileSync } from 'node:fs'
import { test, expect } from './fixtures'

// Not a *.spec.ts: only the opt-in Go lifecycle test selects this failure probe.
test('disposable lifecycle probe', async ({ page, browser }) => {
  test.setTimeout(90_000)
  const marker = process.env.PTDEV_E2E_MARKER
  if (!marker)
    throw new Error('The opt-in lifecycle harness must supply a marker')
  await page.goto('/login')
  await expect(page.getByRole('heading')).toBeVisible()
  const cdp = await browser.newBrowserCDPSession()
  const { processInfo } = await cdp.send('SystemInfo.getProcessInfo')
  writeFileSync(
    marker + '.tmp',
    JSON.stringify({
      results: process.env.PT_E2E_RESULTS_DIR,
      url: process.env.PT_E2E_BASE_URL,
      pids: [process.pid, process.ppid, ...processInfo.map((info) => info.id)],
    }),
  )
  renameSync(marker + '.tmp', marker)
  if (process.env.PTDEV_E2E_MODE === 'failure') {
    expect('deliberate failure').toBe('success')
  } else {
    await new Promise(() => {})
  }
})
