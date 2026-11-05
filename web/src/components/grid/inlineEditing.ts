import type { BoundField } from '../../fields/registry'

export function isGridScalar(field: BoundField) {
  return (
    !field.readOnly &&
    !['checkbox', 'select', 'user', 'attachment'].includes(field.presentation)
  )
}
