import fs from 'node:fs'
import path from 'node:path'
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

function matchPath(expected: string, actual: string) {
  if (!expected.includes('*')) return expected === actual
  const pattern = expected
    .split('*')
    .map((segment) => segment.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
    .join('.*')
  return new RegExp(`^${pattern}$`).test(actual)
}

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

// Optional failures may occur any number of times (including zero) on one page,
// e.g. background revalidation racing a deliberate permission revocation.
const optionalHTTPFailures = new WeakMap<
  Page,
  { method: string; path: string; status: number }[]
>()

export function allowHTTPFailure(
  page: Page,
  method: string,
  path: string,
  status: number,
) {
  const failures = optionalHTTPFailures.get(page) ?? []
  failures.push({ method, path, status })
  optionalHTTPFailures.set(page, failures)
}

type PendingHTTPFailure = { status: number; url: string }

function diagnosticPath(path: string) {
  return path.replace(
    /^\/api\/v1\/exports\/[^/]+$/,
    '/api/v1/exports/[REDACTED]',
  )
}

export function expectedConsoleFailureIndex(
  failures: readonly PendingHTTPFailure[],
  text: string,
  url: string,
) {
  return failures.findIndex(
    (failure) =>
      failure.url === url && text.includes(`status of ${failure.status} `),
  )
}

const paceFile = path.resolve(
  process.env.PT_E2E_RESULTS_DIR ?? './test-results',
  'login-pace.json',
)

export async function paceLogin(page?: Page) {
  let timestamps: number[] = []
  try {
    if (fs.existsSync(paceFile)) {
      timestamps = JSON.parse(fs.readFileSync(paceFile, 'utf8'))
    }
  } catch {
    timestamps = []
  }
  let now = Date.now()
  timestamps = timestamps.filter((t) => now - t <= 60_000)
  if (timestamps.length >= 5) {
    const oldest = timestamps[0] ?? now
    const waitMs = Math.max(0, 61_000 - (now - oldest))
    if (page) await page.waitForTimeout(waitMs)
    else await new Promise((resolve) => setTimeout(resolve, waitMs))
    now = Date.now()
    timestamps = timestamps.filter((t) => now - t <= 60_000)
  }
  timestamps.push(now)
  try {
    fs.mkdirSync(path.dirname(paceFile), { recursive: true })
    fs.writeFileSync(paceFile, JSON.stringify(timestamps), 'utf8')
  } catch {
    // ignore
  }
}

export async function observeBrowser(page: Page) {
  const errors: string[] = []
  const pendingFailures: PendingHTTPFailure[] = []
  const reportResponses =
    page.context().browser()?.browserType().name() === 'firefox'
  page.on('response', (response) => {
    const path = new URL(response.url()).pathname
    const expected = expectedHTTPFailures.get(page) ?? []
    const index = expected.findIndex(
      (failure) =>
        matchPath(failure.path, path) &&
        failure.method === response.request().method() &&
        failure.status === response.status(),
    )
    if (index >= 0) expected.splice(index, 1)
    // Setup negative cases are asserted by AT-101; an absent/revoked session
    // is the normal identity-probe result on public routes.
    if (
      index >= 0 ||
      (path === '/api/v1/setup' && [401, 410].includes(response.status())) ||
      (path === '/api/v1/auth/me' && response.status() === 401) ||
      (optionalHTTPFailures.get(page) ?? []).some(
        (failure) =>
          matchPath(failure.path, path) &&
          failure.method === response.request().method() &&
          failure.status === response.status(),
      )
    ) {
      if (!reportResponses)
        pendingFailures.push({ status: response.status(), url: response.url() })
    } else if (reportResponses && response.status() >= 400) {
      errors.push(
        `HTTP ${response.request().method()} ${response.status()}: ${diagnosticPath(path)}`,
      )
    }
  })
  page.on('console', (message) => {
    if (['warning', 'error'].includes(message.type())) {
      const location = message.location().url
      const expected = expectedConsoleFailureIndex(
        pendingFailures,
        message.text(),
        location,
      )
      if (message.type() === 'error' && expected >= 0) {
        pendingFailures.splice(expected, 1)
        return
      }
      const sourcePath =
        location.startsWith('http://') || location.startsWith('https://')
          ? diagnosticPath(new URL(location).pathname)
          : ''
      const source = sourcePath ? ` (${sourcePath})` : ''
      errors.push(`console.${message.type()}: ${message.text()}${source}`)
    }
  })
  page.on('pageerror', (error) => errors.push(`pageerror: ${error.message}`))
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      console.error(`CSP: ${event.violatedDirective} ${event.blockedURI}`)
    })
  })
  return {
    errors,
    get pendingFailures() {
      return pendingFailures.map((failure) => failure.status)
    },
  }
}

export function expectBrowserClean(
  page: Page,
  diagnostics: { errors: string[]; pendingFailures: number[] },
) {
  const remaining = expectedHTTPFailures.get(page) ?? []
  if (remaining.length) console.error('remaining expected failures', remaining)
  expect(remaining, 'every explicit HTTP failure occurred').toEqual([])
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
      const diagnostics = await observeBrowser(page)
      await use()
      expect(
        expectedHTTPFailures.get(page) ?? [],
        'every explicit HTTP failure occurred',
      ).toEqual([])
      expect(
        diagnostics.pendingFailures,
        'only matched expected setup/identity HTTP console diagnostics',
      ).toEqual([])
      await testInfo.attach('browser-diagnostics', {
        body: JSON.stringify(diagnostics.errors, null, 2),
        contentType: 'application/json',
      })
      expect(
        diagnostics.errors,
        'no browser warnings, errors or CSP violations',
      ).toEqual([])
    },
    { auto: true },
  ],
})

export { expect } from '@playwright/test'
