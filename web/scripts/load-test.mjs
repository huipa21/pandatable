import { open, readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { performance } from 'node:perf_hooks'
import { setTimeout as sleep } from 'node:timers/promises'
import { pathToFileURL } from 'node:url'
import { chromium } from '@playwright/test'
import en from '../src/i18n/en.json' with { type: 'json' }

/** @typedef {import('@playwright/test').Page} Page */
/** @typedef {import('@playwright/test').BrowserContext} BrowserContext */
/** @typedef {import('@playwright/test').APIRequestContext} APIRequestContext */
/** @typedef {HTMLElement & import('@revolist/revogrid').Components.RevoGrid} Grid */
/** @typedef {{login: string, password: string}} Credential */
/** @typedef {{baseURL: string, tableId: string, viewId: string, fieldId: string, users: Credential[]}} LoadConfig */
/** @typedef {{edits: number, scrolls: number, received: number}} Activity */
/** @typedef {{userId: string, subscribed: boolean, stopping: boolean, failure: string, received: number}} RealtimeTracker */
/** @typedef {Activity & RealtimeTracker & {context: BrowserContext, page: Page}} Client */
/** @typedef {{elapsedMs: number, appRssBytes: number, hostUsedBytes: number}} SummarySample */
/** @typedef {SummarySample & {collectedAt: string, hostTotalBytes: number}} MemorySample */
/** @typedef {{tableId: string, fieldId: string, rowIndex: number, rowId: string, scrollIndex: number, value: string}} ExerciseOptions */

const userCount = 10
const requiredDurationMs = 15 * 60 * 1000
const sampleIntervalMs = 6000
const appLimitBytes = 150_000_000
const hostLimitBytes = 3_200_000_000
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/** @param {unknown} input @returns {LoadConfig} */
export function validateConfig(input) {
  if (!input || typeof input !== 'object' || Array.isArray(input))
    throw new Error('Configuration must be an object')
  const raw = /** @type {Record<string, unknown>} */ (input)
  if (typeof raw.baseURL !== 'string')
    throw new Error('A valid baseURL is required')
  let url
  try {
    url = new URL(raw.baseURL)
  } catch {
    throw new Error('A valid baseURL is required')
  }
  const loopback = ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)
  if (
    (url.protocol !== 'https:' && !(url.protocol === 'http:' && loopback)) ||
    url.username ||
    url.password ||
    url.pathname !== '/' ||
    url.search ||
    url.hash
  )
    throw new Error('baseURL must be an HTTPS origin (HTTP only for loopback)')
  if (
    typeof raw.tableId !== 'string' ||
    !uuid.test(raw.tableId) ||
    typeof raw.viewId !== 'string' ||
    !uuid.test(raw.viewId) ||
    typeof raw.fieldId !== 'string' ||
    !uuid.test(raw.fieldId)
  )
    throw new Error('tableId, viewId and fieldId must be UUIDs')
  if (!Array.isArray(raw.users) || raw.users.length !== userCount)
    throw new Error('Exactly ten users are required')
  const names = new Set()
  /** @type {Credential[]} */
  const users = []
  for (const user of raw.users) {
    if (
      !user ||
      typeof user !== 'object' ||
      typeof user.login !== 'string' ||
      !user.login.trim() ||
      typeof user.password !== 'string' ||
      !user.password
    )
      throw new Error('Each user needs a login and password')
    names.add(user.login.trim().toLowerCase())
    users.push({ login: user.login, password: user.password })
  }
  if (names.size !== userCount) throw new Error('User logins must be distinct')
  return {
    baseURL: url.origin,
    tableId: raw.tableId,
    viewId: raw.viewId,
    fieldId: raw.fieldId,
    users,
  }
}

/** @param {unknown} value @returns {Record<string, unknown>} */
function measurementObject(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('Fresh app RSS and host-memory measurements are required')
  return /** @type {Record<string, unknown>} */ (value)
}

/** @param {unknown} snapshot @param {number} elapsedMs @param {string} previousCollectedAt @returns {MemorySample} */
export function readMemorySample(
  snapshot,
  elapsedMs,
  previousCollectedAt = '',
) {
  const payload = measurementObject(snapshot)
  const memory = measurementObject(payload.memory)
  const memoryValue = measurementObject(memory.value)
  const rss = measurementObject(payload.appRssBytes)
  const collectedAt = payload.collectedAt
  if (
    memory.status !== 'ready' ||
    rss.status !== 'ready' ||
    typeof rss.value !== 'number' ||
    !Number.isSafeInteger(rss.value) ||
    rss.value <= 0 ||
    typeof memoryValue.usedBytes !== 'number' ||
    typeof memoryValue.totalBytes !== 'number' ||
    !Number.isSafeInteger(memoryValue.usedBytes) ||
    !Number.isSafeInteger(memoryValue.totalBytes) ||
    memoryValue.usedBytes <= 0 ||
    memoryValue.totalBytes <= 0 ||
    memoryValue.usedBytes > memoryValue.totalBytes ||
    typeof collectedAt !== 'string' ||
    !Number.isFinite(Date.parse(collectedAt)) ||
    (previousCollectedAt &&
      Date.parse(collectedAt) <= Date.parse(previousCollectedAt))
  )
    throw new Error('Fresh app RSS and host-memory measurements are required')
  return {
    elapsedMs,
    collectedAt,
    appRssBytes: rss.value,
    hostUsedBytes: memoryValue.usedBytes,
    hostTotalBytes: memoryValue.totalBytes,
  }
}

/** @param {number} durationMs @param {number} elapsedMs @param {Activity[]} clients @param {SummarySample[]} samples */
export function summarize(durationMs, elapsedMs, clients, samples) {
  const appPeak = samples.length
    ? Math.max(...samples.map((sample) => sample.appRssBytes))
    : null
  const hostPeak = samples.length
    ? Math.max(...samples.map((sample) => sample.hostUsedBytes))
    : null
  const gaps = samples.slice(1).map((sample, index) => {
    const previous = samples[index]
    if (!previous) throw new Error('Missing previous memory sample')
    return sample.elapsedMs - previous.elapsedMs
  })
  const first = samples[0]
  const last = samples.at(-1)
  const covered =
    !!first &&
    !!last &&
    samples.length >= 2 &&
    first.elapsedMs <= 12_000 &&
    last.elapsedMs >= durationMs - 12_000 &&
    gaps.every((gap) => gap > 0 && gap <= 12_000)
  const active =
    clients.length === userCount &&
    clients.every(
      (client) => client.edits > 0 && client.scrolls > 0 && client.received > 0,
    )
  const complete =
    durationMs >= requiredDurationMs &&
    elapsedMs >= durationMs &&
    covered &&
    active
  return {
    complete,
    thresholdsMet:
      complete &&
      appPeak !== null &&
      hostPeak !== null &&
      appPeak < appLimitBytes &&
      hostPeak < hostLimitBytes,
    appPeakBytes: appPeak,
    hostPeakBytes: hostPeak,
    appLimitBytes,
    hostLimitBytes,
    samples: samples.length,
    clients: clients.map(({ edits, scrolls, received }) => ({
      edits,
      scrolls,
      received,
    })),
    qualification:
      'Observation only: independently verify the target host, deployment, workload and browser generator before claiming AT-801.',
  }
}

/** @param {Page} page @param {number} index @param {string} [rowId] */
async function scrollToLoaded(page, index, rowId) {
  await page.locator('revo-grid').evaluate(async (grid, target) => {
    const component = /** @type {Grid} */ (grid)
    await component.scrollToRow(target)
  }, index)
  const deadline = performance.now() + 20_000
  while (performance.now() < deadline) {
    const loaded = await page.locator('revo-grid').evaluate(
      async (grid, { target, expected, rowLabel }) => {
        const component = /** @type {Grid} */ (grid)
        const row = (await component.getSource())[target]
        if (!row?.__rowId || (expected && row.__rowId !== expected))
          return false
        const visible = () => {
          const viewport = grid.getBoundingClientRect()
          return [
            ...grid.querySelectorAll(
              'revogr-data[col-type="colPinStart"] button:not(:disabled)',
            ),
          ].some((button) => {
            if (
              button.getAttribute('aria-label') !==
              rowLabel.replace('{id}', row.__rowId)
            )
              return false
            const rect = button.getBoundingClientRect()
            return (
              rect.width > 0 &&
              rect.height > 0 &&
              rect.bottom > viewport.top &&
              rect.top < viewport.bottom &&
              rect.right > viewport.left &&
              rect.left < viewport.right
            )
          })
        }
        if (!visible()) return false
        await new Promise((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(resolve)),
        )
        return (
          (await component.getSource())[target]?.__rowId === row.__rowId &&
          visible()
        )
      },
      { target: index, expected: rowId, rowLabel: en.rowExpand.open },
    )
    if (loaded) return
    await sleep(50)
  }
  throw new Error(`Timed out waiting for grid row ${index} to load and render`)
}

/** @param {Page} page @param {ExerciseOptions} options */
export async function exerciseOnce(page, options) {
  const { tableId, fieldId, rowIndex, rowId, scrollIndex, value } = options
  await scrollToLoaded(page, scrollIndex)
  await scrollToLoaded(page, rowIndex, rowId)
  await page.locator('revo-grid').evaluate(
    async (grid, { field, index, expected }) => {
      const component = /** @type {Grid} */ (grid)
      const row = (await component.getSource())[index]
      if (row?.__rowId !== expected)
        throw new Error('Grid row changed before editing')
      const column = (await component.getColumns()).find(
        (item) => item.prop === field,
      )
      if (!column) throw new Error('Load-test field is not visible')
      const pin = column.pin ?? 'rgCol'
      if (pin === 'rgCol') await component.scrollToColumnProp(field)
      const store = await component.getColumnStore(pin)
      const source = store.get('source')
      const x = store
        .get('items')
        .findIndex((item) => source[item]?.prop === field)
      if (x < 0) throw new Error('Load-test column is not visible')
      await component.setCellsFocus({ x, y: index }, { x, y: index }, pin)
      component.focus()
      await new Promise((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(resolve)),
      )
      const focused = await component.getFocused()
      if (
        focused?.model?.__rowId !== expected ||
        focused.column?.prop !== field
      )
        throw new Error('Grid did not focus the intended cell')
      await component.setCellEdit(index, column.prop)
    },
    { field: fieldId, index: rowIndex, expected: rowId },
  )
  const editor = page.getByRole('textbox', {
    name: en.field.editor.value,
    exact: true,
  })
  await editor.fill(value, { timeout: 20_000 })
  const [saved] = await Promise.all([
    page.waitForResponse(
      (item) =>
        item.request().method() === 'PATCH' &&
        new URL(item.url()).pathname ===
          `/api/v1/tables/${tableId}/rows/${rowId}`,
      { timeout: 20_000 },
    ),
    editor.press('Enter'),
  ])
  if (saved.status() !== 200) throw new Error('Grid edit failed')
  const body = await saved.json()
  if (body.row?.id !== rowId || body.row?.fields?.[fieldId] !== value)
    throw new Error('Grid edit response did not confirm the intended value')
}

/** @param {Page} page @param {RealtimeTracker} client @param {string} tableId */
export function trackPage(page, client, tableId) {
  page.on('pageerror', () => {
    client.failure = 'Browser page error'
  })
  page.on('crash', () => {
    client.failure = 'Browser page crashed'
  })
  page.on('websocket', (socket) => {
    socket.on('framereceived', ({ payload }) => {
      try {
        const event = JSON.parse(String(payload))
        if (event.t === 'subscribed' && event.tableId === tableId)
          client.subscribed = true
        if (
          event.t === 'row.updated' &&
          event.tableId === tableId &&
          event.by &&
          event.by !== client.userId
        )
          client.received++
        if (['error', 'sub.denied', 'session.revoked'].includes(event.t))
          client.failure = 'Realtime protocol or authorization failure'
      } catch {
        client.failure = 'Invalid realtime event'
      }
    })
    socket.on('close', () => {
      if (!client.stopping) client.failure = 'Realtime connection closed'
    })
  })
}

/** @param {RealtimeTracker} client */
export async function waitForSubscription(client) {
  const deadline = performance.now() + 20_000
  while (!client.subscribed && !client.failure && performance.now() < deadline)
    await sleep(100)
  if (client.failure || !client.subscribed)
    throw new Error(client.failure || 'Realtime subscription timed out')
}

/** @param {APIRequestContext} request @param {'get' | 'post'} method @param {string} path @param {Parameters<APIRequestContext['post']>[1]} options @returns {Promise<unknown>} */
async function requestJSON(request, method, path, options = {}) {
  const response = await request[method](path, { timeout: 20_000, ...options })
  if (!response.ok())
    throw new Error(
      `${method.toUpperCase()} ${path} failed (${response.status()})`,
    )
  return response.json()
}

/** @param {import('../src/api/types').TableView | undefined} view */
export function validateGridView(view) {
  if (
    !view ||
    view.type !== 'grid' ||
    view.scope !== 'shared' ||
    !view.config ||
    view.config.filter ||
    view.config.sort?.length ||
    view.config.groupBy?.length
  )
    throw new Error(
      'Fixture needs a shared, unfiltered, unsorted and ungrouped grid view',
    )
}

/** @param {Client} client @param {LoadConfig} config */
async function prepareFixture(client, config) {
  const schema = /** @type {import('../src/api/types').TableSchema} */ (
    await requestJSON(
      client.context.request,
      'get',
      `/api/v1/tables/${config.tableId}/schema`,
    )
  )
  const field = schema.fields?.find((item) => item.id === config.fieldId)
  const view = schema.views?.find((item) => item.id === config.viewId)
  validateGridView(view)
  const maxLength = field?.options?.maxLength
  if (
    field?.uiType !== 'text' ||
    field.isSystem ||
    field.isUnique ||
    (maxLength !== undefined &&
      (typeof maxLength !== 'number' || maxLength < 16))
  )
    throw new Error('Fixture needs a visible, non-unique text field')
  const query = /** @type {import('../src/api/types').RowQueryResult} */ (
    await requestJSON(
      client.context.request,
      'post',
      `/api/v1/views/${config.viewId}/query`,
      {
        headers: { Origin: config.baseURL, 'X-PT-CSRF': '1' },
        data: { offset: 0, limit: 100, includeCount: true },
      },
    )
  )
  if (
    typeof query.total !== 'number' ||
    !Number.isSafeInteger(query.total) ||
    query.total < 600 ||
    !query.fieldIds?.includes(config.fieldId) ||
    !Array.isArray(query.rows) ||
    query.rows.length < userCount ||
    new Set(query.rows.slice(0, userCount).map((row) => row.id)).size !==
      userCount
  )
    throw new Error('Fixture needs at least 600 rows and a visible test field')
  return { total: query.total, rows: query.rows.slice(0, userCount) }
}

/** @param {LoadConfig} config @param {string} reportPath @param {number} durationMs */
async function runLoad(config, reportPath, durationMs) {
  const report = await open(reportPath, 'wx', 0o600)
  /** @type {MemorySample[]} */
  const samples = []
  /** @type {Client[]} */
  const clients = []
  /** @type {import('@playwright/test').Browser | undefined} */
  let browser
  /** @type {string | undefined} */
  let failure
  /** @type {number | undefined} */
  let start
  /** @type {ReturnType<typeof summarize> | undefined} */
  let result
  /** @param {object} item */
  const write = async (item) => {
    await report.write(`${JSON.stringify(item)}\n`)
  }
  try {
    await write({
      type: 'header',
      startedAt: new Date().toISOString(),
      durationMs,
      users: userCount,
      sampleIntervalMs,
      boundary:
        'Ten independent browser users edit distinct rows, scroll the production grid and receive peer WebSocket events; memory is from the server admin system collector.',
    })
    browser = await chromium.launch({ headless: true })
    for (let index = 0; index < userCount; index++) {
      if (index > 0) await sleep(6100)
      const context = await browser.newContext({
        baseURL: config.baseURL,
        viewport: { width: 1440, height: 900 },
        locale: 'en-US',
      })
      /** @type {Client} */
      const client = {
        context,
        page: await context.newPage(),
        edits: 0,
        scrolls: 0,
        received: 0,
        userId: '',
        subscribed: false,
        stopping: false,
        failure: '',
      }
      clients.push(client)
      const identity =
        /** @type {import('../src/api/types').AuthUserMutation} */ (
          await requestJSON(context.request, 'post', '/api/v1/auth/login', {
            headers: { Origin: config.baseURL, 'X-PT-CSRF': '1' },
            data: config.users[index],
          })
        )
      if (
        !identity.user?.id ||
        identity.user.mustChangePassword ||
        (index === 0 && identity.user.role !== 'admin') ||
        clients.some(
          (other) => other !== client && other.userId === identity.user.id,
        )
      )
        throw new Error(
          'Users must be distinct, active and ready; the first must be an administrator',
        )
      client.userId = identity.user.id
      await context.addInitScript(() => {
        localStorage.setItem('pandatable.locale', 'en')
        localStorage.setItem('pandatable.theme', 'light')
      })
      trackPage(client.page, client, config.tableId)
    }
    const firstClient = clients[0]
    if (!firstClient) throw new Error('Missing administrator session')
    const fixture = await prepareFixture(firstClient, config)
    await Promise.all(
      clients.map(async (client) => {
        await client.page.goto(`/t/${config.tableId}/v/${config.viewId}`)
        await client.page.locator('revo-grid').waitFor({ timeout: 30_000 })
        await waitForSubscription(client)
      }),
    )

    const started = performance.now()
    start = started
    const deadline = started + durationMs
    const controller = new AbortController()
    const sample = async () => {
      let next = started
      let previous = ''
      while (next <= deadline) {
        await sleep(Math.max(0, next - performance.now()), undefined, {
          signal: controller.signal,
        })
        const snapshot = await requestJSON(
          firstClient.context.request,
          'get',
          '/api/v1/admin/system',
        )
        const measurement = readMemorySample(
          snapshot,
          performance.now() - started,
          previous,
        )
        if (
          samples.length &&
          samples[0]?.hostTotalBytes !== measurement.hostTotalBytes
        )
          throw new Error('Host-memory total changed during the run')
        previous = measurement.collectedAt
        samples.push(measurement)
        await write({ type: 'memory', ...measurement })
        next += sampleIntervalMs
      }
    }
    /** @param {Client} client @param {number} index */
    const work = async (client, index) => {
      let iteration = 0
      const pages = Math.floor(fixture.total / 100) - 3
      while (performance.now() < deadline && !controller.signal.aborted) {
        if (client.failure) throw new Error(client.failure)
        const row = fixture.rows[index]
        if (!row) throw new Error('Missing load-test row')
        await exerciseOnce(client.page, {
          tableId: config.tableId,
          fieldId: config.fieldId,
          rowIndex: index,
          rowId: row.id,
          scrollIndex: (3 + ((iteration + index) % pages)) * 100,
          value: `L${index}-${iteration}`,
        })
        client.edits++
        client.scrolls++
        iteration++
        const remaining = deadline - performance.now()
        if (remaining > 0)
          await sleep(Math.min(2000, remaining), undefined, {
            signal: controller.signal,
          })
      }
      if (client.failure) throw new Error(client.failure)
    }
    const tasks = [sample(), ...clients.map(work)]
    try {
      await Promise.all(tasks)
    } catch (error) {
      controller.abort()
      await Promise.allSettled(tasks)
      throw error
    }
    result = summarize(
      durationMs,
      performance.now() - started,
      clients,
      samples,
    )
    if (!result.complete)
      throw new Error(
        'Run is incomplete: duration, samples or user activity did not meet the workload requirements',
      )
    if (!result.thresholdsMet)
      throw new Error('Measured memory exceeded an AT-801 threshold')
  } catch (error) {
    failure =
      error instanceof Error ? error.message : 'Unknown load-test failure'
  } finally {
    for (const client of clients) client.stopping = true
    if (browser) {
      try {
        await browser.close()
      } catch {
        failure = [failure, 'Could not close the browser']
          .filter(Boolean)
          .join('; ')
      }
    }
    result ??= summarize(
      durationMs,
      start ? performance.now() - start : 0,
      clients,
      samples,
    )
    try {
      await write({ type: 'summary', ...result, failure: failure ?? null })
    } catch {
      failure = [failure, 'Could not write the load-test report']
        .filter(Boolean)
        .join('; ')
    }
    try {
      await report.close()
    } catch {
      failure = [failure, 'Could not close the load-test report']
        .filter(Boolean)
        .join('; ')
    }
  }
  if (failure) throw new Error(failure)
  return result
}

/** @param {string[]} args */
async function main(args) {
  if (args.length === 1 && args[0] === '--help') {
    console.log(
      'Usage: node scripts/load-test.mjs --config <private.json> --report <new.jsonl> [--duration-seconds <seconds>]',
    )
    return
  }
  /** @type {Record<string, string>} */
  const values = {}
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index]
    const value = args[index + 1]
    if (
      !key ||
      !['--config', '--report', '--duration-seconds'].includes(key) ||
      !value ||
      values[key]
    )
      throw new Error('Invalid arguments; use --help')
    values[key] = value
  }
  const configPath = values['--config']
  const reportPath = values['--report']
  if (!configPath || !reportPath)
    throw new Error('Both --config and --report are required')
  const seconds = Number(values['--duration-seconds'] ?? 900)
  if (!Number.isSafeInteger(seconds) || seconds < 1 || seconds > 3600)
    throw new Error('Duration must be between 1 and 3600 seconds')
  const config = validateConfig(JSON.parse(await readFile(configPath, 'utf8')))
  const result = await runLoad(config, reportPath, seconds * 1000)
  console.log(JSON.stringify(result))
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(
      'Load test failed:',
      error instanceof Error ? error.message : 'unknown failure',
    )
    process.exitCode = 1
  })
}
