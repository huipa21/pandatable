export interface GridRow {
  id: number
  name: string
  note: string
  quantity: number
  group: string
}

export type EditableField = 'name' | 'note' | 'quantity'
export interface CellPatch {
  index: number
  field: EditableField
  value: string | number
}

export interface WindowSource {
  total: number
  fetch(offset: number, limit: number, signal: AbortSignal): Promise<GridRow[]>
}
