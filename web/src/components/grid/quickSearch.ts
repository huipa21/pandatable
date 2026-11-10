import type { TableField } from '../../api/types'

/** Mirrors `searchableQuickSearchKind` in internal/structure/search.go. */
const searchableKinds = new Set([
  'text',
  'long_text',
  'single_select',
  'multi_select',
  'email',
  'url',
  'phone',
  'integer',
  'decimal',
  'float',
  'currency',
  'percent',
  'rating',
  'autonumber',
  'duration',
  'date',
  'time',
  'datetime',
  'created_at',
  'updated_at',
])

export function isQuickSearchable(field: Pick<TableField, 'uiType'>) {
  return searchableKinds.has(field.uiType)
}

export interface QuickSearch {
  term: string
  fieldId: string | null
}

export interface QuickSearchField {
  id: string
  name: string
  uiType: string
}

/** Visible searchable fields in column order, as the server validates them. */
export function quickSearchFields(
  fields: readonly TableField[],
  visibleIds: readonly string[],
): QuickSearchField[] {
  const byId = new Map(fields.map((field) => [field.id, field]))
  return visibleIds.flatMap((id) => {
    const field = byId.get(id)
    return field && isQuickSearchable(field)
      ? [{ id: field.id, name: field.name, uiType: field.uiType }]
      : []
  })
}

/** Request members for a transient quick search; empty terms send nothing. */
export function quickSearchParams(search: QuickSearch): {
  search?: string
  searchField?: string
} {
  if (!search.term) return {}
  return search.fieldId
    ? { search: search.term, searchField: search.fieldId }
    : { search: search.term }
}
