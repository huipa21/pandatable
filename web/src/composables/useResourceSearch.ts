import { computed, toValue, type MaybeRefOrGetter, type Ref } from 'vue'

export interface ResourceSearchItem {
  id: string
  name: string
  description?: string
  lastOpenedAt: string | null
  index: number
}

function fold(value: string) {
  return value
    .normalize('NFKC')
    .toLowerCase()
    .replace(/ß/g, 'ss')
    .replace(/ς/g, 'σ')
}

function openedTime(value: string | null) {
  if (!value) return null
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? null : parsed
}

export function useResourceSearch<T extends ResourceSearchItem>(
  items: MaybeRefOrGetter<readonly T[]>,
  query: Ref<string>,
  pinnedId?: MaybeRefOrGetter<string | null | undefined>,
) {
  return computed(() => {
    const source = [...toValue(items)]
    const pinned = toValue(pinnedId)
    const noQueryOrder = (left: T, right: T) => {
      const leftOpened = openedTime(left.lastOpenedAt)
      const rightOpened = openedTime(right.lastOpenedAt)
      if (leftOpened !== rightOpened) {
        if (leftOpened === null) return 1
        if (rightOpened === null) return -1
        return rightOpened - leftOpened
      }
      if (left.id === pinned && right.id !== pinned) return -1
      if (right.id === pinned && left.id !== pinned) return 1
      return left.index - right.index
    }

    const needle = fold(query.value.trim())
    if (!needle) return source.sort(noQueryOrder)

    return source
      .map((item) => {
        const name = fold(item.name)
        const description = fold(item.description ?? '')
        const rank = name.startsWith(needle)
          ? 0
          : name.includes(needle)
            ? 1
            : description.includes(needle)
              ? 2
              : -1
        return { item, rank }
      })
      .filter(({ rank }) => rank >= 0)
      .sort(
        (left, right) =>
          left.rank - right.rank || noQueryOrder(left.item, right.item),
      )
      .map(({ item }) => item)
  })
}
