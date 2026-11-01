import {
  allowHTTPFailure,
  expect,
  expectedConsoleFailureIndex,
  expectHTTPFailure,
  observeBrowser,
  test,
} from './fixtures'

test('browser diagnostics match an expected console failure by URL and status', () => {
  const failures = [{ status: 409, url: 'http://127.0.0.1/expected' }]
  const text =
    'Failed to load resource: the server responded with a status of 409 (Conflict)'

  expect(
    expectedConsoleFailureIndex(failures, text, 'http://127.0.0.1/unexpected'),
  ).toBe(-1)
  expect(
    expectedConsoleFailureIndex(failures, text, 'http://127.0.0.1/expected'),
  ).toBe(0)
  expect(expectedConsoleFailureIndex(failures, text, '')).toBe(-1)
  expect(
    expectedConsoleFailureIndex(
      failures,
      'Failed to load resource: the server responded with a status of 404 (Not Found)',
      'http://127.0.0.1/expected',
    ),
  ).toBe(-1)
})

test('browser diagnostics preserve unexpected failures beside expected failures', async ({
  browser,
}) => {
  const probe = await browser.newPage({ baseURL: process.env.PT_E2E_BASE_URL })
  const diagnostics = await observeBrowser(probe)
  try {
    await probe.route('**/api/v1/diagnostic/**', (route) =>
      route.fulfill({
        status: route.request().url().endsWith('/optional') ? 404 : 409,
        contentType: 'application/json',
        body: '{}',
      }),
    )
    await probe.route('**/api/v1/exports/**', (route) =>
      route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: '{}',
      }),
    )
    await probe.goto('/login')
    expectHTTPFailure(probe, 'GET', '/api/v1/diagnostic/expected', 409)
    allowHTTPFailure(probe, 'GET', '/api/v1/diagnostic/optional', 404)
    const statuses = await probe.evaluate(async () => {
      const requests = [
        '/api/v1/diagnostic/expected?case=one',
        '/api/v1/diagnostic/unexpected',
        '/api/v1/diagnostic/optional',
        '/api/v1/diagnostic/optional',
        '/api/v1/exports/private-ticket?key=private-query',
      ]
      const result = []
      for (const url of requests) result.push((await fetch(url)).status)
      return result
    })
    expect(statuses).toEqual([409, 409, 404, 404, 409])
    await expect.poll(() => diagnostics.errors.length).toBe(2)
    expect(diagnostics.errors[0]).toContain('/api/v1/diagnostic/unexpected')
    expect(diagnostics.errors[1]).toContain('/api/v1/exports/[REDACTED]')
    expect(diagnostics.errors.join('\n')).not.toContain('private-ticket')
    expect(diagnostics.errors.join('\n')).not.toContain('private-query')
    expect(diagnostics.pendingFailures).toEqual([])
  } finally {
    await probe.close()
  }
})
