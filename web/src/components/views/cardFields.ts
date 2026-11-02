import type { TableField } from '../../api/types'
import { bindField, type BoundField } from '../../fields/registry'
import type { Result } from '../../fields/types'

export interface CardField {
  field: TableField
  bound: Result<BoundField>
}

export function bindCardFields(
  fields: TableField[],
  ids: string[],
  displayFieldId: string | null,
): CardField[] {
  const byId = new Map(fields.map((field) => [field.id, field]))
  return ids
    .filter((id) => id !== displayFieldId)
    .flatMap((id) => {
      const field = byId.get(id)
      return field
        ? [{ field, bound: bindField(field.uiType, field.options) }]
        : []
    })
}
