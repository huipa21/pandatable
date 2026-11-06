import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import type { Page } from '@playwright/test'
import { test, expect, expectHTTPFailure, paceLogin } from './fixtures'
import en from '../src/i18n/en.json' with { type: 'json' }
import zhCN from '../src/i18n/zh-CN.json' with { type: 'json' }

const engine = process.env.PT_E2E_ENGINE!
const app = process.env.PT_E2E_APP_CONTAINER!
const baseURL = process.env.PT_E2E_BASE_URL!
const project = process.env.COMPOSE_PROJECT_NAME!
const headers = () => ({ Origin: baseURL, 'X-PT-CSRF': '1' })
// App logs grow across a full production run; keep them above the 1 MiB default.
const run = (...args: string[]) =>
  execFileSync(engine, args, {
    encoding: 'utf8',
    maxBuffer: 64 * 1024 * 1024,
  }).trim()

interface CapturedMail {
  recipient: string
  data: string
}

function messages(file: string): CapturedMail[] {
  const contents = run('exec', app, 'cat', file)
  return contents
    ? contents.split('\n').map((line) => JSON.parse(line) as CapturedMail)
    : []
}

async function login(page: Page, username: string, password: string) {
  await paceLogin(page)
  const response = await page.request.post('/api/v1/auth/login', {
    headers: headers(),
    data: { login: username, password },
  })
  expect(response.status()).toBe(200)
}

test('AT-904: SMTP settings, password recovery and opt-in mentions', async ({
  page,
  browser,
  isMobile,
}) => {
  test.setTimeout(180_000)
  if (isMobile) await page.setViewportSize({ width: 390, height: 844 })
  const file = `/data/tmp/pt-smtp-${isMobile ? 'mobile' : 'desktop'}.jsonl`
  const script = fs.readFileSync(path.resolve('e2e', 'smtp-sink.mjs'), 'utf8')
  let sink = run(
    'run',
    '-d',
    '--rm',
    '--network',
    `container:${app}`,
    '--volumes-from',
    app,
    '--user',
    '10001:10001',
    '--env',
    `PT_MAIL_SINK_FILE=${file}`,
    '--entrypoint',
    'node',
    'docker.io/library/node:24.15.0-bookworm-slim',
    '--input-type=module',
    '-e',
    script,
  )
  try {
    await expect.poll(() => run('logs', sink)).toContain('READY')
    await login(page, 'first-admin', 'Setup-password-123')
    await page.goto('/admin/smtp')
    await expect(
      page.getByRole('heading', { name: en.mail.adminTitle }),
    ).toBeVisible()
    await page.getByLabel(en.mail.enabled).check()
    await page.getByLabel(en.mail.host).fill('127.0.0.1')
    await page.getByLabel(en.mail.port).fill('2525')
    await page.getByLabel(en.mail.security).selectOption('none')
    await page.getByLabel(en.mail.username).fill('test-user')
    await page.getByLabel(en.mail.password).fill('test-password')
    await page.getByLabel(en.mail.fromAddress).fill('sender@example.test')
    await page.getByRole('button', { name: en.mail.save }).click()
    await expect(page.getByRole('status')).toContainText(en.mail.saved)
    const stored = await page.request.get('/api/v1/admin/smtp')
    expect(stored.status()).toBe(200)
    const storedText = await stored.text()
    expect(storedText).not.toContain('test-user')
    expect(storedText).not.toContain('test-password')
    expect(JSON.parse(storedText)).toMatchObject({
      hasUsername: true,
      hasPassword: true,
    })
    await page.getByLabel(en.mail.testAddress).fill('test@example.test')
    const sendTest = page.getByRole('button', { name: en.mail.sendTest })
    if (isMobile) await sendTest.tap()
    else await sendTest.click()
    await expect(page.getByRole('status')).toContainText(en.mail.testSent)
    await expect.poll(() => messages(file).length).toBe(1)
    expect(messages(file)[0]!.recipient).toBe('test@example.test')
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true)

    await page.getByLabel(en.mail.password).fill('wrong-password')
    await page.getByRole('button', { name: en.mail.save }).click()
    await expect(page.getByRole('status')).toContainText(en.mail.saved)
    expectHTTPFailure(page, 'POST', '/api/v1/admin/smtp/test', 502)
    await page.getByRole('button', { name: en.mail.sendTest }).click()
    await expect(page.getByRole('alert')).toContainText(en.errors.ERR_SMTP_SEND)
    await page.getByLabel(en.mail.password).fill('test-password')
    await page.getByRole('button', { name: en.mail.save }).click()
    await expect(page.getByRole('status')).toContainText(en.mail.saved)

    if (isMobile) {
      const guest = await browser.newContext({
        baseURL,
        viewport: { width: 390, height: 844 },
        colorScheme: 'dark',
      })
      try {
        const guestPage = await guest.newPage()
        await guestPage.addInitScript(() => {
          localStorage.setItem('pandatable.locale', 'zh-CN')
          localStorage.setItem('pandatable.theme', 'dark')
        })
        await guestPage.goto('/forgot-password')
        await expect(guestPage.getByLabel(zhCN.mail.email)).toBeVisible()
        await expect(guestPage.locator('html')).toHaveAttribute('lang', 'zh-CN')
        await expect(guestPage.locator('html')).toHaveAttribute(
          'data-theme',
          'dark',
        )
        expect(
          await guestPage.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true)
      } finally {
        await guest.close()
      }
      return
    }

    const username = 'mail-recipient'
    const email = 'mail-recipient@example.test'
    const createUser = async (name: string, address: string) => {
      const response = await page.request.post('/api/v1/admin/users', {
        headers: headers(),
        data: {
          username: name,
          email: address,
          displayName: name,
          role: 'user',
          password: 'Temporary-password-123',
        },
      })
      expect(response.status()).toBe(201)
      return (await response.json()).user.id as string
    }
    const userId = await createUser(username, email)
    const outsiderId = await createUser(
      'mail-outsider',
      'mail-outsider@example.test',
    )
    const db = run(
      'ps',
      '--filter',
      `label=com.docker.compose.project=${project}`,
      '--filter',
      'label=com.docker.compose.service=db',
      '--format',
      '{{.ID}}',
    )
    expect(db).toMatch(/^[a-f0-9]+$/)
    expect(userId).toMatch(/^[a-f0-9-]{36}$/)
    expect(outsiderId).toMatch(/^[a-f0-9-]{36}$/)
    const sql = (statement: string) =>
      run(
        'exec',
        db,
        'psql',
        '-U',
        'pandatable',
        '-d',
        'pandatable',
        '-t',
        '-A',
        '-c',
        statement,
      )
    sql(`UPDATE meta.users SET mention_email=true WHERE id='${outsiderId}'`)

    const userContext = await browser.newContext({ baseURL })
    const resetContext = await browser.newContext({ baseURL })
    try {
      const userPage = await userContext.newPage()
      const resetPage = await resetContext.newPage()
      await login(userPage, username, 'Temporary-password-123')
      const requestReset = (address: string) =>
        page.request.post('/api/v1/auth/reset/request', {
          headers: headers(),
          data: { email: address },
        })
      const existing = await requestReset(email)
      const unknown = await requestReset('unknown@example.test')
      expect(existing.status()).toBe(200)
      expect(unknown.status()).toBe(200)
      expect(await existing.json()).toEqual(await unknown.json())
      await expect
        .poll(
          () =>
            messages(file).filter((item) => item.recipient === email).length,
        )
        .toBe(1)
      const tokenFrom = (mail: CapturedMail) => {
        const decoded = mail.data
          .replace(/=\r?\n/g, '')
          .replace(/=([0-9A-F]{2})/gi, (_, hex: string) =>
            String.fromCharCode(Number.parseInt(hex, 16)),
          )
        const match = decoded.match(
          /https?:\/\/[^\s]+\/reset-password#token=([A-Za-z0-9_-]{43})/,
        )
        if (!match) throw new Error('reset email has no valid link')
        return { link: match[0], token: match[1]! }
      }
      const expired = tokenFrom(
        messages(file).find((item) => item.recipient === email)!,
      )
      sql(
        `UPDATE meta.password_resets SET expires_at=now()-interval '1 second' WHERE user_id='${userId}'`,
      )
      const complete = (token: string) =>
        page.request.post('/api/v1/auth/reset/complete', {
          headers: headers(),
          data: { token, password: 'Replacement-password-123' },
        })
      expect((await complete(expired.token)).status()).toBe(400)
      await resetPage.goto('/forgot-password')
      const emailInput = resetPage.getByLabel(en.mail.email)
      await emailInput.fill(email)
      await emailInput.focus()
      await resetPage.keyboard.press('Tab')
      await expect(
        resetPage.getByRole('button', { name: en.mail.requestReset }),
      ).toBeFocused()
      await resetPage.keyboard.press('Enter')
      await expect(resetPage.getByRole('status')).toContainText(
        en.mail.requestSent,
      )
      await expect
        .poll(
          () =>
            messages(file).filter((item) => item.recipient === email).length,
        )
        .toBe(2)
      const valid = tokenFrom(
        messages(file).filter((item) => item.recipient === email)[1]!,
      )
      await resetPage.goto(valid.link)
      await expect(resetPage).toHaveURL(/\/reset-password$/)
      await resetPage
        .getByLabel(en.auth.newPassword, { exact: true })
        .fill('Replacement-password-123')
      await resetPage
        .getByLabel(en.auth.confirmPassword, { exact: true })
        .fill('Replacement-password-123')
      await resetPage.getByRole('button', { name: en.mail.resetSubmit }).click()
      await expect(resetPage).toHaveURL(/\/login$/)
      expect((await userPage.request.get('/api/v1/auth/me')).status()).toBe(401)
      expect((await complete(valid.token)).status()).toBe(400)
      await login(userPage, username, 'Replacement-password-123')
      await userPage.goto('/profile')
      const optIn = userPage.getByLabel(en.mail.mentionOptIn)
      await expect(optIn).not.toBeChecked()

      const workspace = await page.request.post('/api/v1/workspaces', {
        headers: headers(),
        data: { name: 'Mail workspace' },
      })
      expect(workspace.status()).toBe(201)
      const workspaceId = (await workspace.json()).workspace.id as string
      const book = await page.request.post(
        `/api/v1/workspaces/${workspaceId}/books`,
        {
          headers: headers(),
          data: { name: 'Mail book' },
        },
      )
      const bookId = (await book.json()).book.id as string
      const table = await page.request.post(`/api/v1/books/${bookId}/tables`, {
        headers: headers(),
        data: { name: 'Mail table' },
      })
      const tableId = (await table.json()).table.id as string
      const row = await page.request.post(`/api/v1/tables/${tableId}/rows`, {
        headers: headers(),
        data: { rows: [{ fields: {} }] },
      })
      const rowId = (await row.json()).rows[0].id as string
      const grant = await page.request.put('/api/v1/grants', {
        headers: headers(),
        data: {
          resourceType: 'table',
          resourceId: tableId,
          principalType: 'user',
          principalId: userId,
          level: 'read',
        },
      })
      expect(grant.status()).toBe(201)
      const comment = (body: string) =>
        page.request.post(`/api/v1/tables/${tableId}/rows/${rowId}/comments`, {
          headers: headers(),
          data: { body },
        })
      expect((await comment(`@${username} off`)).status()).toBe(201)
      expect(sql('SELECT count(*) FROM meta.mention_mail_jobs')).toBe('0')
      await page.waitForTimeout(1500)
      expect(
        messages(file).filter((item) => item.recipient === email),
      ).toHaveLength(2)
      await optIn.check()
      await userPage.getByRole('button', { name: en.profile.save }).click()
      await expect(userPage.getByRole('status')).toContainText(en.profile.saved)
      const ownerPreference = await page.request.patch('/api/v1/auth/me', {
        headers: headers(),
        data: { mentionEmail: true },
      })
      expect(ownerPreference.status()).toBe(200)
      expect(
        (await comment(`@${username} @first-admin @mail-outsider on`)).status(),
      ).toBe(201)
      await expect
        .poll(
          () =>
            messages(file).filter((item) => item.recipient === email).length,
        )
        .toBe(3)
      await expect
        .poll(() => sql('SELECT count(*) FROM meta.mention_mail_jobs'))
        .toBe('0')
      expect(
        messages(file).some(
          (item) => item.recipient === 'mail-outsider@example.test',
        ),
      ).toBe(false)
      expect(
        messages(file).some((item) => item.recipient === 'owner@example.test'),
      ).toBe(false)

      run('stop', '--time', '1', sink)
      sink = ''
      expect((await comment(`@${username} delivery failure`)).status()).toBe(
        201,
      )
      await expect
        .poll(() => run('logs', app))
        .toContain('mention email delivery failed')
      const notifications = await userPage.request.get('/api/v1/notifications')
      expect((await notifications.json()).unreadCount).toBe(3)
      const optOut = await userPage.request.patch('/api/v1/auth/me', {
        headers: headers(),
        data: { mentionEmail: false },
      })
      expect(optOut.status()).toBe(200)
      sql('UPDATE meta.mention_mail_jobs SET next_attempt_at=now()')
      await expect
        .poll(() => sql('SELECT count(*) FROM meta.mention_mail_jobs'))
        .toBe('0')
      const logs = run('logs', app)
      expect(logs).not.toContain('test-user')
      expect(logs).not.toContain('test-password')
      expect(logs).not.toContain('wrong-password')
      expect(logs).not.toContain(expired.token)
      expect(logs).not.toContain(valid.token)
    } finally {
      await resetContext.close()
      await userContext.close()
    }
  } finally {
    if (sink) run('stop', '--time', '1', sink)
    run('exec', app, 'rm', '-f', file)
  }
})
