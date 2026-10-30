import type { Agg, Op } from './types'

// Field types declare operations; SQL compilation belongs to the query engine.
export const TEXT_OPS: readonly Op[] = [
  'is',
  'is_not',
  'contains',
  'not_contains',
  'starts_with',
  'ends_with',
  'is_empty',
  'is_not_empty',
]
export const NUMERIC_OPS: readonly Op[] = [
  'eq',
  'neq',
  'lt',
  'lte',
  'gt',
  'gte',
  'between',
  'is_empty',
  'is_not_empty',
]
export const TEMPORAL_OPS: readonly Op[] = [
  'is',
  'is_not',
  'before',
  'after',
  'on_or_before',
  'on_or_after',
  'between',
  'is_within',
  'is_empty',
  'is_not_empty',
]
export const TIME_OPS: readonly Op[] = [
  'is',
  'is_not',
  'before',
  'after',
  'on_or_before',
  'on_or_after',
  'between',
  'is_empty',
  'is_not_empty',
]
export const USER_OPS: readonly Op[] = [
  'is',
  'is_not',
  'has_any_of',
  'is_empty',
  'is_not_empty',
]
export const BASIC_AGGS: readonly Agg[] = [
  'none',
  'count',
  'count_empty',
  'count_filled',
  'unique',
]
export const NUMERIC_AGGS: readonly Agg[] = [
  ...BASIC_AGGS,
  'sum',
  'avg',
  'min',
  'max',
]
export const TEMPORAL_AGGS: readonly Agg[] = [
  ...BASIC_AGGS,
  'earliest',
  'latest',
]

export const withoutEmpty = (ops: readonly Op[]) =>
  ops.filter((op) => op !== 'is_empty' && op !== 'is_not_empty')
