import { fail, ok, type FieldKind, type Result, type WireValue } from './types'
import type { AttachmentValue } from '../api/types'

export interface AttachmentOptions {
  accept: string
  maxFiles: number
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const mimePattern = /^[a-z0-9!#$&^_.+-]+\/(?:[a-z0-9!#$&^_.+-]+|\*)$/
const imageTypes = new Set([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
])

function normalizedAccept(value: string): string | null {
  if (value.trim() === '') return ''
  const items = value.split(',').map((item) => item.trim().toLowerCase())
  if (
    items.length > 20 ||
    items.some((item) => !mimePattern.test(item) || item === '*/*' || !item) ||
    new Set(items).size !== items.length
  )
    return null
  return items.join(', ')
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    Object.getPrototypeOf(value) === Object.prototype
  )
}

function normalizeOptions(value: unknown): Result<AttachmentOptions> {
  if (value === null || value === undefined) value = {}
  if (!isRecord(value)) return fail('invalid_options')
  if (Object.keys(value).some((key) => key !== 'accept' && key !== 'maxFiles'))
    return fail('invalid_options')
  const acceptValue = value.accept ?? ''
  const accept =
    typeof acceptValue === 'string' ? normalizedAccept(acceptValue) : null
  const maxFiles = value.maxFiles ?? 10
  if (
    accept === null ||
    typeof maxFiles !== 'number' ||
    !Number.isInteger(maxFiles) ||
    maxFiles < 1 ||
    maxFiles > 20
  )
    return fail('invalid_options')
  return ok({ accept, maxFiles })
}

function normalize(value: unknown): Result<WireValue> {
  if (value === null) return ok(null)
  if (!Array.isArray(value) || value.length > 20) return fail('invalid_array')
  const seen = new Set<string>()
  const files: AttachmentValue[] = []
  for (const item of value) {
    if (
      !isRecord(item) ||
      Object.keys(item).length !== 5 ||
      !Object.hasOwn(item, 'id') ||
      !Object.hasOwn(item, 'name') ||
      !Object.hasOwn(item, 'mime') ||
      !Object.hasOwn(item, 'size') ||
      !Object.hasOwn(item, 'thumb') ||
      typeof item.id !== 'string' ||
      !uuid.test(item.id) ||
      seen.has(item.id) ||
      typeof item.name !== 'string' ||
      [...item.name].length < 1 ||
      [...item.name].length > 255 ||
      /[\p{Cc}/\\]/u.test(item.name) ||
      typeof item.mime !== 'string' ||
      !mimePattern.test(item.mime) ||
      item.mime.endsWith('/*') ||
      typeof item.size !== 'number' ||
      !Number.isSafeInteger(item.size) ||
      item.size < 0 ||
      (item.thumb !== null &&
        item.thumb !== `/api/v1/attachments/${item.id}/thumbnail`) ||
      (item.thumb !== null && !imageTypes.has(item.mime))
    )
      return fail('invalid_array')
    seen.add(item.id)
    files.push({
      id: item.id,
      name: item.name,
      mime: item.mime,
      size: item.size,
      thumb: item.thumb as string | null,
    })
  }
  return ok(files as unknown as WireValue)
}

function asFiles(value: WireValue): AttachmentValue[] {
  return Array.isArray(value) ? (value as unknown as AttachmentValue[]) : []
}

export function attachmentCover(
  value: unknown,
): Result<{ name: string; thumb: string } | null> {
  const normalized = normalize(value)
  if (!normalized.ok) return normalized
  if (normalized.value === null) return ok(null)
  if (!Array.isArray(normalized.value)) return fail('invalid_array')
  for (const item of normalized.value) {
    if (
      item !== null &&
      typeof item === 'object' &&
      !Array.isArray(item) &&
      typeof item.name === 'string' &&
      typeof item.thumb === 'string'
    )
      return ok({ name: item.name, thumb: item.thumb })
  }
  return ok(null)
}

export const attachmentKind: FieldKind<AttachmentOptions> = {
  kind: 'attachment',
  readOnly: false,
  presentation: 'attachment',
  constraints: { required: false, unique: false, default: false },
  normalizeOptions,
  normalize,
  parsePaste: () => fail('invalid_type'),
  formatCopy: (value) =>
    asFiles(value)
      .map((file) => file.name)
      .join(', '),
  formatDisplay: (value) =>
    asFiles(value)
      .map((file) => file.name)
      .join(', '),
  filterOps: () => [],
  aggregates: () => [],
}
