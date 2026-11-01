import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  readMemorySample,
  summarize,
  validateConfig,
  validateGridView,
} from './load-test.mjs'

const id = '00000000-0000-7000-8000-000000000001'
const configuration = () => ({
  baseURL: 'https://table.example',
  tableId: id,
  viewId: id,
  fieldId: id,
  users: Array.from({ length: 10 }, (_, index) => ({
    login: `user${index}`,
    password: 'private',
  })),
})

test('requires ten distinct users and a safe origin', () => {
  assert.equal(validateConfig(configuration()).users.length, 10)
  const tooFew = configuration()
  tooFew.users.pop()
  assert.throws(() => validateConfig(tooFew), /Exactly ten/)
  const duplicates = configuration()
  const duplicate = duplicates.users[9]
  assert.ok(duplicate)
  duplicate.login = 'USER0'
  assert.throws(() => validateConfig(duplicates), /distinct/)
  for (const baseURL of [
    'http://table.example',
    'https://user:password@table.example',
    'https://table.example/path',
  ])
    assert.throws(() => validateConfig({ ...configuration(), baseURL }))
  assert.equal(
    validateConfig({ ...configuration(), baseURL: 'http://127.0.0.1:8080' })
      .baseURL,
    'http://127.0.0.1:8080',
  )
})

test('requires fresh, valid app and host measurements', () => {
  const snapshot = {
    collectedAt: '2026-11-01T00:00:00Z',
    memory: {
      status: 'ready',
      value: { usedBytes: 2_000_000_000, totalBytes: 4_000_000_000 },
    },
    appRssBytes: { status: 'ready', value: 100_000_000 },
  }
  assert.equal(readMemorySample(snapshot, 0).appRssBytes, 100_000_000)
  assert.throws(
    () => readMemorySample(snapshot, 6, snapshot.collectedAt),
    /Fresh/,
  )
  assert.throws(
    () =>
      readMemorySample(
        { ...snapshot, memory: { status: 'unavailable', value: null } },
        0,
      ),
    /Fresh/,
  )
  assert.throws(
    () =>
      readMemorySample(
        { ...snapshot, appRssBytes: { status: 'ready', value: 0 } },
        0,
      ),
    /Fresh/,
  )
})

test('requires a stable shared grid view', () => {
  const view = {
    id,
    name: 'Load view',
    type: 'grid',
    scope: 'shared',
    locked: false,
    position: 0,
    version: 1,
    config: { fields: [], frozen: 0 },
  }
  assert.doesNotThrow(() => validateGridView(view))
  assert.throws(() => validateGridView(undefined), /shared/)
  assert.throws(
    () => validateGridView({ ...view, scope: 'personal' }),
    /shared/,
  )
  assert.throws(
    () =>
      validateGridView({ ...view, config: { ...view.config, groupBy: [id] } }),
    /ungrouped/,
  )
  assert.throws(
    () =>
      validateGridView({
        ...view,
        config: { ...view.config, sort: [{ field: id, dir: 'asc' }] },
      }),
    /unsorted/,
  )
  assert.throws(
    () =>
      validateGridView({
        ...view,
        config: {
          ...view.config,
          filter: { op: 'and', items: [] },
        },
      }),
    /unfiltered/,
  )
})

test('enforces the full duration, every user, sampling gaps and strict byte limits', () => {
  const duration = 900_000
  const clients = Array.from({ length: 10 }, () => ({
    edits: 1,
    scrolls: 1,
    received: 1,
  }))
  const samples = Array.from({ length: 151 }, (_, index) => ({
    elapsedMs: index * 6000,
    appRssBytes: 149_999_999,
    hostUsedBytes: 3_199_999_999,
  }))
  assert.equal(
    summarize(duration, duration, clients, samples).thresholdsMet,
    true,
  )
  assert.equal(summarize(899_999, duration, clients, samples).complete, false)
  assert.equal(
    summarize(duration, duration - 1, clients, samples).complete,
    false,
  )
  assert.equal(
    summarize(duration, duration, clients.slice(1), samples).complete,
    false,
  )
  assert.equal(
    summarize(duration, duration, clients, samples.slice(0, -4)).complete,
    false,
  )
  const appSample = samples[50]
  const hostSample = samples[100]
  assert.ok(appSample)
  assert.ok(hostSample)
  appSample.appRssBytes = 150_000_000
  assert.equal(
    summarize(duration, duration, clients, samples).thresholdsMet,
    false,
  )
  appSample.appRssBytes = 1
  hostSample.hostUsedBytes = 3_200_000_000
  assert.equal(
    summarize(duration, duration, clients, samples).thresholdsMet,
    false,
  )
  assert.match(
    summarize(duration, duration, clients, samples).qualification,
    /independently verify/,
  )
  assert.equal(summarize(duration, 0, clients, []).appPeakBytes, null)
})
