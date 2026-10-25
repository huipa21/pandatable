import {
  test as base,
  expect,
  type APIRequestContext,
  type Page,
} from '@playwright/test'

export async function observeBrowser(page: Page) {
  const errors: string[] = []
  const pendingFailures: number[] = []
  page.on('response', (response) => {
    const path = new URL(response.url()).pathname
    // Setup negative cases are asserted by AT-101; an absent/revoked session
    // is the normal identity-probe result on public routes.
    if (
      (path === '/api/v1/setup' && [401, 410].includes(response.status())) ||
      (path === '/api/v1/auth/me' && response.status() === 401)
    )
      pendingFailures.push(response.status())
  })
  page.on('console', (message) => {
    if (['warning', 'error'].includes(message.type())) {
      const expected = pendingFailures.findIndex((status) =>
        message.text().includes(`status of ${status} `),
      )
      if (message.type() === 'error' && expected >= 0) {
        pendingFailures.splice(expected, 1)
        return
      }
      errors.push(`console.${message.type()}: ${message.text()}`)
    }
  })
  page.on('pageerror', (error) => errors.push(`pageerror: ${error.message}`))
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      console.error(`CSP: ${event.violatedDirective} ${event.blockedURI}`)
    })
  })
  return { errors, pendingFailures }
}

export const test = base.extend<{
  api: APIRequestContext
  browserDiagnostics: void
}>({
  api: async ({ request }, use) => {
    await use(request)
  },
  browserDiagnostics: [
    async ({ page }, use, testInfo) => {
      const { errors, pendingFailures } = await observeBrowser(page)
      await use()
      expect(
        pendingFailures,
        'only matched expected setup/identity HTTP console diagnostics',
      ).toEqual([])
      await testInfo.attach('browser-diagnostics', {
        body: JSON.stringify(errors, null, 2),
        contentType: 'application/json',
      })
      expect(errors, 'no browser warnings, errors or CSP violations').toEqual(
        [],
      )
    },
    { auto: true },
  ],
})

export { expect } from '@playwright/test'
