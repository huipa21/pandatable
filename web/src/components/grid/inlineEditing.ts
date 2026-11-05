import type { BoundField } from '../../fields/registry'

export function isGridScalar(field: BoundField) {
  return (
    !field.readOnly &&
    !['checkbox', 'select', 'user', 'attachment'].includes(field.presentation)
  )
}

/** Writable single/multi-select fields edited through the grid choice list. */
export function isGridChoice(field: BoundField) {
  return !field.readOnly && field.presentation === 'select'
}

/** Writable checkbox fields toggled directly by one grid click (UX-08). */
export function isGridToggle(field: BoundField) {
  return !field.readOnly && field.presentation === 'checkbox'
}

/** What a plain single click on a writable data cell does. */
export function gridPointerAction(field: BoundField): 'edit' | 'toggle' | null {
  if (isGridToggle(field)) return 'toggle'
  if (isGridScalar(field) || isGridChoice(field)) return 'edit'
  return null
}
