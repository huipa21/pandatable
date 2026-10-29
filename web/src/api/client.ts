import { i18n } from '../i18n'
import type { AttachmentValue } from './types'

/** Safe, bounded error details; server messages are never shown. */
export interface ErrorDetails {
  reason?: string
  option?: string
  count?: number
  relatedRowId?: string
}

export class RequestError extends Error {
  constructor(
    readonly code: string,
    readonly status: number,
    readonly details: ErrorDetails = {},
  ) {
    super(code)
  }
}

function readDetails(value: unknown): ErrorDetails {
  if (typeof value !== 'object' || value === null) return {}
  const details: ErrorDetails = {}
  const raw = value as Record<string, unknown>
  if (typeof raw.reason === 'string' && /^[a-z_]{1,40}$/.test(raw.reason))
    details.reason = raw.reason
  if (typeof raw.option === 'string' && /^[A-Za-z]{1,40}$/.test(raw.option))
    details.option = raw.option
  if (
    typeof raw.count === 'number' &&
    Number.isSafeInteger(raw.count) &&
    raw.count >= 0
  )
    details.count = raw.count
  if (
    typeof raw.relatedRowId === 'string' &&
    /^[1-9]\d{0,18}$/.test(raw.relatedRowId)
  )
    details.relatedRowId = raw.relatedRowId
  return details
}

export interface RequestOptions {
  /** Field/view version for optimistic concurrency. */
  ifMatch?: number
}

export async function request<T>(
  path: string,
  method = 'GET',
  body?: unknown,
  signal?: AbortSignal,
  options: RequestOptions = {},
): Promise<T> {
  const headers: Record<string, string> | undefined =
    method === 'GET'
      ? undefined
      : { 'Content-Type': 'application/json', 'X-PT-CSRF': '1' }
  if (headers && options.ifMatch !== undefined)
    headers['If-Match'] = String(options.ifMatch)
  const response = await fetch(`/api/v1${path}`, {
    method,
    cache: 'no-store',
    credentials: 'same-origin',
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
  const result: unknown = await response.json()
  if (!response.ok) {
    const failure =
      typeof result === 'object' &&
      result !== null &&
      'error' in result &&
      typeof result.error === 'object' &&
      result.error !== null
        ? (result.error as Record<string, unknown>)
        : {}
    const code =
      typeof failure.code === 'string' ? failure.code : 'ERR_INTERNAL'
    throw new RequestError(code, response.status, readDetails(failure.details))
  }
  return result as T
}

export async function uploadAttachment(
  tableId: string,
  fieldId: string,
  file: File,
  signal?: AbortSignal,
): Promise<AttachmentValue> {
  const form = new FormData()
  form.append('file', file)
  const response = await fetch(
    `/api/v1/tables/${encodeURIComponent(tableId)}/fields/${encodeURIComponent(fieldId)}/attachments`,
    {
      method: 'POST',
      cache: 'no-store',
      credentials: 'same-origin',
      headers: { 'X-PT-CSRF': '1' },
      body: form,
      signal,
    },
  )
  const result: unknown = await response.json()
  if (!response.ok) {
    const failure =
      typeof result === 'object' &&
      result !== null &&
      'error' in result &&
      typeof result.error === 'object' &&
      result.error !== null
        ? (result.error as Record<string, unknown>)
        : {}
    throw new RequestError(
      typeof failure.code === 'string' ? failure.code : 'ERR_INTERNAL',
      response.status,
      readDetails(failure.details),
    )
  }
  return result as AttachmentValue
}

export async function deleteAttachment(
  tableId: string,
  fieldId: string,
  attachmentId: string,
): Promise<void> {
  await request(
    `/tables/${encodeURIComponent(tableId)}/fields/${encodeURIComponent(fieldId)}/attachments/${encodeURIComponent(attachmentId)}`,
    'DELETE',
  )
}

export async function discardAttachment(
  tableId: string,
  fieldId: string,
  attachmentId: string,
): Promise<void> {
  await request(
    `/tables/${encodeURIComponent(tableId)}/fields/${encodeURIComponent(fieldId)}/attachments/${encodeURIComponent(attachmentId)}/discard`,
    'POST',
  )
}

export function errorMessage(error: unknown): string {
  const code = error instanceof RequestError ? error.code : 'ERR_INTERNAL'
  const count = error instanceof RequestError ? (error.details.count ?? 0) : 0
  const { t } = i18n.global
  switch (code) {
    case 'ERR_FIELD_NAME_CONFLICT':
      return t('errors.ERR_FIELD_NAME_CONFLICT')
    case 'ERR_VERSION_CONFLICT':
      return t('errors.ERR_VERSION_CONFLICT')
    case 'ERR_REQUIRED_HAS_EMPTY':
      return t('errors.ERR_REQUIRED_HAS_EMPTY', { count })
    case 'ERR_UNIQUE_HAS_DUPLICATES':
      return t('errors.ERR_UNIQUE_HAS_DUPLICATES', { count })
    case 'ERR_DISPLAY_FIELD_REQUIRED':
      return t('errors.ERR_DISPLAY_FIELD_REQUIRED')
    case 'ERR_RELATION_CONFLICT':
      return t('errors.ERR_RELATION_CONFLICT')
    case 'ERR_RESTRICT_VIOLATION':
      return t('errors.ERR_RESTRICT_VIOLATION')
    case 'ERR_REQUIRED_FIELD':
      return t('errors.ERR_REQUIRED_FIELD')
    case 'ERR_UNIQUE_CONFLICT':
      return t('errors.ERR_UNIQUE_CONFLICT')
    case 'ERR_SCHEMA_CHANGED':
      return t('errors.ERR_SCHEMA_CHANGED')
    case 'ERR_UPLOAD_TOO_LARGE':
      return t('errors.ERR_UPLOAD_TOO_LARGE')
    case 'ERR_ATTACHMENT_TYPE':
      return t('errors.ERR_ATTACHMENT_TYPE')
    case 'ERR_IMAGE_TOO_LARGE':
      return t('errors.ERR_IMAGE_TOO_LARGE')
    case 'ERR_INVALID_IMAGE':
      return t('errors.ERR_INVALID_IMAGE')
    case 'ERR_ATTACHMENT_IN_USE':
      return t('errors.ERR_ATTACHMENT_IN_USE')
    case 'ERR_VALIDATION':
      return t('errors.ERR_VALIDATION')
    case 'ERR_UNAUTHENTICATED':
      return t('errors.ERR_UNAUTHENTICATED')
    case 'ERR_INVALID_CREDENTIALS':
      return t('errors.ERR_INVALID_CREDENTIALS')
    case 'ERR_PASSWORD_CHANGE_REQUIRED':
      return t('errors.ERR_PASSWORD_CHANGE_REQUIRED')
    case 'ERR_PERMISSION_DENIED':
      return t('errors.ERR_PERMISSION_DENIED')
    case 'ERR_IDENTITY_CONFLICT':
      return t('errors.ERR_IDENTITY_CONFLICT')
    case 'ERR_LAST_ADMIN':
      return t('errors.ERR_LAST_ADMIN')
    case 'ERR_GROUP_NAME_CONFLICT':
      return t('errors.ERR_GROUP_NAME_CONFLICT')
    case 'ERR_MEMBERSHIP_EXISTS':
      return t('errors.ERR_MEMBERSHIP_EXISTS')
    case 'ERR_PRINCIPAL_DISABLED':
      return t('errors.ERR_PRINCIPAL_DISABLED')
    case 'ERR_PRINCIPAL_NOT_FOUND':
      return t('errors.ERR_PRINCIPAL_NOT_FOUND')
    case 'ERR_LAST_OWNER':
      return t('errors.ERR_LAST_OWNER')
    case 'ERR_AUTH_BUSY':
      return t('errors.ERR_AUTH_BUSY')
    case 'ERR_RATE_LIMITED':
      return t('errors.ERR_RATE_LIMITED')
    case 'ERR_CSRF':
      return t('errors.ERR_CSRF')
    case 'ERR_NOT_FOUND':
      return t('errors.ERR_NOT_FOUND')
    case 'ERR_SYSTEM_UNAVAILABLE':
      return t('errors.ERR_SYSTEM_UNAVAILABLE')
    case 'ERR_DDL_CONFLICT':
      return t('errors.ERR_DDL_CONFLICT')
    case 'ERR_DDL_LOCK_TIMEOUT':
      return t('errors.ERR_DDL_LOCK_TIMEOUT')
    default:
      return t('errors.ERR_INTERNAL')
  }
}
