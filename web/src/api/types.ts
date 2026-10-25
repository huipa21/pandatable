export interface AuthUser {
  id: string
  username: string
  email: string | null
  displayName: string
  role: 'admin' | 'user'
  mustChangePassword: boolean
  locale: 'en' | 'zh-CN'
  theme: 'light' | 'dark' | 'system'
  timezone: string | null
  effectiveTimezone: string
  avatarColor: string
}

export interface LoginRequest {
  login: string
  password: string
}

export interface SetupRequest {
  token: string
  username: string
  email: string
  displayName: string
  password: string
  locale: 'en' | 'zh-CN'
}

export interface SetupStatus {
  needed: boolean
}

export interface PasswordRequest {
  current: string
  new: string
}

export type ProfilePatch = Partial<
  Pick<AuthUser, 'displayName' | 'locale' | 'theme' | 'timezone'>
>

export interface AuthMutation {
  eventId: string
}

export interface AuthUserMutation extends AuthMutation {
  user: AuthUser
}

export interface APIError {
  error: {
    code: string
    message: string
    details?: Record<string, unknown>
  }
}
