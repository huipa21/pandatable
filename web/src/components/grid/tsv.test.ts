import { describe, expect, it } from 'vitest'
import { formatTsv, parseTsv, TsvError } from './tsv'

describe('spreadsheet TSV boundary', () => {
  it('round-trips tabs, quotes, multiline, empty and Unicode cells', () => {
    const cells = [
      ['plain', 'tab\there', 'line 1\r\nline 2', 'say "hello"', ''],
      ['中文', '', '\n', '"', 'last'],
    ]
    expect(parseTsv(formatTsv(cells))).toEqual(cells)
  })

  it('accepts Excel/Sheets CRLF and terminal newline without an extra row', () => {
    expect(parseTsv('"first\nsecond"\t"said ""yes"""\r\nx\t\r\n')).toEqual([
      ['first\nsecond', 'said "yes"'],
      ['x', ''],
    ])
    expect(parseTsv('')).toEqual([['']])
    expect(parseTsv('\t')).toEqual([['', '']])
    expect(parseTsv('a\rb\nc\r\n')).toEqual([['a'], ['b'], ['c']])
    expect(parseTsv('""')).toEqual([['']])
  })

  it('rejects malformed quotes and oversized matrices explicitly', () => {
    expect(() => parseTsv('"open')).toThrow(TsvError)
    expect(() => parseTsv('"closed"x')).toThrow(TsvError)
    expect(() => parseTsv(Array(5001).fill('x').join('\t'))).toThrow(TsvError)
    expect(parseTsv(Array(5000).fill('x').join('\t'))[0]).toHaveLength(5000)
  })
})
