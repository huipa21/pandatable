import { i18n } from '../i18n'
import type { Kind, Reason } from './types'

/** Localized validation message for a codec reason (static keys only). */
export function fieldErrorMessage(reason: Reason): string {
  const { t } = i18n.global
  switch (reason) {
    case 'unknown_kind':
      return t('field.error.unknown_kind')
    case 'invalid_options':
      return t('field.error.invalid_options')
    case 'invalid_type':
      return t('field.error.invalid_type')
    case 'invalid_text':
      return t('field.error.invalid_text')
    case 'too_long':
      return t('field.error.too_long')
    case 'invalid_integer':
      return t('field.error.invalid_integer')
    case 'invalid_decimal':
      return t('field.error.invalid_decimal')
    case 'invalid_number':
      return t('field.error.invalid_number')
    case 'imprecise_number':
      return t('field.error.imprecise_number')
    case 'out_of_range':
      return t('field.error.out_of_range')
    case 'too_many_decimals':
      return t('field.error.too_many_decimals')
    case 'invalid_boolean':
      return t('field.error.invalid_boolean')
    case 'invalid_date':
      return t('field.error.invalid_date')
    case 'invalid_datetime':
      return t('field.error.invalid_datetime')
    case 'nonexistent_local_time':
      return t('field.error.nonexistent_local_time')
    case 'timezone_required':
      return t('field.error.timezone_required')
    case 'unknown_choice':
      return t('field.error.unknown_choice')
    case 'duplicate_choice':
      return t('field.error.duplicate_choice')
    case 'invalid_email':
      return t('field.error.invalid_email')
    case 'invalid_url':
      return t('field.error.invalid_url')
    case 'url_scheme':
      return t('field.error.url_scheme')
    case 'read_only':
      return t('field.error.read_only')
  }
}

export function fieldKindLabel(kind: Kind): string {
  const { t } = i18n.global
  switch (kind) {
    case 'text':
      return t('field.kind.text')
    case 'long_text':
      return t('field.kind.long_text')
    case 'integer':
      return t('field.kind.integer')
    case 'decimal':
      return t('field.kind.decimal')
    case 'float':
      return t('field.kind.float')
    case 'checkbox':
      return t('field.kind.checkbox')
    case 'date':
      return t('field.kind.date')
    case 'datetime':
      return t('field.kind.datetime')
    case 'single_select':
      return t('field.kind.single_select')
    case 'multi_select':
      return t('field.kind.multi_select')
    case 'email':
      return t('field.kind.email')
    case 'url':
      return t('field.kind.url')
    case 'id':
      return t('field.kind.id')
    case 'created_at':
      return t('field.kind.created_at')
    case 'created_by':
      return t('field.kind.created_by')
    case 'updated_at':
      return t('field.kind.updated_at')
    case 'updated_by':
      return t('field.kind.updated_by')
  }
}
