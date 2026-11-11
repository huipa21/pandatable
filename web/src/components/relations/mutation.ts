import type { LinkMutation } from '../../api/types'
import { linkUndoSteps } from '../../stores/undo'

export function relationLinkPath(
  tableId: string,
  rowId: string,
  fieldId: string,
) {
  return (
    `/tables/${encodeURIComponent(tableId)}/rows/` +
    `${encodeURIComponent(rowId)}/links/${encodeURIComponent(fieldId)}`
  )
}

export function relationLinkUndoSteps(result: LinkMutation, rowId: string) {
  if (
    !result?.row ||
    result.row.id !== rowId ||
    typeof result.row.fields !== 'object' ||
    result.row.fields === null ||
    Array.isArray(result.row.fields) ||
    !Array.isArray(result.changes)
  )
    throw new Error('Link mutation returned an unsupported response')
  return linkUndoSteps(result.changes)
}
