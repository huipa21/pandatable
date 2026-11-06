import { expect, it } from 'vitest'
import {
  maxColumnWidth,
  minColumnWidth,
  parseColumnWidthInput,
} from './viewColumns'

it('accepts inclusive width bounds and rejects invalid Format input', () => {
  expect(parseColumnWidthInput(String(minColumnWidth))).toBe(minColumnWidth)
  expect(parseColumnWidthInput(String(maxColumnWidth))).toBe(maxColumnWidth)
  for (const value of ['', '59', '1001', '60.5', 'Infinity'])
    expect(parseColumnWidthInput(value)).toBeUndefined()
})
