import type { APIRequestContext } from '@playwright/test'
import type {
  AdminUserMutation,
  AuthUser,
  Navigation,
  WorkspacePreference,
  WorkspacePreferenceMutation,
} from '../src/api/types'
import en from '../src/i18n/en.json' with { type: 'json' }
import { expect, paceLogin, test } from './fixtures'

const preferencePath = '/api/v1/auth/workspace-preference'
const headers = () => ({
  Origin: process.env.PT_E2E_BASE_URL!,
  'X-PT-CSRF': '1',
})

async function readPreference(request: APIRequestContext) {
  const response = await request.get(preferencePath)
  expect(response.status()).toBe(200)
  expect(response.headers()['cache-control']).toBe('no-store')
  return (await response.json()) as WorkspacePreference
}

async function resolvePreference(request: APIRequestContext) {
  const response = await request.post(`${preferencePath}/resolve`, {
    headers: headers(),
    data: {},
  })
  expect(response.status()).toBe(200)
  const result = (await response.json()) as WorkspacePreferenceMutation
  expect(result.workspaceId).toBeTruthy()
  expect(result.eventId).toBeTruthy()
  return result.workspaceId
}

test('UX-01: default workspaces persist, respect access and recover once', async ({
  page,
  browser,
}, testInfo) => {
  test.setTimeout(150_000)
  const temporary = 'Temporary-password-123'
  const changed = 'Changed-password-123'
  const username = `ux01-${Date.now()}-${testInfo.project.name.startsWith('mobile') ? 'm' : 'd'}`
  await paceLogin(page)
  await page.goto('/login')
  await page.getByLabel(en.auth.login, { exact: true }).fill('first-admin')
  await page
    .getByLabel(en.auth.password, { exact: true })
    .fill('Setup-password-123')
  await page.getByRole('button', { name: en.auth.signIn, exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)

  const adminUser = (await (
    await page.request.get('/api/v1/auth/me')
  ).json()) as AuthUser
  const adminPreference = await readPreference(page.request)
  expect(adminPreference.workspaceId).toBeTruthy()
  const adminStarter = adminPreference.workspaceId!
  const adminNav = (await (
    await page.request.get('/api/v1/nav')
  ).json()) as Navigation
  expect(adminNav.workspaces).toContainEqual(
    expect.objectContaining({
      id: adminStarter,
      name: 'Default',
      level: 'owner',
    }),
  )

  const created = await page.request.post('/api/v1/admin/users', {
    headers: headers(),
    data: {
      username,
      displayName: 'Workspace user',
      role: 'user',
      password: temporary,
    },
  })
  expect(created.status()).toBe(201)
  const normalUser = ((await created.json()) as AdminUserMutation).user
  expect(normalUser.mustChangePassword).toBe(true)

  const userContext = await browser.newContext({
    baseURL: process.env.PT_E2E_BASE_URL,
  })
  const userRequest = userContext.request
  try {
    await paceLogin()
    let login = await userRequest.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: temporary },
    })
    expect(login.status()).toBe(200)
    expect((await userRequest.get(preferencePath)).status()).toBe(403)
    const changePassword = await userRequest.post('/api/v1/auth/password', {
      headers: headers(),
      data: { current: temporary, new: changed },
    })
    expect(changePassword.status()).toBe(200)
    await paceLogin()
    login = await userRequest.post('/api/v1/auth/login', {
      headers: headers(),
      data: { login: username, password: changed },
    })
    expect(login.status()).toBe(200)

    const userStarter = (await readPreference(userRequest)).workspaceId
    expect(userStarter).toBeTruthy()
    const userNav = (await (
      await userRequest.get('/api/v1/nav')
    ).json()) as Navigation
    expect(userNav.workspaces).toContainEqual(
      expect.objectContaining({
        id: userStarter,
        name: 'Default',
        level: 'owner',
      }),
    )
    expect(
      (
        await page.request.put(preferencePath, {
          headers: headers(),
          data: { workspaceId: userStarter },
        })
      ).status(),
    ).toBe(404)
    expect(
      (
        await userRequest.put(preferencePath, {
          headers: headers(),
          data: { workspaceId: adminStarter },
        })
      ).status(),
    ).toBe(404)

    expect(
      (
        await userRequest.patch(`/api/v1/workspaces/${userStarter}`, {
          headers: headers(),
          data: { name: 'Renamed starter' },
        })
      ).status(),
    ).toBe(200)
    expect(await resolvePreference(userRequest)).toBe(userStarter)
    const another = await userRequest.post('/api/v1/workspaces', {
      headers: headers(),
      data: { name: 'Preferred workspace' },
    })
    expect(another.status()).toBe(201)
    const anotherID = ((await another.json()) as { workspace: { id: string } })
      .workspace.id
    expect(
      (
        await userRequest.put(preferencePath, {
          headers: headers(),
          data: { workspaceId: anotherID },
        })
      ).status(),
    ).toBe(200)
    expect((await readPreference(userRequest)).workspaceId).toBe(anotherID)
    expect((await readPreference(page.request)).workspaceId).toBe(adminStarter)

    const grant = await userRequest.put('/api/v1/grants', {
      headers: headers(),
      data: {
        resourceType: 'workspace',
        resourceId: anotherID,
        principalType: 'user',
        principalId: adminUser.id,
        level: 'read',
      },
    })
    expect(grant.status()).toBe(201)
    const grantID = ((await grant.json()) as { grant: { id: string } }).grant.id
    expect(
      (
        await page.request.put(preferencePath, {
          headers: headers(),
          data: { workspaceId: anotherID },
        })
      ).status(),
    ).toBe(200)
    expect((await readPreference(userRequest)).workspaceId).toBe(anotherID)
    expect(
      (
        await userRequest.delete(`/api/v1/grants/${grantID}`, {
          headers: headers(),
        })
      ).status(),
    ).toBe(200)
    expect((await readPreference(page.request)).workspaceId).toBeNull()
    expect(await resolvePreference(page.request)).toBe(adminStarter)

    expect(
      (
        await userRequest.delete(`/api/v1/workspaces/${anotherID}`, {
          headers: headers(),
        })
      ).status(),
    ).toBe(200)
    expect((await readPreference(userRequest)).workspaceId).toBeNull()
    expect(await resolvePreference(userRequest)).toBe(userStarter)
    expect(
      (
        await userRequest.delete(`/api/v1/workspaces/${userStarter}`, {
          headers: headers(),
        })
      ).status(),
    ).toBe(200)
    const replacements = await Promise.all([
      resolvePreference(userRequest),
      resolvePreference(userRequest),
    ])
    expect(replacements[0]).toBe(replacements[1])
    expect(replacements[0]).not.toBe(userStarter)
    const replacementNav = (await (
      await userRequest.get('/api/v1/nav')
    ).json()) as Navigation
    expect(replacementNav.workspaces).toEqual([
      expect.objectContaining({
        id: replacements[0],
        name: 'Default',
        level: 'owner',
      }),
    ])
  } finally {
    await userContext.close()
  }
})
