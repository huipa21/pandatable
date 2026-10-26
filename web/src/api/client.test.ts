import { describe, expect, it } from 'vitest'
import { i18n } from '../i18n'
import { errorMessage, RequestError } from './client'

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
})
