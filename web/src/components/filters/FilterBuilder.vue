<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FilterNode, RowValue, TableSchema } from '../../api/types'
import type { Op } from '../../fields/types'

defineOptions({ name: 'FilterBuilder' })

const props = withDefaults(
  defineProps<{
    schema: TableSchema
    modelValue: FilterNode | null
    depth?: number
  }>(),
  { depth: 1 },
)
const emit = defineEmits<{ 'update:modelValue': [value: FilterNode | null] }>()
const { t } = useI18n()

type Group = { op: 'and' | 'or'; items: FilterNode[] }
type Condition = { field: string; cmp: Op; value?: RowValue }

const orderedFields = computed(() =>
  [...props.schema.fields]
    .filter(
      (field) =>
        field.filterable !== false && (field.filterOps?.length ?? 0) > 0,
    )
    .sort((left, right) => left.position - right.position),
)
const group = computed<Group>(() =>
  isGroup(props.modelValue) ? props.modelValue : { op: 'and', items: [] },
)
const operatorLabels = computed<Record<Op, string>>(() => ({
  is: t('filter.operators.is'),
  is_not: t('filter.operators.is_not'),
  contains: t('filter.operators.contains'),
  not_contains: t('filter.operators.not_contains'),
  starts_with: t('filter.operators.starts_with'),
  ends_with: t('filter.operators.ends_with'),
  is_empty: t('filter.operators.is_empty'),
  is_not_empty: t('filter.operators.is_not_empty'),
  eq: t('filter.operators.eq'),
  neq: t('filter.operators.neq'),
  lt: t('filter.operators.lt'),
  lte: t('filter.operators.lte'),
  gt: t('filter.operators.gt'),
  gte: t('filter.operators.gte'),
  between: t('filter.operators.between'),
  before: t('filter.operators.before'),
  after: t('filter.operators.after'),
  on_or_before: t('filter.operators.on_or_before'),
  on_or_after: t('filter.operators.on_or_after'),
  is_within: t('filter.operators.is_within'),
  is_checked: t('filter.operators.is_checked'),
  is_not_checked: t('filter.operators.is_not_checked'),
  is_any_of: t('filter.operators.is_any_of'),
  is_none_of: t('filter.operators.is_none_of'),
  has_any_of: t('filter.operators.has_any_of'),
  has_all_of: t('filter.operators.has_all_of'),
  has_none_of: t('filter.operators.has_none_of'),
  is_exactly: t('filter.operators.is_exactly'),
  has_key: t('filter.operators.has_key'),
  path_equals: t('filter.operators.path_equals'),
  contained_in: t('filter.operators.contained_in'),
}))
const linkCountLabels = computed<Partial<Record<Op, string>>>(() => ({
  eq: t('filter.operators.countEq'),
  neq: t('filter.operators.countNeq'),
  lt: t('filter.operators.countLt'),
  lte: t('filter.operators.countLte'),
  gt: t('filter.operators.countGt'),
  gte: t('filter.operators.countGte'),
  between: t('filter.operators.countBetween'),
}))

function isGroup(value: FilterNode | null): value is Group {
  return value !== null && 'items' in value
}

function pathEqualsValue(item: FilterNode) {
  const value = selectedValue(item)
  if (typeof value !== 'object' || value === null || Array.isArray(value))
    return { path: [] as string[], expected: '' }
  return {
    path: Array.isArray(value.path)
      ? value.path.filter(
          (segment): segment is string => typeof segment === 'string',
        )
      : [],
    expected: typeof value.value === 'string' ? value.value : '',
  }
}

function updatePathEquals(
  index: number,
  part: 'path' | 'expected',
  value: string,
) {
  const current = pathEqualsValue(
    group.value.items[index] ?? { op: 'and', items: [] },
  )
  const path = part === 'path' ? value.split('.').filter(Boolean) : current.path
  const expected = part === 'expected' ? value : current.expected
  updateValue(index, { path, value: expected })
}

function betweenValue(item: FilterNode, bound: number) {
  const value = selectedValue(item)
  return Array.isArray(value) ? (value[bound] ?? '') : ''
}

function updateBetween(index: number, bound: number, value: string) {
  const current = selectedValue(
    group.value.items[index] ?? { op: 'and', items: [] },
  )
  const values: RowValue[] = Array.isArray(current) ? [...current] : ['', '']
  values[bound] = value
  updateValue(index, values)
}

function fieldOptions(fieldId: string) {
  return (
    props.schema.fields.find((item) => item.id === fieldId)?.filterOps ?? []
  )
}

function isCondition(value: FilterNode): value is Condition {
  return !('items' in value)
}

function selectedField(item: FilterNode) {
  return isCondition(item) ? item.field : ''
}

function selectedOperator(item: FilterNode) {
  return isCondition(item) ? item.cmp : ''
}

function selectedValue(item: FilterNode): RowValue | undefined {
  return isCondition(item) ? item.value : undefined
}

function defaultValue(fieldId: string, cmp: Op): RowValue | undefined {
  if (
    cmp === 'is_empty' ||
    cmp === 'is_not_empty' ||
    cmp === 'is_checked' ||
    cmp === 'is_not_checked'
  )
    return undefined
  const field = orderedFields.value.find((item) => item.id === fieldId)
  if (cmp === 'is_within') return { rel: 'past', days: 7 }
  if (cmp === 'path_equals') return { path: [], value: '' }
  if (
    field &&
    ['date', 'datetime', 'created_at', 'updated_at'].includes(field.uiType)
  )
    return { rel: 'today' }
  if (field && ['user', 'created_by', 'updated_by'].includes(field.uiType))
    return { rel: 'me' }
  const choices = field?.options.choices
  if (Array.isArray(choices) && choices.length) {
    const first = choices[0]
    const label =
      typeof first === 'object' && first !== null && 'label' in first
        ? String(first.label)
        : ''
    return cmp === 'is_any_of' || cmp === 'has_any_of' ? [label] : label
  }
  if (cmp === 'between') return ['', '']
  if (cmp === 'is_any_of' || cmp === 'has_any_of') return []
  return ''
}

function newCondition(fieldId = orderedFields.value[0]?.id ?? ''): Condition {
  const cmp = fieldOptions(fieldId)[0] ?? 'is'
  return { field: fieldId, cmp, value: defaultValue(fieldId, cmp) }
}

function publish(items: FilterNode[], op = group.value.op) {
  emit('update:modelValue', items.length ? { op, items } : null)
}

function updateGroup(op: 'and' | 'or') {
  publish(group.value.items, op)
}

function addCondition() {
  publish([...group.value.items, newCondition()])
}

function addGroup() {
  if (props.depth >= 3) return
  publish([...group.value.items, { op: 'and', items: [newCondition()] }])
}

function updateItem(index: number, value: FilterNode | null) {
  const items = [...group.value.items]
  if (value) items[index] = value
  else items.splice(index, 1)
  publish(items)
}

function updateField(index: number, fieldId: string) {
  updateItem(index, newCondition(fieldId))
}

function updateOperator(index: number, cmp: Op) {
  const item = group.value.items[index]
  if (!item || !isCondition(item)) return
  updateItem(index, { ...item, cmp, value: defaultValue(item.field, cmp) })
}

function updateValue(index: number, value: RowValue | undefined) {
  const item = group.value.items[index]
  if (!item || !isCondition(item)) return
  updateItem(index, { ...item, value })
}

function fieldFor(item: FilterNode) {
  return orderedFields.value.find((field) => field.id === selectedField(item))
}

function operatorLabel(fieldId: string, operator: Op) {
  const field = orderedFields.value.find((item) => item.id === fieldId)
  return field?.uiType === 'link'
    ? (linkCountLabels.value[operator] ?? operatorLabels.value[operator])
    : operatorLabels.value[operator]
}

function choices(fieldId: string) {
  const raw = orderedFields.value.find((field) => field.id === fieldId)?.options
    .choices
  if (!Array.isArray(raw)) return []
  return raw.flatMap((choice) =>
    typeof choice === 'object' && choice !== null && 'label' in choice
      ? [{ label: String(choice.label), value: String(choice.label) }]
      : [],
  )
}

function relativeChoice(value: unknown) {
  if (typeof value !== 'object' || value === null || !('rel' in value))
    return 'today'
  if (
    'days' in value &&
    'rel' in value &&
    (value.rel === 'past' || value.rel === 'next')
  )
    return `${String(value.rel)}:${String(value.days)}`
  return String(value.rel)
}

function updateRelative(index: number, rel: string) {
  if (rel.startsWith('past:') || rel.startsWith('next:')) {
    const direction = rel.startsWith('past:') ? 'past' : 'next'
    updateValue(index, { rel: direction, days: Number(rel.split(':')[1]) })
  } else {
    updateValue(index, { rel })
  }
}

function valueInput(
  field: TableSchema['fields'][number] | undefined,
  operator: Op | '',
) {
  if (!field) return 'text'
  if (
    field.uiType === 'link' &&
    ['eq', 'neq', 'lt', 'lte', 'gt', 'gte', 'between'].includes(operator)
  )
    return 'number'
  if (field.uiType === 'date') return 'date'
  if (
    [
      'integer',
      'decimal',
      'float',
      'currency',
      'percent',
      'rating',
      'autonumber',
      'id',
    ].includes(field.uiType)
  )
    return 'text'
  return 'text'
}

function updateInputValue(index: number, item: FilterNode, value: string) {
  const cmp = selectedOperator(item)
  if (
    [
      'is_any_of',
      'is_none_of',
      'has_any_of',
      'has_all_of',
      'has_none_of',
      'is_exactly',
    ].includes(cmp)
  )
    updateValue(
      index,
      value
        .split(',')
        .map((part) => part.trim())
        .filter(Boolean),
    )
  else updateValue(index, value)
}

function displayValue(value: RowValue | undefined) {
  return typeof value === 'string' || typeof value === 'number'
    ? String(value)
    : ''
}
</script>

<template>
  <fieldset class="filter-group" :data-depth="depth">
    <legend>{{ t('filter.group') }} {{ depth }}</legend>
    <label>
      <span>{{ t('filter.match') }}</span>
      <select
        :value="group.op"
        @change="
          updateGroup(
            ($event.target as HTMLSelectElement).value as 'and' | 'or',
          )
        "
      >
        <option value="and">{{ t('filter.all') }}</option>
        <option value="or">{{ t('filter.any') }}</option>
      </select>
    </label>
    <div v-for="(item, index) in group.items" :key="index" class="filter-item">
      <FilterBuilder
        v-if="!isCondition(item)"
        :schema="schema"
        :model-value="item"
        :depth="depth + 1"
        @update:model-value="updateItem(index, $event)"
      />
      <template v-else>
        <label>
          <span>{{ t('filter.field') }}</span>
          <select
            :value="selectedField(item)"
            @change="
              updateField(index, ($event.target as HTMLSelectElement).value)
            "
          >
            <option
              v-for="field in orderedFields"
              :key="field.id"
              :value="field.id"
            >
              {{ field.name }}
            </option>
          </select>
        </label>
        <label>
          <span>{{ t('filter.operator') }}</span>
          <select
            :value="selectedOperator(item)"
            @change="
              updateOperator(
                index,
                ($event.target as HTMLSelectElement).value as Op,
              )
            "
          >
            <option
              v-for="operator in fieldOptions(selectedField(item))"
              :key="operator"
              :value="operator"
            >
              {{ operatorLabel(selectedField(item), operator) }}
            </option>
          </select>
        </label>
        <template
          v-if="
            ![
              'is_empty',
              'is_not_empty',
              'is_checked',
              'is_not_checked',
            ].includes(selectedOperator(item))
          "
        >
          <label
            v-if="
              ['date', 'datetime', 'created_at', 'updated_at'].includes(
                fieldFor(item)?.uiType ?? '',
              )
            "
          >
            <span>{{ t('filter.relativeDate') }}</span>
            <select
              :value="relativeChoice(selectedValue(item))"
              @change="
                updateRelative(
                  index,
                  ($event.target as HTMLSelectElement).value,
                )
              "
            >
              <template v-if="selectedOperator(item) === 'is_within'">
                <option value="past:7">{{ t('filter.past7Days') }}</option>
                <option value="next:7">{{ t('filter.next7Days') }}</option>
                <option value="this_week">{{ t('filter.thisWeek') }}</option>
                <option value="this_month">{{ t('filter.thisMonth') }}</option>
                <option value="this_year">{{ t('filter.thisYear') }}</option>
              </template>
              <template v-else>
                <option value="today">{{ t('filter.today') }}</option>
                <option value="yesterday">{{ t('filter.yesterday') }}</option>
                <option value="tomorrow">{{ t('filter.tomorrow') }}</option>
              </template>
            </select>
          </label>
          <label
            v-else-if="
              ['user', 'created_by', 'updated_by'].includes(
                fieldFor(item)?.uiType ?? '',
              )
            "
          >
            <span>{{ t('filter.relativeUser') }}</span>
            <select
              :value="relativeChoice(selectedValue(item))"
              @change="
                updateRelative(
                  index,
                  ($event.target as HTMLSelectElement).value,
                )
              "
            >
              <option value="me">{{ t('filter.me') }}</option>
            </select>
          </label>
          <label v-else-if="choices(selectedField(item)).length">
            <span>{{ t('filter.value') }}</span>
            <select
              v-if="
                [
                  'is_any_of',
                  'is_none_of',
                  'has_any_of',
                  'has_all_of',
                  'has_none_of',
                  'is_exactly',
                ].includes(selectedOperator(item))
              "
              multiple
              :value="
                Array.isArray(selectedValue(item))
                  ? (selectedValue(item) as string[])
                  : []
              "
              @change="
                updateValue(
                  index,
                  Array.from(
                    ($event.target as HTMLSelectElement).selectedOptions,
                    (option) => option.value,
                  ),
                )
              "
            >
              <option
                v-for="choice in choices(selectedField(item))"
                :key="choice.value"
                :value="choice.value"
              >
                {{ choice.label }}
              </option>
            </select>
            <select
              v-else
              :value="displayValue(selectedValue(item))"
              @change="
                updateValue(index, ($event.target as HTMLSelectElement).value)
              "
            >
              <option
                v-for="choice in choices(selectedField(item))"
                :key="choice.value"
                :value="choice.value"
              >
                {{ choice.label }}
              </option>
            </select>
          </label>
          <template v-else-if="selectedOperator(item) === 'between'">
            <label v-for="bound in [0, 1]" :key="bound">
              <span>{{
                bound === 0 ? t('filter.valueFrom') : t('filter.valueTo')
              }}</span>
              <input
                :type="valueInput(fieldFor(item), selectedOperator(item))"
                :value="betweenValue(item, bound)"
                @input="
                  updateBetween(
                    index,
                    bound,
                    ($event.target as HTMLInputElement).value,
                  )
                "
              />
            </label>
          </template>
          <template v-else-if="selectedOperator(item) === 'path_equals'">
            <label>
              <span>{{ t('filter.jsonPath') }}</span>
              <input
                type="text"
                :value="pathEqualsValue(item).path.join('.')"
                @input="
                  updatePathEquals(
                    index,
                    'path',
                    ($event.target as HTMLInputElement).value,
                  )
                "
              />
            </label>
            <label>
              <span>{{ t('filter.jsonExpected') }}</span>
              <input
                type="text"
                :value="pathEqualsValue(item).expected"
                @input="
                  updatePathEquals(
                    index,
                    'expected',
                    ($event.target as HTMLInputElement).value,
                  )
                "
              />
            </label>
          </template>
          <label v-else>
            <span>{{ t('filter.value') }}</span>
            <input
              :type="valueInput(fieldFor(item), selectedOperator(item))"
              :value="displayValue(selectedValue(item))"
              @input="
                updateInputValue(
                  index,
                  item,
                  ($event.target as HTMLInputElement).value,
                )
              "
            />
          </label>
        </template>
        <button type="button" @click="updateItem(index, null)">
          {{ t('filter.removeCondition') }}
        </button>
      </template>
    </div>
    <div class="filter-actions">
      <button type="button" @click="addCondition">
        {{ t('filter.addCondition') }}
      </button>
      <button v-if="depth < 3" type="button" @click="addGroup">
        {{ t('filter.addGroup') }}
      </button>
      <button
        v-if="depth > 1"
        type="button"
        @click="emit('update:modelValue', null)"
      >
        {{ t('filter.removeGroup') }}
      </button>
      <button
        v-if="depth === 1 && group.items.length"
        type="button"
        @click="publish([])"
      >
        {{ t('filter.clear') }}
      </button>
    </div>
  </fieldset>
</template>

<style scoped>
.filter-group {
  display: grid;
  gap: 0.6rem;
  min-width: 0;
  margin: 0.5rem 0;
  padding: 0.75rem;
  border: 1px solid var(--n-border-color, #888);
  border-radius: 0.4rem;
}

.filter-item,
.filter-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: end;
  gap: 0.5rem;
  min-width: 0;
}

.filter-item > label,
.filter-group > label {
  display: grid;
  gap: 0.25rem;
}

.filter-item input,
.filter-item select,
.filter-group select,
.filter-actions button,
.filter-item button {
  min-height: 2.5rem;
  max-width: 100%;
}

.filter-item > .filter-group {
  flex: 1 1 100%;
}
</style>
