export interface GridFooterColumn {
  prop: string | number
  size?: number
  pin?: string
}

export interface ColumnFooterCell {
  key: string
  fieldId: string | null
  width: number
  stickyLeft?: number
}

export function columnFooterCells(
  columns: readonly GridFooterColumn[],
): ColumnFooterCell[] {
  let pinnedWidth = 0
  return columns.map((column) => {
    const width =
      typeof column.size === 'number' && Number.isFinite(column.size)
        ? column.size
        : 180
    const pinned = column.pin === 'colPinStart'
    const cell: ColumnFooterCell = {
      key: String(column.prop),
      fieldId:
        String(column.prop) === '__selected' ? null : String(column.prop),
      width,
      ...(pinned ? { stickyLeft: pinnedWidth } : {}),
    }
    if (pinned) pinnedWidth += width
    return cell
  })
}
