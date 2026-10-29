import { checkboxKind, decimalKind, floatKind, integerKind } from './number'
import { multiSelectKind, singleSelectKind } from './choice'
import { systemKinds } from './system'
import { dateKind, datetimeKind } from './temporal'
import { textKinds } from './text'
import { batch2Kinds } from './batch2'
import {
  fail,
  ok,
  type Agg,
  type Constraints,
  type FieldEnv,
  type FieldKind,
  type Kind,
  type Op,
  type Presentation,
  type Result,
  type WireValue,
} from './types'

// Batch-1 FieldKind registry (D-121). Only the lazy Field panel chunk imports it
// (D-130); it joins the lazy grid chunk in T1.16.
const registry: { [K in Kind]: FieldKind<never> } = {
  ...textKinds,
  integer: integerKind,
  decimal: decimalKind,
  float: floatKind,
  checkbox: checkboxKind,
  date: dateKind,
  datetime: datetimeKind,
  single_select: singleSelectKind,
  multi_select: multiSelectKind,
  ...batch2Kinds,
  ...systemKinds,
} as { [K in Kind]: FieldKind<never> }

export const KINDS = Object.keys(registry) as Kind[]

export const isKind = (value: unknown): value is Kind =>
  typeof value === 'string' && Object.hasOwn(registry, value)

export const isSystemKind = (kind: Kind) =>
  kind === 'id' ||
  kind === 'created_at' ||
  kind === 'created_by' ||
  kind === 'updated_at' ||
  kind === 'updated_by'

/** Option-independent constraint support of a kind (doc 07). */
export const kindConstraints = (kind: Kind): Constraints => ({
  ...registry[kind].constraints,
})

/** A kind bound to normalized options, like Go codec.Field. */
export interface BoundField {
  kind: Kind
  readOnly: boolean
  presentation: Presentation
  constraints: Constraints
  options: unknown
  normalize(value: unknown): Result<WireValue>
  parsePaste(text: string, env: FieldEnv): Result<WireValue>
  formatCopy(value: WireValue, env: FieldEnv): string
  formatDisplay(value: WireValue, env: FieldEnv): string
  filterOps(): Op[]
  aggregates(): Agg[]
}

export function bindField(
  kind: string,
  rawOptions: unknown,
): Result<BoundField> {
  if (!isKind(kind)) return fail('unknown_kind')
  const definition = registry[kind] as FieldKind<unknown>
  const normalized = definition.normalizeOptions(rawOptions)
  if (!normalized.ok) return normalized
  const options = normalized.value
  return ok({
    kind,
    readOnly: definition.readOnly,
    presentation: definition.presentation,
    constraints: { ...definition.constraints },
    options,
    normalize: (value) => definition.normalize(value, options),
    parsePaste: (text, env) => definition.parsePaste(text, options, env),
    formatCopy: (value, env) => definition.formatCopy(value, options, env),
    formatDisplay: (value, env) =>
      definition.formatDisplay(value, options, env),
    filterOps: () => definition.filterOps(options),
    aggregates: () => definition.aggregates(options),
  })
}
