import { fail, isWireValue, ok, type FieldKind, type WireValue } from './types'

const constraints = { required: false, unique: false, default: false }

function computedField(
  kind: 'lookup' | 'rollup',
): FieldKind<Record<string, unknown>> {
  return {
    kind,
    readOnly: true,
    presentation: 'text',
    constraints,
    normalizeOptions(raw) {
      if (typeof raw !== 'object' || raw === null || Array.isArray(raw))
        return fail('invalid_options')
      return ok(raw as Record<string, unknown>)
    },
    normalize(value) {
      if (kind === 'lookup' && !Array.isArray(value) && value !== '🔒')
        return fail('invalid_type')
      if (value !== null && typeof value === 'object' && !Array.isArray(value))
        return fail('invalid_type')
      return ok(value as WireValue)
    },
    parsePaste: () => fail('read_only'),
    formatCopy(value) {
      return format(value)
    },
    formatDisplay(value) {
      return format(value)
    },
    filterOps: () => [],
    aggregates: () => [],
  }
}

const formulaField: FieldKind<Record<string, unknown>> = {
  kind: 'formula',
  readOnly: true,
  presentation: 'text',
  constraints,
  normalizeOptions(raw) {
    if (typeof raw !== 'object' || raw === null || Array.isArray(raw))
      return fail('invalid_options')
    return ok(raw as Record<string, unknown>)
  },
  normalize(value) {
    return isWireValue(value) ? ok(value) : fail('invalid_type')
  },
  parsePaste: () => fail('read_only'),
  formatCopy(value) {
    return format(value)
  },
  formatDisplay(value) {
    return format(value)
  },
  filterOps: () => [],
  aggregates: () => [],
}

function format(value: WireValue): string {
  if (value === null) return ''
  if (Array.isArray(value)) return value.map(format).filter(Boolean).join(', ')
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

export const computedKinds: Record<
  'lookup' | 'rollup' | 'formula',
  FieldKind<unknown>
> = {
  lookup: computedField('lookup') as FieldKind<unknown>,
  rollup: computedField('rollup') as FieldKind<unknown>,
  formula: formulaField as FieldKind<unknown>,
}
