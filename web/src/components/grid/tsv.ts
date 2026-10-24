export class TsvError extends Error {}

export function parseTsv(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let value = ''
  let quoted = false
  let closed = false
  for (let i = 0; i < text.length; i++) {
    const c = text[i]!
    if (quoted) {
      if (c === '"') {
        if (text[i + 1] === '"') {
          value += '"'
          i++
        } else {
          quoted = false
          closed = true
        }
      } else {
        value += c
      }
      continue
    }
    if (c === '\t' || c === '\n' || c === '\r') {
      row.push(value)
      value = ''
      closed = false
      if (c !== '\t') {
        rows.push(row)
        row = []
        if (c === '\r' && text[i + 1] === '\n') i++
      }
    } else if (closed) {
      throw new TsvError('Unexpected character after quoted cell')
    } else if (c === '"' && value === '') {
      quoted = true
    } else {
      value += c
    }
  }
  if (quoted) throw new TsvError('Unterminated quoted cell')
  if (row.length || value || !rows.length || closed) {
    row.push(value)
    rows.push(row)
  }
  if (rows.reduce((n, cells) => n + cells.length, 0) > 5000) {
    throw new TsvError('Spike clipboard limit exceeded')
  }
  return rows
}

export function formatTsv(rows: readonly (readonly unknown[])[]): string {
  return rows
    .map((row) =>
      row
        .map((cell) => {
          const text = String(cell ?? '')
          return /["\t\r\n]/.test(text)
            ? `"${text.replaceAll('"', '""')}"`
            : text
        })
        .join('\t'),
    )
    .join('\r\n')
}
