import { expect, it } from 'vitest'
import { columnFooterCells, columnFooterScrollLeft } from './columnFooters'

it('keeps summary cells in visible column order and uses the rendered widths', () => {
  expect(
    columnFooterCells([
      { prop: '__selected', size: 104, pin: 'colPinStart' },
      { prop: 'name', size: 220, pin: 'colPinStart' },
      { prop: 'amount', size: 280 },
    ]),
  ).toEqual([
    { key: '__selected', fieldId: null, width: 104, stickyLeft: 0 },
    { key: 'name', fieldId: 'name', width: 220, stickyLeft: 104 },
    { key: 'amount', fieldId: 'amount', width: 280 },
  ])
})

it('follows reordered and frozen columns without inventing widths', () => {
  expect(
    columnFooterCells([
      { prop: '__selected', size: 104, pin: 'colPinStart' },
      { prop: 'amount', size: 240, pin: 'colPinStart' },
      { prop: 'name', size: 180 },
    ]),
  ).toEqual([
    { key: '__selected', fieldId: null, width: 104, stickyLeft: 0 },
    { key: 'amount', fieldId: 'amount', width: 240, stickyLeft: 104 },
    { key: 'name', fieldId: 'name', width: 180 },
  ])
})

it('uses the grid default for columns without an explicit size', () => {
  expect(columnFooterCells([{ prop: 'name' }])).toEqual([
    { key: 'name', fieldId: 'name', width: 180 },
  ])
})

it('clamps footer scroll coordinates to the rendered column span', () => {
  const cells = columnFooterCells([
    { prop: '__selected', size: 104, pin: 'colPinStart' },
    { prop: 'name', size: 180, pin: 'colPinStart' },
    { prop: 'amount', size: 400 },
    { prop: 'total', size: 320 },
  ])
  expect(columnFooterScrollLeft(900, 600, cells)).toBe(404)
  expect(columnFooterScrollLeft(150, 600, cells)).toBe(150)
  expect(columnFooterScrollLeft(-20, 600, cells)).toBe(0)
  expect(columnFooterScrollLeft(100, 1200, cells)).toBe(0)
  expect(columnFooterScrollLeft(150, 0, cells)).toBe(150)
})
