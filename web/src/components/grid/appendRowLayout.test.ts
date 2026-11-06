import { expect, it } from 'vitest'
import { appendRowLayoutWidth } from './appendRowLayout'

it('append row covers the viewport when columns are narrower', () => {
  expect(appendRowLayoutWidth(960, [104, 180, 180])).toBe(960)
})

it('append row covers the rendered column span when columns are wider', () => {
  expect(appendRowLayoutWidth(960, [104, 320, 320, 320])).toBe(1064)
})

it('append row keeps the width when columns are reordered or frozen', () => {
  const widths = [104, 180, 240, 300]
  expect(appendRowLayoutWidth(700, widths)).toBe(
    appendRowLayoutWidth(700, [...widths].reverse()),
  )
})
