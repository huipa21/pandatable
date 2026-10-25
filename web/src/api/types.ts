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

export interface AdminUser {
  id: string
  username: string
  email: string | null
  displayName: string
  role: 'admin' | 'user'
  status: 'active' | 'disabled'
  mustChangePassword: boolean
  lastLoginAt: string | null
  createdAt: string
  updatedAt: string
}

export interface UserListing {
  items: AdminUser[]
  total: number
}

export interface CreateUserRequest {
  username: string
  email?: string | null
  displayName: string
  role: AdminUser['role']
  password: string
}

export type AdminUserPatch = Partial<
  Pick<AdminUser, 'email' | 'displayName' | 'role' | 'status'>
>

export interface AdminUserMutation extends AuthMutation {
  user: AdminUser
  sessionsRevoked?: boolean
}

export interface AdminGroup {
  id: string
  name: string
  description: string
  createdAt: string
  memberCount: number
}

export interface GroupListing {
  items: AdminGroup[]
  total: number
}

export interface GroupMutation extends AuthMutation {
  group: AdminGroup
}
