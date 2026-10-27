import type { Components } from '@revolist/revogrid'
import { writeFile } from 'node:fs/promises'
import { test, expect } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }

interface QueryMeasurement {
  offset: number
  limit: number
  includeCount: boolean
  start: number
  headers: number
  bodyEnd: number | null
  status: number
  firstId: string | null
  rows: number | null
  total: number | null
}

interface PaintMeasurement {
  index: number
  start: number
  painted: number | null
  frames: number
}

declare global {
  interface Window {
    ptBaseline: {
      queries: QueryMeasurement[]
      paints: PaintMeasurement[]
      pending: PaintMeasurement
    }
  }
}

test('AT-312 workflow: real 200k production grid paint and global middle pages', async ({
  page,
}, testInfo) => {
  test.setTimeout(150_000)
  const tableId = process.env.PT_E2E_PERF_TABLE
  const viewId = process.env.PT_E2E_PERF_VIEW
  const reportPath = process.env.PT_E2E_PERF_REPORT
  if (!tableId || !viewId || !reportPath || process.env.PT_E2E_PERF !== '1')
    throw new Error('Use ptdev e2e --perf with its owned real seed fixture')
  await page.setViewportSize({ width: 1440, height: 900 })
  const login = await page.request.post('/api/v1/auth/login', {
    headers: {
      Origin: process.env.PT_E2E_BASE_URL!,
      'X-PT-CSRF': '1',
    },
    data: { login: 'first-admin', password: 'Setup-password-123' },
  })
  expect(login.status()).toBe(200)
  // This fresh context has no SPA visit or asset prewarming before navigation.
  await page.addInitScript(
    ({ rowAction }: { rowAction: string }) => {
      localStorage.setItem('pandatable.locale', 'en')
      localStorage.setItem('pandatable.theme', 'light')
      const first: PaintMeasurement = {
        index: 0,
        start: 0,
        painted: null,
        frames: 0,
      }
      const state = {
        queries: [] as QueryMeasurement[],
        paints: [first],
        pending: first,
      }
      window.ptBaseline = state
      const originalFetch = window.fetch.bind(window)
      window.fetch = async (input, init) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.href
              : input.url
        if (
          !/\/api\/v1\/views\/[^/]+\/query$/.test(
            new URL(url, location.href).pathname,
          ) ||
          typeof init?.body !== 'string'
        )
          return originalFetch(input, init)
        const body: {
          offset?: number
          limit?: number
          includeCount?: boolean
        } = JSON.parse(init.body)
        const sample: QueryMeasurement = {
          offset: body.offset ?? 0,
          limit: body.limit ?? 100,
          includeCount: body.includeCount ?? true,
          start: performance.now(),
          headers: 0,
          bodyEnd: null,
          status: 0,
          firstId: null,
          rows: null,
          total: null,
        }
        state.queries.push(sample)
        const response = await originalFetch(input, init)
        sample.headers = performance.now()
        sample.status = response.status
        void response
          .clone()
          .json()
          .then((result: { rows: { id: string }[]; total: number | null }) => {
            sample.bodyEnd = performance.now()
            sample.rows = result.rows.length
            sample.firstId = result.rows[0]?.id ?? null
            sample.total = result.total
          })
          .catch((cause: unknown) =>
            console.error('Performance response observation failed', cause),
          )
        return response
      }
      function frame() {
        const pending = state.pending
        if (pending.painted === null) {
          const grid = document.querySelector('revo-grid')
          const expected = `Seed row ${String(pending.index + 1).padStart(6, '0')}`
          const cell = [...(grid?.querySelectorAll('.rgCell') ?? [])].find(
            (element) => {
              if (element.textContent?.trim() !== expected) return false
              const rect = element.getBoundingClientRect()
              const viewport = grid?.getBoundingClientRect()
              return (
                viewport &&
                rect.width > 0 &&
                rect.height > 0 &&
                rect.bottom > viewport.top &&
                rect.top < viewport.bottom &&
                rect.right > viewport.left &&
                rect.left < viewport.right
              )
            },
          )
          const action = [
            ...(grid?.querySelectorAll(
              'revogr-data[col-type="colPinStart"] button:not(:disabled)',
            ) ?? []),
          ].find(
            (button) =>
              button.getAttribute('aria-label') ===
              rowAction.replace('{id}', String(pending.index + 1)),
          )
          if (cell && action) {
            pending.frames++
            // Two consecutive rAFs bound a real rendering opportunity, rather
            // than stopping at response headers or Vue's DOM commit.
            if (pending.frames >= 2) pending.painted = performance.now()
          } else pending.frames = 0
        }
        requestAnimationFrame(frame)
      }
      requestAnimationFrame(frame)
    },
    { rowAction: en.rowExpand.open },
  )

  const samples: {
    index: number
    scrollStart: number
    paintMs: number
    cachedRows: number
    totalSlots: number
    query: QueryMeasurement
  }[] = []
  try {
    await page.goto(`/t/${tableId}/v/${viewId}`, { waitUntil: 'commit' })
    for (const index of [0, 100000, 100500, 99900, 101000, 100100]) {
      if (index !== 0) {
        await page
          .locator('revo-grid')
          .evaluate(async (element, globalIndex) => {
            const pending: PaintMeasurement = {
              index: globalIndex,
              start: performance.now(),
              painted: null,
              frames: 0,
            }
            window.ptBaseline.pending = pending
            window.ptBaseline.paints.push(pending)
            await (element as HTMLElement & Components.RevoGrid).scrollToRow(
              globalIndex,
            )
          }, index)
      }
      await page.waitForFunction(
        (globalIndex) => {
          const state = window.ptBaseline
          return (
            state?.pending.index === globalIndex &&
            state.pending.painted !== null &&
            state.queries.some(
              (query) =>
                query.offset === globalIndex &&
                query.start >= state.pending.start &&
                query.bodyEnd !== null,
            )
          )
        },
        index,
        { timeout: 5000, polling: 'raf' },
      )
      const sample = await page
        .locator('revo-grid')
        .evaluate(async (element, globalIndex) => {
          const state = window.ptBaseline
          const paint = state.pending
          const query = state.queries.find(
            (item) => item.offset === globalIndex && item.start >= paint.start,
          )
          if (paint.painted === null || !query)
            throw new Error('Missing real paint/query boundary')
          const source = await (
            element as HTMLElement & Components.RevoGrid
          ).getSource()
          return {
            index: globalIndex,
            scrollStart: paint.start,
            paintMs: paint.painted - paint.start,
            cachedRows: source.filter((row) => row.__rowId).length,
            totalSlots: source.length,
            query,
          }
        }, index)
      samples.push(sample)
    }
  } finally {
    const observations = await page.evaluate(() => ({
      ...window.ptBaseline,
      timeOrigin: performance.timeOrigin,
      navigation: performance
        .getEntriesByType('navigation')
        .map((entry) => entry.toJSON()),
      assets: performance
        .getEntriesByType('resource')
        .filter((entry) => new URL(entry.name).pathname.startsWith('/assets/'))
        .map((entry) => entry.toJSON()),
      userAgent: navigator.userAgent,
      viewport: { width: innerWidth, height: innerHeight },
    }))
    const report = {
      boundary:
        'Fresh authenticated document navigation to visible exact real row plus enabled action across two rAFs. Middle samples start before public scrollToRow(globalIndex); request and response-body boundaries are recorded independently. No mocks, spike timers, viewport row indices or asset prewarming.',
      acceptance:
        'Timing assertions alone do not certify target host or NFR-1. Correlate host.json, resources.jsonl, physical host allocation and production image version.',
      samples,
      observations,
    }
    console.log('200k browser baseline:', JSON.stringify(samples))
    await writeFile(reportPath, JSON.stringify(report, null, 2), {
      mode: 0o600,
      flag: 'wx',
    })
    await testInfo.attach('performance-baseline', {
      body: JSON.stringify(report, null, 2),
      contentType: 'application/json',
    })
  }
  for (const sample of samples) {
    expect(sample.totalSlots).toBe(200000)
    expect(sample.cachedRows).toBeGreaterThan(0)
    expect(sample.cachedRows).toBeLessThanOrEqual(300)
    expect(sample.query.limit).toBe(100)
    expect(sample.query.status).toBe(200)
    expect(sample.query.rows).toBe(100)
    expect(sample.query.firstId).toBe(String(sample.index + 1))
    expect(sample.query.start).toBeGreaterThanOrEqual(sample.scrollStart)
    expect(sample.query.includeCount).toBe(sample.index === 0)
    if (sample.index === 0) expect(sample.query.total).toBe(200000)
    expect(
      sample.paintMs,
      sample.index === 0
        ? 'first usable paint from document navigation <1.5s'
        : `global offset ${sample.index}: scroll-to-usable-paint <300ms`,
    ).toBeLessThan(sample.index === 0 ? 1500 : 300)
    if (sample.index !== 0)
      expect(
        sample.query.bodyEnd! - sample.query.start,
        `global offset ${sample.index}: request-to-body <300ms`,
      ).toBeLessThan(300)
  }
})
