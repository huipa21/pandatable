import { i18n } from '../i18n'

export class RequestError extends Error {
  constructor(
    readonly code: string,
    readonly status: number,
  ) {
    super(code)
  }
}

export async function request<T>(
  path: string,
  method = 'GET',
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    method,
    cache: 'no-store',
    credentials: 'same-origin',
    headers:
      method === 'GET'
        ? undefined
        : { 'Content-Type': 'application/json', 'X-PT-CSRF': '1' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
  const result: unknown = await response.json()
  if (!response.ok) {
    const code =
      typeof result === 'object' &&
      result !== null &&
      'error' in result &&
      typeof result.error === 'object' &&
      result.error !== null &&
      'code' in result.error &&
      typeof result.error.code === 'string'
        ? result.error.code
        : 'ERR_INTERNAL'
    throw new RequestError(code, response.status)
  }
  return result as T
}

export function errorMessage(error: unknown): string {
  const code = error instanceof RequestError ? error.code : 'ERR_INTERNAL'
  const { t } = i18n.global
  switch (code) {
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
