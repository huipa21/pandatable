import { ESLint } from 'eslint'
import { beforeAll, describe, expect, it } from 'vitest'
import { checkParity, messageKeys } from '../../scripts/check-i18n.mjs'
// Raw JSON is required here: Vite precompiles normal locale imports for CSP.
import en from './en.json?raw'
import zh from './zh-CN.json?raw'

describe('AT-802 i18n local gate', () => {
  const eslint = new ESLint()

  beforeAll(async () => {
    await eslint.calculateConfigForFile('src/I18nProbe.vue')
  })

  it('requires identical, non-empty message keys', () => {
    expect(checkParity(JSON.parse(en), JSON.parse(zh))).toBeGreaterThan(0)
    expect(() => checkParity({ a: 'A' }, { b: 'B' })).toThrow(
      'i18n key mismatch',
    )
    expect(() => messageKeys({ a: '' })).toThrow()
    expect(() => messageKeys({ a: ['A'] })).toThrow()
    expect(() => messageKeys({ 'a.b': 'A' })).toThrow('Invalid message key')
  })

  it('rejects hard-coded template text and visible attributes, but accepts translations', async () => {
    for (const template of [
      '<p>Untranslated text</p>',
      '<input placeholder="Untranslated hint">',
      '<button aria-label="Untranslated label"></button>',
      '<NButton label="Untranslated label" />',
      `<input :placeholder="'Untranslated hint'">`,
      `<p>{{ 'Untranslated expression' }}</p>`,
    ]) {
      const [result] = await eslint.lintText(
        `<template>${template}</template>`,
        { filePath: 'src/I18nProbe.vue' },
      )
      expect(
        result?.messages.some(
          (message) =>
            message.ruleId === '@intlify/vue-i18n/no-raw-text' ||
            message.ruleId === 'vue/no-restricted-syntax',
        ),
        template,
      ).toBe(true)
    }
    const [translated] = await eslint.lintText(
      `<template><p>{{ $t('common.appName') }}</p></template>`,
      { filePath: 'src/I18nProbe.vue' },
    )
    expect(translated?.errorCount).toBe(0)
  })
})
