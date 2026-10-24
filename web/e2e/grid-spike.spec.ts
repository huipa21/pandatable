import type { Page } from '@playwright/test'
import type { Components } from '@revolist/revogrid'
import type { WindowStats } from '../src/components/grid/window'
import { test, expect } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zh from '../src/i18n/zh-CN.json' with { type: 'json' }

type GridElement = HTMLElement & Components.RevoGrid
const path = '/_spikes/revogrid'
const stats = async (page: Page): Promise<WindowStats> =>
  JSON.parse((await page.getByTestId('window-json').textContent())!)
const cell = (page: Page, row: number, col: number) =>
  page.locator(
    `revogr-data[col-type="rgCol"] .rgCell[data-rgrow="${row}"][data-rgcol="${col}"]`,
  )

async function mount(page: Page, navigate = true) {
  if (navigate) await page.goto(path)
  await page.getByRole('button', { name: en.spike.start, exact: true }).click()
  await expect.poll(async () => (await stats(page)).pending).toBe(0)
  await expect(cell(page, 0, 0)).toHaveText('Synthetic row 1')
}

test('isolated opt-in, 200k window, bounded DOM/cache, stale navigation and unmount', async ({
  page,
  isMobile,
}, testInfo) => {
  if (isMobile) {
    await page.goto(path)
    await expect(page.getByText(en.spike.mobile)).toBeVisible()
    await expect(page.locator('revo-grid')).toHaveCount(0)
    await expect(
      page.getByRole('button', { name: en.spike.start, exact: true }),
    ).toHaveCount(0)
    await page
      .getByRole('button', { name: en.common.languages.zhCN, exact: true })
      .click()
    await expect(page.getByText(zh.spike.mobile)).toBeVisible()
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true)
    return
  }
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/login')
  expect(
    await page.evaluate(() =>
      performance
        .getEntriesByType('resource')
        .map((e) => e.name)
        .filter((url) => /DataGrid|revo-grid|revogr-/.test(url)),
    ),
  ).toEqual([])
  await page.goto(path)
  await expect(page.locator('revo-grid')).toHaveCount(0)
  const cdp = await page.context().newCDPSession(page)
  async function listeners() {
    const result: Record<string, string[]> = {}
    for (const expression of ['window', 'document']) {
      const { result: object } = await cdp.send('Runtime.evaluate', {
        expression,
        objectGroup: 't08',
      })
      if (!object.objectId) throw new Error('Missing browser listener target')
      const { listeners } = await cdp.send('DOMDebugger.getEventListeners', {
        objectId: object.objectId,
      })
      result[expression] = listeners.map((listener) => listener.type).sort()
    }
    await cdp.send('Runtime.releaseObjectGroup', { objectGroup: 't08' })
    return result
  }
  await cdp.send('HeapProfiler.collectGarbage')
  const baselineHeap = await cdp.send('Runtime.getHeapUsage')
  const mountedAt = Date.now()
  await mount(page, false)
  const firstVisibleMs = Date.now() - mountedAt
  const observations: unknown[] = []
  for (const [label, expected] of [
    [en.spike.begin, 0],
    [en.spike.middle, 1000],
    [en.spike.end, 1999],
    [en.spike.begin, 0],
  ] as const) {
    const started = Date.now()
    await page.getByRole('button', { name: label, exact: true }).click()
    await expect.poll(async () => (await stats(page)).activePage).toBe(expected)
    await expect.poll(async () => (await stats(page)).pending).toBe(0)
    await expect(page.locator('revo-grid')).toContainText(
      expected === 1999
        ? 'Synthetic row 200000'
        : `Synthetic row ${expected * 100 + 1}`,
    )
    const state = await stats(page)
    const dom = await page.locator('revo-grid').evaluate((element) => ({
      rows: element.querySelectorAll('.rgRow').length,
      cells: element.querySelectorAll('.rgCell').length,
      nodes: element.querySelectorAll('*').length,
      width: element.clientWidth,
      height: element.clientHeight,
    }))
    expect(state.cachedRows).toBeLessThanOrEqual(300)
    expect(dom.rows).toBeLessThan(60)
    expect(dom.cells).toBeLessThan(200)
    const materialized = await page
      .locator('revo-grid')
      .evaluate(async (element) => {
        const source = await (element as GridElement).getSource()
        return {
          length: source.length,
          loaded: source.filter((row) => row.id !== 0).length,
          unique: new Set(source).size,
        }
      })
    expect(materialized.length).toBe(200_000)
    expect(materialized.loaded).toBeLessThanOrEqual(300)
    expect(materialized.unique).toBeLessThanOrEqual(301)
    const heap = await cdp.send('Runtime.getHeapUsage')
    await cdp.send('HeapProfiler.collectGarbage')
    observations.push({
      label,
      elapsedMs: Date.now() - started,
      state,
      dom,
      materialized,
      heap,
      retainedHeap: await cdp.send('Runtime.getHeapUsage'),
    })
  }
  // Real overlapping requests are cancelled; older unit-test responses also ignore cancellation.
  await page.getByRole('button', { name: en.spike.middle, exact: true }).click()
  await page.getByRole('button', { name: en.spike.end, exact: true }).click()
  await expect.poll(async () => (await stats(page)).activePage).toBe(1999)
  await expect.poll(async () => (await stats(page)).pending).toBe(0)
  await expect(page.locator('revo-grid')).toContainText('Synthetic row 200000')
  expect(
    (await stats(page)).requests.some(
      (request) => request.status === 'cancelled',
    ),
  ).toBe(true)
  await page.getByRole('button', { name: en.spike.middle, exact: true }).click()
  await page.getByRole('button', { name: en.spike.stop, exact: true }).click()
  await expect(page.locator('revo-grid')).toHaveCount(0)
  expect(await stats(page)).toMatchObject({
    disposed: true,
    cachedRows: 0,
    pending: 0,
    editedCells: 0,
  })
  await cdp.send('HeapProfiler.collectGarbage')
  const unmountedHeap = await cdp.send('Runtime.getHeapUsage')
  const unmountedListeners = await listeners()
  for (let cycle = 0; cycle < 2; cycle++) {
    await page
      .getByRole('button', { name: en.spike.start, exact: true })
      .click()
    await expect(cell(page, 0, 0)).toHaveText('Synthetic row 1')
    await page.getByRole('button', { name: en.spike.stop, exact: true }).click()
    await expect(page.locator('revo-grid')).toHaveCount(0)
  }
  expect(await listeners()).toEqual(unmountedListeners)
  const memory = {
    baselineHeap,
    firstVisibleMs,
    unmountedHeap,
    unmountedListeners,
  }
  await testInfo.attach('browser-memory', {
    body: JSON.stringify(memory, null, 2),
    contentType: 'application/json',
  })
  await testInfo.attach('200k-window-observations', {
    body: JSON.stringify(observations, null, 2),
    contentType: 'application/json',
  })
  console.log('T0.8 WINDOW', JSON.stringify({ memory, observations }))
})

test('wrapper editing, keyboard ranges, quoted TSV, Ctrl+D, fill handle, grouping and theme', async ({
  page,
  context,
  isMobile,
}, testInfo) => {
  test.skip(
    isMobile,
    'Desktop clipboard/fill only by D-016; mobile isolation is tested separately',
  )
  await page.setViewportSize({ width: 1440, height: 1000 })
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await mount(page)
  const grid = page.locator('revo-grid')
  await cell(page, 0, 1).click()
  await page.keyboard.press('Enter')
  const note = page.getByRole('textbox', { name: en.spike.noteEditor })
  await expect(note).toBeVisible()
  await note.fill('first line\nsaid "yes"\tvalue')
  await note.press('Enter')
  await expect(cell(page, 0, 1)).toHaveText('first line\nsaid "yes"\tvalue')
  await cell(page, 0, 1).dblclick()
  await note.fill('cancel this')
  await note.press('Escape')
  await expect(cell(page, 0, 1)).not.toHaveText('cancel this')
  await grid.evaluate(async (element) =>
    (element as GridElement).setCellsFocus({ x: 1, y: 0 }, { x: 1, y: 0 }),
  )
  await page.keyboard.press('Control+c')
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  expect(copied).toBe('"first line\nsaid ""yes""\tvalue"')
  await grid.evaluate(async (element) =>
    (element as GridElement).setCellsFocus({ x: 1, y: 1 }, { x: 1, y: 1 }),
  )
  await page.keyboard.press('Control+v')
  await expect(cell(page, 1, 1)).toHaveText('first line\nsaid "yes"\tvalue')
  await page.keyboard.press('Shift+ArrowDown')
  await expect
    .poll(async () =>
      grid.evaluate(async (element) =>
        (element as GridElement).getSelectedRange(),
      ),
    )
    .toMatchObject({ y: 1, y1: 2 })
  await page.keyboard.press('Control+d')
  await expect(cell(page, 2, 1)).toHaveText('first line\nsaid "yes"\tvalue')
  const handle = grid.locator(
    'revogr-overlay-selection[coltype="rgCol"][type="rgRow"] .autofill-handle',
  )
  await expect(handle).toBeVisible()
  const from = await handle.boundingBox()
  const to = await cell(page, 4, 1).boundingBox()
  expect(from).not.toBeNull()
  expect(to).not.toBeNull()
  await page.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2)
  await page.mouse.down()
  await page.mouse.move(to!.x + to!.width / 2, to!.y + to!.height / 2, {
    steps: 12,
  })
  await page.mouse.up()
  await expect(cell(page, 4, 1)).toHaveText('first line\nsaid "yes"\tvalue')
  const selectFrom = await cell(page, 6, 0).boundingBox()
  const selectTo = await cell(page, 8, 1).boundingBox()
  await page.mouse.move(selectFrom!.x + 20, selectFrom!.y + 15)
  await page.mouse.down()
  await page.mouse.move(selectTo!.x + 20, selectTo!.y + 15, { steps: 12 })
  await page.mouse.up()
  await expect
    .poll(async () =>
      grid.evaluate(async (element) =>
        (element as GridElement).getSelectedRange(),
      ),
    )
    .toMatchObject({ x: 0, x1: 1, y: 6, y1: 8 })
  await testInfo.attach('quoted-tsv', {
    body: copied,
    contentType: 'text/plain',
  })
  await page.keyboard.press('Control+a')
  await page.keyboard.press('Control+c')
  await expect(page.getByRole('alert')).toHaveText(en.spike.copyError)
  const colors = () =>
    grid
      .locator('.rgCell')
      .first()
      .evaluate((element) => ({
        color: getComputedStyle(element).color,
        background: getComputedStyle(element).backgroundColor,
      }))
  const light = await colors()
  await page
    .getByRole('button', { name: en.common.themes.dark, exact: true })
    .click()
  await expect(grid).toHaveAttribute('theme', 'darkCompact')
  await expect.poll(colors).not.toEqual(light)
  await testInfo.attach('dark-grid', {
    body: await grid.screenshot(),
    contentType: 'image/png',
  })
  await page
    .getByRole('button', { name: en.common.languages.zhCN, exact: true })
    .click()
  await expect(grid).toContainText(zh.spike.fields.note)
  await page
    .getByRole('button', { name: zh.common.themes.light, exact: true })
    .click()
  await expect(grid).toHaveAttribute('theme', 'compact')
  await page.setViewportSize({ width: 1024, height: 1000 })
  const pinned = grid
    .locator('revogr-data[col-type="colPinStart"] .rgCell')
    .first()
  const pinnedBefore = await pinned.boundingBox()
  await grid.evaluate(async (element) =>
    (element as GridElement).scrollToColumnProp('group'),
  )
  await expect(
    grid
      .locator('revogr-header')
      .getByRole('columnheader', { name: zh.spike.fields.group, exact: true }),
  ).toBeVisible()
  const pinnedAfter = await pinned.boundingBox()
  expect(pinnedAfter!.x).toBe(pinnedBefore!.x)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true)
  await page.getByRole('checkbox', { name: zh.spike.groupSample }).check()
  await expect.poll(async () => (await stats(page)).total).toBe(100)
  await expect(grid).toContainText(zh.spike.groupA)
  await page
    .getByRole('button', {
      name: zh.spike.collapse.replace('{name}', zh.spike.groupA),
    })
    .click()
  await expect(grid).toContainText(zh.spike.groupB)
  await page
    .getByRole('button', {
      name: zh.spike.collapse.replace('{name}', zh.spike.groupB),
    })
    .click()
  await expect
    .poll(async () =>
      grid.evaluate(
        async (element) =>
          (await (element as GridElement).getVisibleSource()).length,
      ),
    )
    .toBe(2)
  await page
    .getByRole('button', {
      name: zh.spike.expand.replace('{name}', zh.spike.groupA),
    })
    .click()
  await expect
    .poll(async () =>
      grid.evaluate(
        async (element) =>
          (await (element as GridElement).getVisibleSource()).length,
      ),
    )
    .toBe(52)
  await testInfo.attach('light-grouped-grid', {
    body: await grid.screenshot(),
    contentType: 'image/png',
  })
  await testInfo.attach('grouping-markup', {
    body: (await grid.innerHTML()).slice(0, 16000),
    contentType: 'text/plain',
  })
})

test.describe('tablet touch (not mobile cards)', () => {
  test.use({
    viewport: { width: 1024, height: 1000 },
    hasTouch: true,
    isMobile: false,
  })
  test('tap focus and actual touch swipe are observed at tablet width', async ({
    page,
    context,
  }, testInfo) => {
    test.skip(
      testInfo.project.name !== 'desktop-chromium',
      'One explicit tablet observation, not a second mobile-grid project',
    )
    await mount(page)
    const grid = page.locator('revo-grid')
    await cell(page, 0, 0).tap()
    const focus = await grid.evaluate(async (element) =>
      (element as GridElement).getFocused(),
    )
    expect(focus?.cell).toEqual({ x: 0, y: 0 })
    const cdp = await context.newCDPSession(page)
    const box = await grid.boundingBox()
    const x = box!.x + 450
    const y = box!.y + 350
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x, y }],
    })
    for (let step = 1; step <= 10; step++) {
      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchMove',
        touchPoints: [{ x, y: y - step * 22 }],
      })
    }
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    })
    const scroll = await grid
      .locator('revogr-viewport-scroll.rgCol .vertical-inner')
      .evaluate((element) => element.scrollTop)
    const observation = {
      viewport: page.viewportSize(),
      focus: focus?.cell,
      scrollTopAfterTouchSwipe: scroll,
    }
    await testInfo.attach('tablet-touch', {
      body: JSON.stringify(observation),
      contentType: 'application/json',
    })
    console.log('T0.8 TABLET', JSON.stringify(observation))
    await page.setViewportSize({ width: 700, height: 1000 })
    await expect(page.locator('revo-grid')).toHaveCount(0)
    await expect(page.getByText(en.spike.mobile)).toBeVisible()
  })
})
