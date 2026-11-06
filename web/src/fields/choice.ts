import {
  OptionSet,
  codePoints,
  isPlainObject,
  lowerSimple,
  trimSpace,
  validString,
} from './common'
import { BASIC_AGGS } from './capabilities'
import {
  fail,
  ok,
  type Choice,
  type FieldKind,
  type Result,
  type WireValue,
} from './types'

const MAX_CHOICES = 500
const MAX_LABEL = 100
const CHOICE_ID = /^[A-Za-z0-9_-]{1,64}$/

/** Allowed choice color tokens; the first is the default (D-118). */
export const CHOICE_COLORS = [
  'gray',
  'red',
  'orange',
  'amber',
  'yellow',
  'lime',
  'green',
  'teal',
  'cyan',
  'blue',
  'indigo',
  'purple',
  'pink',
] as const

export interface ChoiceOptions {
  choices: Choice[]
}

const validLabel = (label: string) => {
  const count = codePoints(label)
  return (
    count >= 1 &&
    count <= MAX_LABEL &&
    validString(label, false, false) &&
    !label.includes(',')
  )
}

function parseChoice(raw: unknown): Result<Choice> {
  if (!isPlainObject(raw)) return fail('invalid_options', 'choices')
  const set = OptionSet.parse(raw)
  if (!set.ok) return fail('invalid_options', 'choices')
  const read = (key: string, required: boolean): string | null => {
    const value = set.value.take(key)
    if (value === undefined || value === null) return required ? null : ''
    return typeof value === 'string' ? value : null
  }
  const id = read('id', true)
  const label = read('label', true)
  const color = read('color', false)
  if (id === null || label === null || color === null || !set.value.done().ok)
    return fail('invalid_options', 'choices')
  const choice = {
    id,
    label: trimSpace(label),
    color: color || CHOICE_COLORS[0],
  }
  if (
    !CHOICE_ID.test(choice.id) ||
    !validLabel(choice.label) ||
    !(CHOICE_COLORS as readonly string[]).includes(choice.color)
  )
    return fail('invalid_options', 'choices')
  return ok(choice)
}

/** Exact label, then unique lower-case match. */
function matchChoice(choices: readonly Choice[], text: string) {
  const exact = choices.findIndex((choice) => choice.label === text)
  if (exact >= 0) return exact
  const folded = lowerSimple(text)
  return choices.findIndex((choice) => lowerSimple(choice.label) === folded)
}

const ordered = (choices: readonly Choice[], selected: Set<number>) =>
  choices
    .filter((_, index) => selected.has(index))
    .map((choice) => choice.label)

/** Stored labels of a wire value (unknown labels included). */
export const choiceLabels = (value: WireValue): string[] =>
  typeof value === 'string'
    ? [value]
    : Array.isArray(value)
      ? value.filter((item): item is string => typeof item === 'string')
      : []

function choiceKind(multi: boolean): FieldKind<ChoiceOptions> {
  return {
    kind: multi ? 'multi_select' : 'single_select',
    readOnly: false,
    presentation: 'select',
    constraints: { required: true, unique: false, default: true },
    normalizeOptions(raw) {
      const set = OptionSet.parse(raw)
      if (!set.ok) return set
      const choices: Choice[] = []
      const rawChoices = set.value.take('choices')
      if (rawChoices !== undefined && rawChoices !== null) {
        if (!Array.isArray(rawChoices) || rawChoices.length > MAX_CHOICES)
          return fail('invalid_options', 'choices')
        const ids = new Set<string>()
        const folds = new Set<string>()
        for (const item of rawChoices) {
          const choice = parseChoice(item)
          if (!choice.ok) return choice
          const fold = lowerSimple(choice.value.label)
          if (ids.has(choice.value.id) || folds.has(fold))
            return fail('invalid_options', 'choices')
          ids.add(choice.value.id)
          folds.add(fold)
          choices.push(choice.value)
        }
      }
      const done = set.value.done()
      return done.ok ? ok({ choices }) : done
    },
    normalize(value, options) {
      if (value === null) return ok(null)
      const labels = options.choices.map((choice) => choice.label)
      if (!multi) {
        if (typeof value !== 'string') return fail('invalid_type')
        if (value === '') return ok(null)
        return labels.includes(value) ? ok(value) : fail('unknown_choice')
      }
      if (!Array.isArray(value)) return fail('invalid_type')
      if (value.length > MAX_CHOICES) return fail('unknown_choice')
      const selected = new Set<number>()
      for (const item of value) {
        if (typeof item !== 'string') return fail('invalid_type')
        const index = labels.indexOf(item)
        if (index < 0) return fail('unknown_choice')
        if (selected.has(index)) return fail('duplicate_choice')
        selected.add(index)
      }
      return selected.size === 0
        ? ok(null)
        : ok(ordered(options.choices, selected))
    },
    parsePaste(input, options) {
      const text = trimSpace(input)
      if (text === '') return ok(null)
      if (!multi) {
        const index = matchChoice(options.choices, text)
        return index < 0
          ? fail('unknown_choice')
          : ok(options.choices[index]?.label ?? null)
      }
      const selected = new Set<number>()
      for (const raw of text.split(',')) {
        const part = trimSpace(raw)
        if (part === '') continue
        const index = matchChoice(options.choices, part)
        if (index < 0) return fail('unknown_choice')
        selected.add(index)
      }
      return selected.size === 0
        ? ok(null)
        : ok(ordered(options.choices, selected))
    },
    formatCopy: (value) => choiceLabels(value).join(', '),
    formatDisplay: (value) => choiceLabels(value).join(', '),
    filterOps: () =>
      multi
        ? [
            'has_any_of',
            'has_all_of',
            'has_none_of',
            'is_exactly',
            'is_empty',
            'is_not_empty',
          ]
        : [
            'is',
            'is_not',
            'is_any_of',
            'is_none_of',
            'is_empty',
            'is_not_empty',
          ],
    aggregates: () =>
      multi
        ? [
            'none',
            'count',
            'count_empty',
            'count_filled',
            'percent_empty',
            'percent_filled',
          ]
        : [...BASIC_AGGS],
  }
}

export const singleSelectKind = choiceKind(false)
export const multiSelectKind = choiceKind(true)
