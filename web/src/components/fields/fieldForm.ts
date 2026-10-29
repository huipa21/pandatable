// Pure Field-editor logic (D-130): drafts, request bodies and error text. Lives
// in the lazy field chunk with the registry; the server stays authoritative.
import { errorMessage, RequestError } from '../../api/client'
import type { TableField } from '../../api/types'
import { i18n } from '../../i18n'
import { CHOICE_COLORS } from '../../fields/choice'
import { fieldErrorMessage } from '../../fields/messages'
import { bindField, type BoundField } from '../../fields/registry'
import type {
  Choice,
  FieldEnv,
  Kind,
  Reason,
  WireValue,
} from '../../fields/types'

export const USER_KINDS = [
  'text',
  'long_text',
  'integer',
  'decimal',
  'float',
  'checkbox',
  'date',
  'datetime',
  'single_select',
  'multi_select',
  'time',
  'duration',
  'currency',
  'percent',
  'rating',
  'phone',
  'uuid',
  'json',
  'inet',
  'cidr',
  'text_array',
  'int_array',
  'user',
  'multi_user',
  'autonumber',
  'attachment',
  'email',
  'url',
] as const satisfies readonly Kind[]

/** Options that change the physical column type; fixed after creation. */
export const STORAGE_OPTIONS: Partial<Record<Kind, readonly string[]>> = {
  text: ['maxLength'],
  integer: ['size'],
  decimal: ['precision', 'scale'],
  float: ['size'],
  currency: ['scale'],
  rating: ['max'],
}

export const MAX_DESCRIPTION = 1000

export interface FieldDraft {
  name: string
  description: string
  kind: Kind
  maxLength: string
  size: string
  precision: string
  scale: string
  currency: string
  ratingMax: string
  displayDecimals: string
  thousandSep: boolean
  dateFormat: string
  timeFormat: string
  durationFormat: string
  timezone: string
  fixedTz: string
  autoGenerate: boolean
  autoNumberPrefix: string
  autoNumberPadding: string
  attachmentAccept: string
  attachmentMaxFiles: string
  choices: Choice[]
  isRequired: boolean
  isUnique: boolean
  /** Free-text default for non-select, non-checkbox kinds. */
  defaultText: string
  /** Selected choice ids (stable across label edits). */
  defaultChoices: string[]
  defaultBool: '' | 'true' | 'false'
  fillEmpty: boolean
}

const text = (value: unknown, fallback: string) =>
  typeof value === 'string' || typeof value === 'number'
    ? String(value)
    : fallback

export function newChoiceId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(9))
  return 'c' + [...bytes].map((b) => b.toString(36).padStart(2, '0')).join('')
}

export function newChoice(label = ''): Choice {
  return { id: newChoiceId(), label, color: CHOICE_COLORS[0] }
}

/** A draft for a new field, or one seeded from an existing field's metadata. */
export function draftFrom(field: TableField | null, env: FieldEnv): FieldDraft {
  const options = (field?.options ?? {}) as Record<string, unknown>
  const kind = (field?.uiType ?? 'text') as Kind
  const draft: FieldDraft = {
    name: field?.name ?? '',
    description: field?.description ?? '',
    kind,
    maxLength: text(options.maxLength, ''),
    size: text(options.size, kind === 'float' ? 'double' : 'int'),
    precision: text(options.precision, '18'),
    scale: text(options.scale, '2'),
    currency: text(options.currency, ''),
    ratingMax: text(options.max, '5'),
    displayDecimals: text(options.displayDecimals, ''),
    thousandSep: options.thousandSep !== false,
    dateFormat: text(options.dateFormat, 'iso'),
    timeFormat: text(options.timeFormat, '24h'),
    durationFormat: text(options.format, 'h:mm'),
    timezone: text(options.timezone, 'user'),
    fixedTz: text(options.fixedTz, ''),
    autoGenerate: options.autoGenerate === true,
    autoNumberPrefix: text(options.prefix, ''),
    autoNumberPadding: text(options.padding, '0'),
    attachmentAccept: text(options.accept, ''),
    attachmentMaxFiles: text(options.maxFiles, '10'),
    choices: Array.isArray(options.choices)
      ? (options.choices as Choice[]).map((choice) => ({ ...choice }))
      : [],
    isRequired: field?.isRequired ?? false,
    isUnique: field?.isUnique ?? false,
    defaultText: '',
    defaultChoices: [],
    defaultBool: '',
    fillEmpty: false,
  }
  const value = field?.default ?? null
  if (kind === 'checkbox')
    draft.defaultBool = value === true ? 'true' : value === false ? 'false' : ''
  else if (kind === 'single_select' || kind === 'multi_select') {
    const labels =
      typeof value === 'string' ? [value] : Array.isArray(value) ? value : []
    draft.defaultChoices = draft.choices
      .filter((choice) => labels.includes(choice.label))
      .map((choice) => choice.id)
  } else if (value !== null && field) {
    const bound = bindField(kind, field.options)
    draft.defaultText = bound.ok
      ? bound.value.formatCopy(value, env)
      : String(value)
  }
  return draft
}

const integer = (value: string) => {
  const trimmed = value.trim()
  return /^\d{1,6}$/.test(trimmed) ? Number(trimmed) : trimmed
}

/** Raw API options for a draft; invalid entries are kept for validation. */
export function draftOptions(draft: FieldDraft): Record<string, unknown> {
  switch (draft.kind) {
    case 'text':
      return draft.maxLength.trim()
        ? { maxLength: integer(draft.maxLength) }
        : {}
    case 'integer':
      return { size: draft.size, thousandSep: draft.thousandSep }
    case 'decimal':
      return {
        precision: integer(draft.precision),
        scale: integer(draft.scale),
        thousandSep: draft.thousandSep,
      }
    case 'currency':
      return {
        ...(draft.currency.trim() ? { currency: draft.currency.trim() } : {}),
        scale: integer(draft.scale),
      }
    case 'percent':
      return draft.displayDecimals.trim()
        ? { displayDecimals: integer(draft.displayDecimals) }
        : {}
    case 'rating':
      return { max: integer(draft.ratingMax), icon: 'star' }
    case 'float':
      return draft.displayDecimals.trim()
        ? {
            size: draft.size,
            thousandSep: draft.thousandSep,
            displayDecimals: integer(draft.displayDecimals),
          }
        : { size: draft.size, thousandSep: draft.thousandSep }
    case 'date':
      return { dateFormat: draft.dateFormat }
    case 'datetime':
      return draft.timezone === 'fixed'
        ? {
            dateFormat: draft.dateFormat,
            timeFormat: draft.timeFormat,
            timezone: 'fixed',
            fixedTz: draft.fixedTz.trim(),
          }
        : {
            dateFormat: draft.dateFormat,
            timeFormat: draft.timeFormat,
            timezone: draft.timezone,
          }
    case 'time':
      return { format: draft.timeFormat }
    case 'duration':
      return { format: draft.durationFormat }
    case 'uuid':
      return draft.autoGenerate ? { autoGenerate: true } : {}
    case 'autonumber':
      return {
        ...(draft.autoNumberPrefix ? { prefix: draft.autoNumberPrefix } : {}),
        padding: integer(draft.autoNumberPadding),
      }
    case 'attachment':
      return {
        ...(draft.attachmentAccept.trim()
          ? { accept: draft.attachmentAccept.trim() }
          : {}),
        maxFiles: integer(draft.attachmentMaxFiles),
      }
    case 'single_select':
    case 'multi_select':
      return {
        choices: draft.choices.map((choice) => ({
          id: choice.id,
          label: choice.label,
          color: choice.color,
        })),
      }
    default:
      return {}
  }
}

type Checked<T> = { ok: true; value: T } | { ok: false; message: string }

const failed = (reason: Reason, option?: string): Checked<never> => ({
  ok: false,
  message:
    option === 'default'
      ? i18n.global.t('field.panel.defaultInvalid', {
          message: fieldErrorMessage(reason),
        })
      : fieldErrorMessage(reason),
})

export function bindDraft(draft: FieldDraft): Checked<BoundField> {
  const bound = bindField(draft.kind, draftOptions(draft))
  return bound.ok ? bound : failed(bound.reason, bound.option)
}

/** The canonical wire default of a draft, or null for no default. */
export function draftDefault(
  draft: FieldDraft,
  bound: BoundField,
  env: FieldEnv,
): Checked<WireValue> {
  if (draft.kind === 'checkbox')
    return {
      ok: true,
      value: draft.defaultBool === '' ? null : draft.defaultBool === 'true',
    }
  if (draft.kind === 'single_select' || draft.kind === 'multi_select') {
    const labels = draft.choices
      .filter((choice) => draft.defaultChoices.includes(choice.id))
      .map((choice) => choice.label)
    if (labels.length === 0) return { ok: true, value: null }
    const raw = draft.kind === 'single_select' ? labels[0]! : labels
    const value = bound.normalize(raw)
    return value.ok ? value : failed(value.reason, 'default')
  }
  if (draft.defaultText.trim() === '') return { ok: true, value: null }
  const value = bound.parsePaste(draft.defaultText, env)
  return value.ok ? value : failed(value.reason, 'default')
}

const validName = (name: string) => {
  const trimmed = name.trim()
  const count = [...trimmed].length
  return count >= 1 && count <= 100 && !/\p{Cc}/u.test(trimmed)
}

const validDescription = (description: string) =>
  [...description.trim()].length <= MAX_DESCRIPTION &&
  !/[^\P{Cc}\n\t]/u.test(description.trim())

export function draftProblem(draft: FieldDraft): string | null {
  const { t } = i18n.global
  if (!validName(draft.name)) return t('field.panel.nameInvalid')
  if (!validDescription(draft.description))
    return t('field.panel.descriptionInvalid')
  return null
}

const same = (left: unknown, right: unknown) =>
  JSON.stringify(left ?? null) === JSON.stringify(right ?? null)

export type FieldBody = Record<string, unknown>

/** POST body for a new field. */
export function createBody(
  draft: FieldDraft,
  env: FieldEnv,
): Checked<FieldBody> {
  const problem = draftProblem(draft)
  if (problem) return { ok: false, message: problem }
  const bound = bindDraft(draft)
  if (!bound.ok) return bound
  const value = draftDefault(draft, bound.value, env)
  if (!value.ok) return value
  const { constraints } = bound.value
  const body: FieldBody = {
    name: draft.name.trim(),
    uiType: draft.kind,
    description: draft.description.trim(),
    options:
      draft.kind === 'currency' && !draft.currency.trim()
        ? draftOptions(draft)
        : bound.value.options,
    isRequired: constraints.required && draft.isRequired,
    isUnique: constraints.unique && draft.isUnique,
  }
  if (value.value !== null) body.default = value.value
  return { ok: true, value: body }
}

/** PATCH body with only changed members; system fields allow name/description. */
export function patchBody(
  field: TableField,
  draft: FieldDraft,
  env: FieldEnv,
): Checked<FieldBody> {
  const problem = draftProblem(draft)
  if (problem) return { ok: false, message: problem }
  const body: FieldBody = {}
  if (draft.name.trim() !== field.name) body.name = draft.name.trim()
  if (draft.description.trim() !== field.description)
    body.description = draft.description.trim()
  if (!field.isSystem) {
    const bound = bindDraft(draft)
    if (!bound.ok) return bound
    const previous = bindField(field.uiType, field.options)
    if (!previous.ok || !same(previous.value.options, bound.value.options))
      body.options = bound.value.options
    const value = draftDefault(draft, bound.value, env)
    if (!value.ok) return value
    if (!same(value.value, field.default)) body.default = value.value
    const { constraints } = bound.value
    if (constraints.required && draft.isRequired !== field.isRequired)
      body.isRequired = draft.isRequired
    if (constraints.unique && draft.isUnique !== field.isUnique)
      body.isUnique = draft.isUnique
    if (body.isRequired === true && draft.fillEmpty) body.fillEmpty = true
  }
  if (Object.keys(body).length === 0)
    return { ok: false, message: i18n.global.t('field.panel.noChanges') }
  return { ok: true, value: body }
}

/** Translated mutation error using only codes and safe details. */
export function mutationMessage(cause: unknown): string {
  const { t } = i18n.global
  if (cause instanceof RequestError && cause.code === 'ERR_VALIDATION') {
    switch (cause.details.reason) {
      case 'system_field':
        return t('field.panel.reason.system_field')
      case 'constraint_unsupported':
        return t('field.panel.reason.constraint_unsupported')
      case 'storage_change':
        return t('field.panel.reason.storage_change')
      case 'display_ineligible':
        return t('field.panel.reason.display_ineligible')
    }
    const reason = cause.details.reason as Reason | undefined
    if (reason && CODEC_REASONS.has(reason)) {
      const checked = failed(reason, cause.details.option)
      if (!checked.ok) return checked.message
    }
  }
  return errorMessage(cause)
}

const CODEC_REASONS = new Set<string>([
  'unknown_kind',
  'invalid_options',
  'invalid_type',
  'invalid_text',
  'too_long',
  'invalid_integer',
  'invalid_decimal',
  'invalid_number',
  'imprecise_number',
  'out_of_range',
  'too_many_decimals',
  'invalid_boolean',
  'invalid_date',
  'invalid_datetime',
  'nonexistent_local_time',
  'timezone_required',
  'unknown_choice',
  'duplicate_choice',
  'invalid_email',
  'invalid_url',
  'url_scheme',
  'invalid_time',
  'invalid_duration',
  'invalid_rating',
  'invalid_phone',
  'invalid_uuid',
  'invalid_json',
  'invalid_ip',
  'invalid_cidr',
  'invalid_array',
  'read_only',
])

export const displayEligible = (field: TableField) =>
  !field.isSystem && field.uiType !== 'checkbox'

export function sizeLabel(size: string): string {
  const { t } = i18n.global
  switch (size) {
    case 'small':
      return t('field.panel.sizes.small')
    case 'big':
      return t('field.panel.sizes.big')
    case 'real':
      return t('field.panel.sizes.real')
    case 'double':
      return t('field.panel.sizes.double')
    default:
      return t('field.panel.sizes.int')
  }
}

export function dateFormatLabel(format: string): string {
  const { t } = i18n.global
  switch (format) {
    case 'slash':
      return t('field.panel.dateFormats.slash')
    case 'us':
      return t('field.panel.dateFormats.us')
    case 'eu':
      return t('field.panel.dateFormats.eu')
    default:
      return t('field.panel.dateFormats.iso')
  }
}

export const timeFormatLabel = (format: string) =>
  format === '12h'
    ? i18n.global.t('field.panel.timeFormats.h12')
    : i18n.global.t('field.panel.timeFormats.h24')

export const timezoneLabel = (zone: string) =>
  zone === 'fixed'
    ? i18n.global.t('field.panel.timezones.fixed')
    : i18n.global.t('field.panel.timezones.user')

export function colorLabel(color: string): string {
  const { t } = i18n.global
  switch (color) {
    case 'red':
      return t('field.panel.colors.red')
    case 'orange':
      return t('field.panel.colors.orange')
    case 'amber':
      return t('field.panel.colors.amber')
    case 'yellow':
      return t('field.panel.colors.yellow')
    case 'lime':
      return t('field.panel.colors.lime')
    case 'green':
      return t('field.panel.colors.green')
    case 'teal':
      return t('field.panel.colors.teal')
    case 'cyan':
      return t('field.panel.colors.cyan')
    case 'blue':
      return t('field.panel.colors.blue')
    case 'indigo':
      return t('field.panel.colors.indigo')
    case 'purple':
      return t('field.panel.colors.purple')
    case 'pink':
      return t('field.panel.colors.pink')
    default:
      return t('field.panel.colors.gray')
  }
}
