import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { useResourceSearch, type ResourceSearchItem } from './useResourceSearch'

const items: ResourceSearchItem[] = [
  { id: 'plain', name: 'Alpha table', lastOpenedAt: null, index: 0 },
  { id: 'description', name: 'Other', description: 'Shared alpha', lastOpenedAt: null, index: 1 },
  { id: 'contains', name: 'An alpha table', lastOpenedAt: null, index: 2 },
  { id: 'prefix', name: 'Alpha', lastOpenedAt: null, index: 3 },
]

describe('useResourceSearch', () => {
  it('orders recent resources first, then the pinned item, then server order', () => {
    const query = ref('')
    const result = useResourceSearch(
      [
        { id: 'first', name: 'First', lastOpenedAt: null, index: 0 },
        { id: 'pinned', name: 'Pinned', lastOpenedAt: null, index: 2 },
        { id: 'older', name: 'Older', lastOpenedAt: '2025-01-01T00:00:00Z', index: 1 },
        { id: 'newer', name: 'Newer', lastOpenedAt: '2025-02-01T00:00:00Z', index: 3 },
      ],
      query,
      'pinned',
    )
    expect(result.value.map((item) => item.id)).toEqual([
      'newer',
      'older',
      'pinned',
      'first',
    ])
  })

  it('ranks name prefix, name contains and description matches, using recency for ties', () => {
    const query = ref('alpha')
    const result = useResourceSearch(items, query)
    expect(result.value.map((item) => item.id)).toEqual([
      'plain',
      'prefix',
      'contains',
      'description',
    ])
  })

  it('uses NFKC and case-folded matching for CJK and accented names', () => {
    const query = ref('ＡＬＰＨＡ')
    const cjk = { id: 'cjk', name: '表格 Alpha', lastOpenedAt: null, index: 1 }
    const accented = {
      id: 'accented',
      name: 'CAFÉ',
      lastOpenedAt: null,
      index: 0,
    }
    expect(useResourceSearch([cjk, ...items], query).value.map(({ id }) => id))
      .toContain('cjk')
    query.value = '表格'
    expect(useResourceSearch([cjk], query).value.map(({ id }) => id)).toEqual([
      'cjk',
    ])
    query.value = 'cafe\u0301'
    expect(useResourceSearch([accented], query).value.map(({ id }) => id)).toEqual([
      'accented',
    ])
  })

  it('treats a whitespace-only query as no query', () => {
    const query = ref(' \t　 ')
    const result = useResourceSearch(items, query)
    expect(result.value.map((item) => item.id)).toEqual([
      'plain',
      'description',
      'contains',
      'prefix',
    ])
  })

  it('falls back to the pinned item before server order when nothing is recent', () => {
    const query = ref('')
    const result = useResourceSearch(items, query, 'contains')
    expect(result.value.map((item) => item.id)).toEqual([
      'contains',
      'plain',
      'description',
      'prefix',
    ])
  })
})
