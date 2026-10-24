import { test as base, expect, type APIRequestContext } from '@playwright/test'

// Later auth fixtures can extend this test and seed real users through api.
// No setup/auth/seed endpoints exist in M0.
export const test = base.extend<{
  api: APIRequestContext
  browserDiagnostics: void
}>({
  api: async ({ request }, use) => {
    await use(request)
  },
  browserDiagnostics: [
    async ({ page }, use, testInfo) => {
      const errors: string[] = []
      page.on('console', (message) => {
        if (['warning', 'error'].includes(message.type())) {
          errors.push(`console.${message.type()}: ${message.text()}`)
        }
      })
      page.on('pageerror', (error) =>
        errors.push(`pageerror: ${error.message}`),
      )
      await page.addInitScript(() => {
        document.addEventListener('securitypolicyviolation', (event) => {
          console.error(`CSP: ${event.violatedDirective} ${event.blockedURI}`)
        })
      })
      await use()
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
