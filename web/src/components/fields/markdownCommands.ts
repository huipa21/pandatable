import { codePoints } from '../../fields/common'
import { MAX_LONG_TEXT_LENGTH } from '../../fields/text'

export type MdCommand =
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'code'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'quote'
  | 'bullet'
  | 'ordered'
  | 'check'
  | 'link'
  | 'image'

export interface TextSel {
  value: string
  start: number
  end: number
}

export interface MdPlaceholder {
  text: string
  url: string
}

export const MD_SHORTCUTS: Record<string, MdCommand> = {
  'mod+b': 'bold',
  'mod+i': 'italic',
  'mod+u': 'underline',
}

type InlineCommand = 'bold' | 'italic' | 'underline' | 'strike' | 'code'
type BlockCommand = 'h1' | 'h2' | 'h3' | 'bullet' | 'ordered' | 'check'

const INLINE: Record<InlineCommand, readonly [string, string]> = {
  bold: ['**', '**'],
  italic: ['_', '_'],
  underline: ['<u>', '</u>'],
  strike: ['~~', '~~'],
  code: ['`', '`'],
}

const FENCE = '```'
const WORD_CHAR = /^[\p{L}\p{N}\p{M}]$/u
const QUOTE_PREFIX = /^(?:> ?)+/
const LEADING_WS = /^[ \t]*/
const BLOCK_PREFIX = /^(?:#{1,6} |- \[[ xX]\] |[-*+] |\d{1,9}[.)] )/

function isBlock(cmd: MdCommand): cmd is BlockCommand {
  return (
    cmd === 'h1' ||
    cmd === 'h2' ||
    cmd === 'h3' ||
    cmd === 'bullet' ||
    cmd === 'ordered' ||
    cmd === 'check'
  )
}

/** Converts CR/CRLF to LF and clamps/orders the selection to the result. */
function normalize(sel: TextSel): TextSel {
  const raw = sel.value
  const clamp = (n: number) =>
    Math.min(Math.max(Number.isFinite(n) ? Math.trunc(n) : 0, 0), raw.length)
  let start = clamp(sel.start)
  let end = clamp(sel.end)
  if (start > end) [start, end] = [end, start]
  if (!raw.includes('\r')) return { value: raw, start, end }
  const shift = (pos: number) => {
    let removed = 0
    for (let i = 0; i < pos; i++)
      if (raw[i] === '\r' && raw[i + 1] === '\n') removed++
    return pos - removed
  }
  return {
    value: raw.replace(/\r\n?/g, '\n'),
    start: shift(start),
    end: shift(end),
  }
}

function charBefore(value: string, pos: number): string {
  if (pos <= 0) return ''
  const low = value.charCodeAt(pos - 1)
  if (pos >= 2 && low >= 0xdc00 && low <= 0xdfff) {
    const high = value.charCodeAt(pos - 2)
    if (high >= 0xd800 && high <= 0xdbff) return value.slice(pos - 2, pos)
  }
  return value.slice(pos - 1, pos)
}

function charAfter(value: string, pos: number): string {
  if (pos >= value.length) return ''
  const cp = value.codePointAt(pos)
  return cp === undefined ? '' : String.fromCodePoint(cp)
}

const isWordChar = (ch: string) => ch !== '' && WORD_CHAR.test(ch)

function wordAt(value: string, pos: number): [number, number] | null {
  let start = pos
  let end = pos
  for (let ch = charBefore(value, start); isWordChar(ch);) {
    start -= ch.length
    ch = charBefore(value, start)
  }
  for (let ch = charAfter(value, end); isWordChar(ch);) {
    end += ch.length
    ch = charAfter(value, end)
  }
  return start === end ? null : [start, end]
}

/** Shrinks a range so it neither starts nor ends with whitespace. */
function trimRange(
  value: string,
  start: number,
  end: number,
): [number, number] {
  while (start < end && /\s/.test(value[start]!)) start++
  while (end > start && /\s/.test(value[end - 1]!)) end--
  return [start, end]
}

/** Range an inline command targets; equal bounds mean "insert at caret". */
function inlineTarget(sel: TextSel): [number, number] {
  if (sel.start === sel.end)
    return wordAt(sel.value, sel.start) ?? [sel.start, sel.start]
  const [start, end] = trimRange(sel.value, sel.start, sel.end)
  return start === end ? [sel.start, sel.start] : [start, end]
}

// `_` emphasis is not recognized inside words (snake_case), so it must not
// be treated as an existing marker there.
function flanksOk(
  cmd: InlineCommand,
  value: string,
  start: number,
  end: number,
) {
  if (cmd !== 'italic') return true
  return (
    !isWordChar(charBefore(value, start)) && !isWordChar(charAfter(value, end))
  )
}

type Wrap = 'inside' | 'outside' | null

function wrapState(
  cmd: InlineCommand,
  value: string,
  start: number,
  end: number,
): Wrap {
  const [open, close] = INLINE[cmd]
  const inner = value.slice(start, end)
  if (
    inner.length >= open.length + close.length &&
    inner.startsWith(open) &&
    inner.endsWith(close) &&
    flanksOk(cmd, value, start, end)
  )
    return 'inside'
  if (
    start >= open.length &&
    value.slice(start - open.length, start) === open &&
    value.slice(end, end + close.length) === close &&
    flanksOk(cmd, value, start - open.length, end + close.length)
  )
    return 'outside'
  return null
}

function splice(value: string, start: number, end: number, insert: string) {
  return value.slice(0, start) + insert + value.slice(end)
}

function applyInline(
  cmd: InlineCommand,
  sel: TextSel,
  placeholder: MdPlaceholder,
): TextSel {
  const { value } = sel
  const [open, close] = INLINE[cmd]
  const [start, end] = inlineTarget(sel)
  const state = wrapState(cmd, value, start, end)
  if (state === 'inside') {
    const inner = value.slice(start + open.length, end - close.length)
    return {
      value: splice(value, start, end, inner),
      start,
      end: start + inner.length,
    }
  }
  if (state === 'outside') {
    const from = start - open.length
    return {
      value: splice(value, from, end + close.length, value.slice(start, end)),
      start: from,
      end: end - open.length,
    }
  }
  const text = start === end ? placeholder.text : value.slice(start, end)
  return {
    value: splice(value, start, end, open + text + close),
    start: start + open.length,
    end: start + open.length + text.length,
  }
}

interface LineSpan {
  from: number
  to: number
  lines: string[]
}

const lineStart = (value: string, pos: number) =>
  pos <= 0 ? 0 : value.lastIndexOf('\n', pos - 1) + 1

/** Whole lines touched by [start, end]; a range ending right after LF excludes the next line. */
function touchedLines(value: string, start: number, end: number): LineSpan {
  const last = end > start && value[end - 1] === '\n' ? end - 1 : end
  const from = lineStart(value, start)
  const next = value.indexOf('\n', Math.max(last, from))
  const to = next === -1 ? value.length : next
  return { from, to, lines: value.slice(from, to).split('\n') }
}

function multiLineRange(sel: TextSel): [number, number] | null {
  const [start, end] = trimRange(sel.value, sel.start, sel.end)
  return start < end && sel.value.slice(start, end).includes('\n')
    ? [start, end]
    : null
}

interface Fence {
  /** Range removed to unwrap, and the content range inside it. */
  outer: [number, number]
  inner: [number, number]
}

function findFence(value: string, start: number, end: number): Fence | null {
  const { lines, from, to } = touchedLines(value, start, end)
  if (
    lines.length >= 2 &&
    lines[0]!.startsWith(FENCE) &&
    lines[lines.length - 1]!.trim() === FENCE
  ) {
    const innerFrom = from + lines[0]!.length + 1
    const innerTo = to - lines[lines.length - 1]!.length - 1
    return {
      outer: [from, to],
      inner: [innerFrom, Math.max(innerFrom, innerTo)],
    }
  }
  if (from === 0 || to >= value.length) return null
  const prevFrom = lineStart(value, from - 1)
  const nextEnd = value.indexOf('\n', to + 1)
  const nextTo = nextEnd === -1 ? value.length : nextEnd
  if (
    value.slice(prevFrom, from - 1).startsWith(FENCE) &&
    value.slice(to + 1, nextTo).trim() === FENCE
  )
    return { outer: [prevFrom, nextTo], inner: [from, to] }
  return null
}

function applyFence(sel: TextSel, range: [number, number]): TextSel {
  const { value } = sel
  const fence = findFence(value, range[0], range[1])
  if (fence) {
    const content = value.slice(fence.inner[0], fence.inner[1])
    const at = fence.outer[0]
    return {
      value: splice(value, fence.outer[0], fence.outer[1], content),
      start: at,
      end: at + content.length,
    }
  }
  const { from, to } = touchedLines(value, range[0], range[1])
  const content = value.slice(from, to)
  const at = from + FENCE.length + 1
  return {
    value: splice(value, from, to, `${FENCE}\n${content}\n${FENCE}`),
    start: at,
    end: at + content.length,
  }
}

/** Per-line edit: keep `keep` chars, replace the next `oldLen` chars with `insert`. */
interface LineEdit {
  keep: number
  oldLen: number
  insert: string
}

interface LineParts {
  outer: string
  body: string
  prefix: string
}

function blockParts(line: string): LineParts {
  const quote = QUOTE_PREFIX.exec(line)?.[0] ?? ''
  const ws = LEADING_WS.exec(line.slice(quote.length))![0]
  const outer = quote + ws
  const body = line.slice(outer.length)
  return { outer, body, prefix: BLOCK_PREFIX.exec(body)?.[0] ?? '' }
}

function blockType(prefix: string): BlockCommand | 'h' | null {
  if (prefix === '') return null
  if (prefix.startsWith('#')) {
    const level = prefix.length - 1
    return level <= 3 ? (`h${level}` as BlockCommand) : 'h'
  }
  if (prefix.startsWith('- [')) return 'check'
  if (/^\d/.test(prefix)) return 'ordered'
  return 'bullet'
}

const BLOCK_MARK: Record<Exclude<BlockCommand, 'ordered'>, string> = {
  h1: '# ',
  h2: '## ',
  h3: '### ',
  bullet: '- ',
  check: '- [ ] ',
}

/** Lines a line command acts on: blank lines are skipped unless every line is blank. */
function activeLines(lines: string[], blank: (line: string) => boolean) {
  const all = lines.map((_, i) => i)
  const filled = all.filter((i) => !blank(lines[i]!))
  return filled.length === 0 ? all : filled
}

const isBlankBlock = (line: string) => blockParts(line).body.trim() === ''

function blockActive(cmd: BlockCommand, lines: string[]): boolean {
  return activeLines(lines, isBlankBlock).every(
    (i) => blockType(blockParts(lines[i]!).prefix) === cmd,
  )
}

function blockEdits(cmd: BlockCommand, lines: string[]): Map<number, LineEdit> {
  const remove = blockActive(cmd, lines)
  const edits = new Map<number, LineEdit>()
  let number = 0
  for (const i of activeLines(lines, isBlankBlock)) {
    const { outer, prefix } = blockParts(lines[i]!)
    let insert = ''
    if (!remove) insert = cmd === 'ordered' ? `${++number}. ` : BLOCK_MARK[cmd]
    edits.set(i, { keep: outer.length, oldLen: prefix.length, insert })
  }
  return edits
}

const quotePrefix = (line: string) => /^> ?/.exec(line)?.[0] ?? ''

function quoteActive(lines: string[]): boolean {
  return activeLines(lines, (line) => line.trim() === '').every(
    (i) => quotePrefix(lines[i]!) !== '',
  )
}

function quoteEdits(lines: string[]): Map<number, LineEdit> {
  const remove = quoteActive(lines)
  const edits = new Map<number, LineEdit>()
  lines.forEach((line, i) => {
    const prefix = quotePrefix(line)
    if (remove) {
      if (prefix) edits.set(i, { keep: 0, oldLen: prefix.length, insert: '' })
    } else {
      edits.set(i, {
        keep: 0,
        oldLen: 0,
        insert: line === '' && lines.length > 1 ? '>' : '> ',
      })
    }
  })
  return edits
}

function mapColumn(col: number, edit: LineEdit | undefined, sticky: boolean) {
  if (!edit || col < edit.keep) return col
  if (col === edit.keep && sticky) return col
  if (col <= edit.keep + edit.oldLen) return edit.keep + edit.insert.length
  return col - edit.oldLen + edit.insert.length
}

function applyLines(sel: TextSel, cmd: 'quote' | BlockCommand): TextSel {
  const { value, start, end } = sel
  const { from, to, lines } = touchedLines(value, start, end)
  const edits = cmd === 'quote' ? quoteEdits(lines) : blockEdits(cmd, lines)
  const out = lines.map((line, i) => {
    const edit = edits.get(i)
    if (!edit) return line
    return (
      line.slice(0, edit.keep) +
      edit.insert +
      line.slice(edit.keep + edit.oldLen)
    )
  })
  const block = out.join('\n')
  const mapPos = (pos: number, sticky: boolean) => {
    if (pos < from) return pos
    if (pos > to) return pos - (to - from) + block.length
    let offset = from
    let rest = pos - from
    let mapped = from
    for (let i = 0; i < lines.length; i++) {
      const len = lines[i]!.length
      if (rest <= len) {
        mapped = offset + mapColumn(rest, edits.get(i), sticky)
        break
      }
      rest -= len + 1
      offset += out[i]!.length + 1
    }
    return mapped
  }
  const collapsed = start === end
  return {
    value: value.slice(0, from) + block + value.slice(to),
    start: mapPos(start, !collapsed),
    end: collapsed ? mapPos(start, false) : mapPos(end, false),
  }
}

function applyLink(
  sel: TextSel,
  image: boolean,
  placeholder: MdPlaceholder,
): TextSel {
  const { value, start, end } = sel
  const selected = value.slice(start, end)
  const text = selected.trim() === '' ? placeholder.text : selected
  const head = `${image ? '!' : ''}[${text}](`
  const insert = `${head}${placeholder.url})`
  const urlStart = start + head.length
  return {
    value: splice(value, start, end, insert),
    start: urlStart,
    end: urlStart + placeholder.url.length,
  }
}

/**
 * Applies a toolbar command to plain text, inserting or toggling Markdown
 * syntax. Results that would exceed the long_text bound return the input
 * unchanged.
 */
export function applyCommand(
  cmd: MdCommand,
  sel: TextSel,
  placeholder: MdPlaceholder,
): TextSel {
  const input = normalize(sel)
  let result: TextSel
  if (cmd === 'link' || cmd === 'image')
    result = applyLink(input, cmd === 'image', placeholder)
  else if (cmd === 'quote' || isBlock(cmd)) result = applyLines(input, cmd)
  else {
    const fence = cmd === 'code' ? multiLineRange(input) : null
    result = fence
      ? applyFence(input, fence)
      : applyInline(cmd, input, placeholder)
  }
  if (
    result.value.length > MAX_LONG_TEXT_LENGTH &&
    codePoints(result.value) > MAX_LONG_TEXT_LENGTH
  )
    return { value: sel.value, start: sel.start, end: sel.end }
  return result
}

/** Toggle state of a command for the current selection (for aria-pressed). */
export function isActive(cmd: MdCommand, sel: TextSel): boolean {
  if (cmd === 'link' || cmd === 'image') return false
  const input = normalize(sel)
  const { value, start, end } = input
  if (cmd === 'quote') return quoteActive(touchedLines(value, start, end).lines)
  if (isBlock(cmd))
    return blockActive(cmd, touchedLines(value, start, end).lines)
  if (cmd === 'code') {
    const range = multiLineRange(input)
    if (range) return findFence(value, range[0], range[1]) !== null
  }
  const [from, to] = inlineTarget(input)
  return wrapState(cmd, value, from, to) !== null
}
