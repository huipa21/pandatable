import { OptionSet } from './common'
import {
  BASIC_AGGS,
  NUMERIC_OPS,
  TEMPORAL_OPS,
  USER_OPS,
  withoutEmpty,
} from './capabilities'
import { formatInstantCopy, formatInstantDisplay } from './temporal'
import { fail, ok, type FieldKind, type Kind } from './types'

type SystemKind = Extract<
  Kind,
  'id' | 'created_at' | 'created_by' | 'updated_at' | 'updated_by'
>

/** Read-only system kinds (D-120): no editor, paste, default or constraints. */
function systemKind(kind: SystemKind): FieldKind<Record<string, never>> {
  const temporal = kind === 'created_at' || kind === 'updated_at'
  const user = kind === 'created_by' || kind === 'updated_by'
  return {
    kind,
    readOnly: true,
    presentation: temporal ? 'datetime' : user ? 'user' : 'number',
    constraints: { required: false, unique: false, default: false },
    normalizeOptions(raw) {
      const set = OptionSet.parse(raw)
      if (!set.ok) return set
      const done = set.value.done()
      return done.ok ? ok({}) : done
    },
    normalize: () => fail('read_only'),
    parsePaste: () => fail('read_only'),
    formatCopy(value, _options, env) {
      if (temporal) return formatInstantCopy(value, {}, env)
      return typeof value === 'string' ? value : ''
    },
    formatDisplay(value, _options, env) {
      if (temporal)
        return formatInstantDisplay(
          value,
          { dateFormat: 'iso', timeFormat: '24h' },
          env,
        )
      return typeof value === 'string' ? value : ''
    },
    filterOps: () =>
      kind === 'id'
        ? withoutEmpty(NUMERIC_OPS)
        : temporal
          ? withoutEmpty(TEMPORAL_OPS)
          : [...USER_OPS],
    aggregates: () =>
      kind === 'id'
        ? ['none', 'count', 'min', 'max']
        : temporal
          ? ['none', 'count', 'earliest', 'latest']
          : [...BASIC_AGGS],
  }
}

export const systemKinds = {
  id: systemKind('id'),
  created_at: systemKind('created_at'),
  created_by: systemKind('created_by'),
  updated_at: systemKind('updated_at'),
  updated_by: systemKind('updated_by'),
}
