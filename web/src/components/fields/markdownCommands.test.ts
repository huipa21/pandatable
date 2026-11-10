import { describe, expect, it } from 'vitest'
import {
  applyCommand,
  isActive,
  MD_SHORTCUTS,
  type MdCommand,
  type TextSel,
} from './markdownCommands'
import { MAX_LONG_TEXT_LENGTH } from '../../fields/text'

const PH = { text: 'text', url: 'https://' }

// `«…»` marks a selection and `|` a caret.
function sel(marked: string): TextSel {
  const caret = marked.indexOf('|')
  if (caret >= 0)
    return { value: marked.replace('|', ''), start: caret, end: caret }
  const start = marked.indexOf('«')
  const end = marked.indexOf('»') - 1
  return { value: marked.replace('«', '').replace('»', ''), start, end }
}

function show({ value, start, end }: TextSel): string {
  if (start === end) return `${value.slice(0, start)}|${value.slice(start)}`
  return `${value.slice(0, start)}«${value.slice(start, end)}»${value.slice(end)}`
}

const run = (cmd: MdCommand, marked: string) =>
  show(applyCommand(cmd, sel(marked), PH))

describe('CES-03.1 inline commands', () => {
  it('bold wraps the selection and toggles off', () => {
    const on = applyCommand('bold', sel('say «hello» now'), PH)
    expect(show(on)).toBe('say **«hello»** now')
    expect(isActive('bold', on)).toBe(true)
    const off = applyCommand('bold', on, PH)
    expect(show(off)).toBe('say «hello» now')
    expect(isActive('bold', off)).toBe(false)
    expect(run('bold', 'say «**hello**» now')).toBe('say «hello» now')
  })

  it('bolds the word at the caret', () => {
    expect(run('bold', 'say hel|lo now')).toBe('say **«hello»** now')
    expect(run('bold', 'say **hel|lo** now')).toBe('say «hello» now')
  })

  it('inserts and selects the placeholder when there is no word', () => {
    const on = applyCommand('italic', sel('a | b'), PH)
    expect(show(on)).toBe('a _«text»_ b')
    expect(show(applyCommand('italic', on, PH))).toBe('a «text» b')
  })

  it('underline uses <u> tags; strike and code use their markers', () => {
    expect(run('underline', '«word»')).toBe('<u>«word»</u>')
    expect(run('underline', '<u>wo|rd</u>')).toBe('«word»')
    expect(run('strike', '«x»')).toBe('~~«x»~~')
    expect(run('code', 'call fo|o()')).toBe('call `«foo»`()')
  })

  it('keeps surrounding whitespace outside the markers', () => {
    expect(run('bold', 'a« b »c')).toBe('a **«b»** c')
  })

  it('does not treat intraword underscores as italic markers', () => {
    expect(isActive('italic', sel('snake_«case»_name'))).toBe(false)
    expect(isActive('italic', sel('_«case»_'))).toBe(true)
  })

  it('maps mod shortcuts to bold, italic and underline', () => {
    expect(MD_SHORTCUTS).toEqual({
      'mod+b': 'bold',
      'mod+i': 'italic',
      'mod+u': 'underline',
    })
  })
})

describe('CES-03.2 line commands', () => {
  it('h2 prefixes every touched line and h1 replaces it', () => {
    const h2 = applyCommand('h2', sel('o«ne\ntw»o\nthree'), PH)
    expect(h2.value).toBe('## one\n## two\nthree')
    expect(isActive('h2', h2)).toBe(true)
    const h1 = applyCommand('h1', h2, PH)
    expect(h1.value).toBe('# one\n# two\nthree')
    expect(isActive('h2', h1)).toBe(false)
    expect(applyCommand('h1', h1, PH).value).toBe('one\ntwo\nthree')
  })

  it('bullet toggles off only when all lines have it', () => {
    const mixed = applyCommand('bullet', sel('«- a\nb»'), PH)
    expect(mixed.value).toBe('- a\n- b')
    expect(applyCommand('bullet', mixed, PH).value).toBe('a\nb')
  })

  it('ordered renumbers 1., 2. and replaces other list markers', () => {
    expect(applyCommand('ordered', sel('«a\n- b\n\nc»'), PH).value).toBe(
      '1. a\n2. b\n\n3. c',
    )
    expect(applyCommand('ordered', sel('«1. a\n7. b»'), PH).value).toBe('a\nb')
  })

  it('quote and checklist prefix lines; caret moves after a new prefix', () => {
    expect(run('quote', 'a|b')).toBe('> a|b')
    expect(run('quote', '> a|b')).toBe('a|b')
    expect(run('check', 'x\n|')).toBe('x\n- [ ] |')
    expect(applyCommand('bullet', sel('> «a\n> b»'), PH).value).toBe(
      '> - a\n> - b',
    )
  })

  it('a selection ending right after a line break excludes the next line', () => {
    expect(applyCommand('bullet', sel('«a\n»b'), PH).value).toBe('- a\nb')
  })
})

describe('CES-03.3 link, fence and bounds', () => {
  it('link and image select the URL placeholder', () => {
    expect(run('link', 'see «docs»')).toBe('see [docs](«https://»)')
    expect(run('link', 'see |')).toBe('see [text](«https://»)')
    expect(run('image', '|')).toBe('![text](«https://»)')
    expect(isActive('link', sel('[a](b)'))).toBe(false)
  })

  it('multi-line code creates a fence on separate lines and toggles off', () => {
    const on = applyCommand('code', sel('x\n«a\nb»\ny'), PH)
    expect(show(on)).toBe('x\n```\n«a\nb»\n```\ny')
    expect(isActive('code', on)).toBe(true)
    expect(show(applyCommand('code', on, PH))).toBe('x\n«a\nb»\ny')
  })

  it('returns the input unchanged when the result would be too long', () => {
    const input = { value: 'a'.repeat(MAX_LONG_TEXT_LENGTH), start: 0, end: 1 }
    expect(applyCommand('bold', input, PH)).toEqual(input)
    expect(applyCommand('h1', input, PH)).toEqual(input)
  })

  it('normalizes CR line breaks to LF', () => {
    const out = applyCommand(
      'bullet',
      { value: 'a\r\nb', start: 0, end: 4 },
      PH,
    )
    expect(out).toEqual({ value: '- a\n- b', start: 0, end: 7 })
  })
})
