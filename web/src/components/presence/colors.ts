const colors = [
  '#176b5b',
  '#8a3975',
  '#375da5',
  '#9c4a24',
  '#6848a1',
  '#984152',
  '#246a7d',
  '#6c5a14',
]

export function presenceColor(userId: string): string {
  let hash = 2166136261
  for (const character of userId)
    hash = Math.imul(hash ^ character.charCodeAt(0), 16777619)
  return colors[(hash >>> 0) % colors.length]!
}
