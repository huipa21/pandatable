import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import { i18n, loadLocale } from '../i18n'
import { errorMessage, RequestError } from './client'
import enRaw from '../i18n/en.json?raw'
import zhRaw from '../i18n/zh-CN.json?raw'

beforeAll(async () => {
  await Promise.all([loadLocale('en'), loadLocale('zh-CN')])
})

afterEach(() => {
  i18n.global.locale.value = 'en'
})

describe('row API errors', () => {
  it('translates safe required, unique, and schema-change errors', () => {
    expect(errorMessage(new RequestError('ERR_REQUIRED_FIELD', 400))).toBe(
      i18n.global.t('errors.ERR_REQUIRED_FIELD'),
    )
    expect(errorMessage(new RequestError('ERR_UNIQUE_CONFLICT', 409))).toBe(
      i18n.global.t('errors.ERR_UNIQUE_CONFLICT'),
    )
    expect(errorMessage(new RequestError('ERR_SCHEMA_CHANGED', 409))).toBe(
      i18n.global.t('errors.ERR_SCHEMA_CHANGED'),
    )
  })

  it('translates relation conflict and restrict errors', () => {
    expect(errorMessage(new RequestError('ERR_RELATION_CONFLICT', 409))).toBe(
      i18n.global.t('errors.ERR_RELATION_CONFLICT'),
    )
    expect(errorMessage(new RequestError('ERR_RESTRICT_VIOLATION', 409))).toBe(
      i18n.global.t('errors.ERR_RESTRICT_VIOLATION'),
    )
  })

  it('localizes every shared error in both languages and hides unknown details', () => {
    const catalogs = {
      en: JSON.parse(enRaw) as { errors: Record<string, string> },
      'zh-CN': JSON.parse(zhRaw) as { errors: Record<string, string> },
    }
    expect(Object.keys(catalogs['zh-CN'].errors).sort()).toEqual(
      Object.keys(catalogs.en.errors).sort(),
    )
    const setupOnly = new Set(['ERR_SETUP_TOKEN', 'ERR_SETUP_COMPLETE'])
    for (const locale of ['en', 'zh-CN'] as const) {
      i18n.global.locale.value = locale
      for (const [code, expected] of Object.entries(catalogs[locale].errors)) {
        if (setupOnly.has(code)) continue
        expect(
          errorMessage(new RequestError(code, 400, { count: 3 })),
          code,
        ).toBe(expected.replaceAll('{count}', '3'))
      }
      expect(errorMessage(new Error('private server detail'))).toBe(
        i18n.global.t('errors.ERR_INTERNAL'),
      )
      expect(errorMessage(new RequestError('ERR_UNKNOWN', 500))).toBe(
        i18n.global.t('errors.ERR_INTERNAL'),
      )
    }
  })
})
