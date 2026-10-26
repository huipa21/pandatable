import {
  test as base,
  expect,
  type APIRequestContext,
  type Page,
} from '@playwright/test'

const expectedHTTPFailures = new WeakMap<
  Page,
  { method: string; path: string; status: number }[]
>()

export function expectHTTPFailure(
  page: Page,
  method: string,
  path: string,
  status: number,
) {
  const failures = expectedHTTPFailures.get(page) ?? []
  failures.push({ method, path, status })
  expectedHTTPFailures.set(page, failures)
}

export async function observeBrowser(page: Page) {
  const errors: string[] = []
  const pendingFailures: number[] = []
  page.on('response', (response) => {
    const path = new URL(response.url()).pathname
    const expected = expectedHTTPFailures.get(page) ?? []
    const index = expected.findIndex(
      (failure) =>
        failure.path === path &&
        failure.method === response.request().method() &&
        failure.status === response.status(),
    )
    if (index >= 0) {
      expected.splice(index, 1)
      pendingFailures.push(response.status())
      return
    }
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

export function expectBrowserClean(
  page: Page,
  diagnostics: { errors: string[]; pendingFailures: number[] },
) {
  expect(
    expectedHTTPFailures.get(page) ?? [],
    'every explicit HTTP failure occurred',
  ).toEqual([])
  expect(
    diagnostics.pendingFailures,
    'only matched expected HTTP console diagnostics',
  ).toEqual([])
  expect(
    diagnostics.errors,
    'no browser warnings, errors or CSP violations',
  ).toEqual([])
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
        expectedHTTPFailures.get(page) ?? [],
        'every explicit HTTP failure occurred',
      ).toEqual([])
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
