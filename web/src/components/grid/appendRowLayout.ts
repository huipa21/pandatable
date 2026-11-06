export function appendRowLayoutWidth(
  viewportWidth: number,
  columnWidths: readonly number[],
): number {
  const columnSpan = columnWidths.reduce((total, width) => total + width, 0)
  return Math.max(viewportWidth, columnSpan)
}
