import type { Op } from '../fields/types'

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
  mentionEmail: boolean
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
  Pick<
    AuthUser,
    'displayName' | 'locale' | 'theme' | 'timezone' | 'mentionEmail'
  >
>

export interface AuthMutation {
  eventId: string
}

export interface AuthUserMutation extends AuthMutation {
  user: AuthUser
}

export interface WorkspacePreference {
  workspaceId: string | null
}

export interface WorkspacePreferenceMutation extends AuthMutation {
  workspaceId: string
}

export interface SetWorkspacePreferenceRequest {
  workspaceId: string
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

export interface AppSettings {
  appName: string
  defaultLocale: 'en' | 'zh-CN'
}

export interface AdminSettings extends AppSettings {
  defaultTimezone: string
  defaultCurrency: string
  uploadMaxMb: number
  imageMaxMegapixels: number
  historyRetentionDays: number
  trashRetentionDays: number
}

export interface AttachmentValue {
  id: string
  name: string
  mime: string
  size: number
  thumb: string | null
}

export interface SettingsMutation extends AuthMutation {
  settings: AdminSettings
}

export interface SMTPSettings {
  enabled: boolean
  host: string
  port: number
  security: 'starttls' | 'tls' | 'none'
  hasUsername: boolean
  hasPassword: boolean
  fromAddress: string
}

export type SMTPPatch = Partial<
  Omit<SMTPSettings, 'hasUsername' | 'hasPassword'>
> & {
  username?: string
  password?: string
}

export interface AdminWorkspace {
  id: string
  name: string
  owners: {
    id: string
    type: 'user' | 'group'
    name: string
    username: string | null
    status: 'active' | 'disabled' | null
  }[]
  ownerTotal: number
  orphaned: boolean
  attachmentBytes: number
}

export interface WorkspaceListing {
  items: AdminWorkspace[]
  total: number
}

export interface AdminOwnership {
  resourceType: 'workspace' | 'book' | 'table'
  resourceId: string
  name: string
  workspaceId: string
  workspaceName: string
  trashed: boolean
}

export interface OwnershipListing {
  items: AdminOwnership[]
  total: number
}

export interface BackupRecord {
  id: string
  fileName: string
  sizeBytes: number | null
  status: 'running' | 'done' | 'failed'
  errorCode?: string
  createdBy: string | null
  createdByName: string
  createdAt: string
  finishedAt: string | null
  progress?: {
    stage: 'dumping' | 'archiving' | 'finalizing'
    percent: number
  }
}

export interface BackupListing {
  items: BackupRecord[]
  freeBytes: number
}

export type ResourceKind = 'workspace' | 'book' | 'table'
export type TrashKind = ResourceKind | 'field' | 'view'

export interface TrashItem {
  kind: TrashKind
  id: string
  name: string
  workspaceId: string
  workspaceName: string
  parentId: string
  parentName: string
  deletedAt: string
  expiresAt: string
}

export interface TrashListing {
  items: TrashItem[]
}

export type AccessLevel = 'none' | 'read' | 'write' | 'owner'
export type GrantLevel = Exclude<AccessLevel, 'none'>
export type PrincipalType = 'user' | 'group'

export interface Grant {
  id: string
  resourceType: ResourceKind
  resourceId: string
  principalType: PrincipalType
  principalId: string
  level: GrantLevel
  grantedBy: string | null
  createdAt: string
  principal: {
    name: string
    username: string | null
    status: 'active' | 'disabled' | null
  } | null
}

export interface DirectGrant extends Grant {
  inheritedLevel: GrantLevel | null
}

export interface GrantListing {
  resource: { type: ResourceKind; id: string }
  effectiveLevel: AccessLevel
  items: DirectGrant[]
  inherited: Grant[]
}

export interface DirectoryPrincipal {
  id: string
  name: string
  username: string | null
}

export interface DirectoryListing {
  items: DirectoryPrincipal[]
  hasMore: boolean
}

export interface NavView {
  id: string
  name: string
  type: string
  scope: string
}

export interface NavTable {
  id: string
  name: string
  position: number
  schemaVersion: number
  level: AccessLevel
  views: NavView[]
}

export interface NavBook {
  id: string
  name: string
  position: number
  level: AccessLevel
  pathOnly: boolean
  tables: NavTable[]
}

export interface NavWorkspace {
  id: string
  name: string
  icon: string | null
  color: string | null
  createdAt: string
  level: AccessLevel
  pathOnly: boolean
  books: NavBook[]
}

export interface Navigation {
  workspaces: NavWorkspace[]
}

export interface ViewColumns {
  fields: { id: string; visible: boolean; width?: number }[]
  frozen: number
  filter?: FilterNode | null
  rowHeight?: 'short' | 'medium' | 'tall' | 'extra'
  sort?: { field: string; dir: 'asc' | 'desc' }[]
  groupBy?: string[]
  aggregates?: ViewAggregate[]
  highlight?: HighlightRule[]
  form?: FormConfig
  kanban?: KanbanConfig
  gallery?: GalleryConfig
}

export interface KanbanConfig {
  groupField: string | null
  fields: string[]
}

export interface GalleryConfig {
  coverField: string | null
  fields: string[]
}

export interface FormConfig {
  title: string
  description: string
  fields: FormFieldConfig[]
  afterSubmit: 'message' | 'new'
}

export interface FormFieldConfig {
  id: string
  label: string
  help: string
  required: boolean
}

export interface ViewAggregate {
  fieldId: string
  aggregate: string
}

export type HighlightColor =
  'red-2' | 'orange-2' | 'amber-3' | 'green-2' | 'blue-2' | 'purple-2'

export type HighlightTarget =
  { type: 'row' } | { type: 'cells'; fields: string[] }

export interface HighlightRule {
  id: string
  when: FilterNode
  target: HighlightTarget
  color: HighlightColor
}

export interface TableView extends NavView {
  locked: boolean
  position: number
  version: number
  config: ViewColumns
}

export interface ViewMutation extends AuthMutation {
  view: TableView
}

export interface TableSchema {
  table: {
    id: string
    bookId: string
    name: string
    displayFieldId: string | null
    position: number
    schemaVersion: number
    createdAt: string
  }
  fields: {
    id: string
    name: string
    description: string
    uiType: string
    options: Record<string, unknown>
    filterable?: boolean
    filterOps?: Op[]
    isSystem: boolean
    isRequired: boolean
    isUnique: boolean
    /** Canonical wire default (D-115/D-127) or null. */
    default: null | string | number | boolean | string[]
    position: number
    version: number
  }[]
  relations: Relation[]
  views: TableView[]
  schemaVersion: number
  myLevel: AccessLevel
}

export type TableField = TableSchema['fields'][number]

export interface Relation {
  id: string
  kind: 'one_one' | 'one_many' | 'many_many'
  parentTableId: string
  childTableId: string
  parentFieldId: string
  childFieldId: string
  onDelete: 'set_null' | 'cascade' | 'restrict'
}

export interface RelationLink {
  id: string
  label: string
}

/** Field mutation response (D-123); `field` is absent after deletion. */
export interface FieldMutation extends AuthMutation {
  field?: TableField
  schema: TableSchema
}

export interface TypeChangeDryRun {
  total: number
  convertible: number
  failing: number
  samples: { id: string; value: string }[]
}

export interface FormulaCheck {
  type?: string
  error?: { position: number; message: string }
}

export type RowValue =
  null | boolean | number | string | RowValue[] | { [key: string]: RowValue }

export interface DataRow {
  /** Canonical positive bigint decimal string (D-115). */
  id: string
  /** Values keyed by field UUID; physical PostgreSQL names are never exposed. */
  fields: Record<string, RowValue>
  commentCount?: number
  _hl_row?: HighlightColor | null
  _hl_cells?: Record<string, HighlightColor>
  cascaded?: { tableId: string; rows: DataRow[] }[]
  restoreVersion?: string
}

export interface RowHistoryChange {
  o?: RowValue
  n?: RowValue
}

export interface RowHistory {
  id: string
  authorId: string | null
  authorName: string
  action: 'insert' | 'update' | 'delete' | 'link' | 'unlink' | 'restore'
  changes: Record<string, RowHistoryChange>
  createdAt: string
}

export interface HistoryListing {
  items: RowHistory[]
  hasMore: boolean
}

export interface MentionUser {
  id: string
  username: string
  name: string
}

export interface RowComment {
  id: string
  authorId: string | null
  authorName: string
  body: string
  mentions: MentionUser[]
  createdAt: string
  editedAt: string | null
}

export interface CommentListing {
  items: RowComment[]
  hasMore: boolean
}

export interface CommentMutation extends AuthMutation {
  comment: RowComment
}

export interface MentionNotification {
  id: string
  tableId: string
  tableName: string
  rowId: string
  commentId: string
  authorName: string
  createdAt: string
  readAt: string | null
}

export interface NotificationListing {
  items: MentionNotification[]
  unreadCount: number
  hasMore: boolean
}

export interface RowSort {
  fieldId: string
  direction: 'asc' | 'desc'
}

export interface ExportViewQuery {
  fieldIds?: string[]
  sorts?: RowSort[]
  groupBy?: string[]
  search?: string
}

export interface ImportColumn {
  index: number
  name: string
  uiType: string
  options: Record<string, unknown> | null
  examples?: string[]
  packageKey?: string
  linkTarget?: string
  relinkTargetId?: string
  targetFieldId?: string
}

export interface ImportAnalysis {
  format: 'csv' | 'xlsx' | 'package'
  encoding?: string
  delimiter?: string
  hasHeader: boolean
  sampledRows: number
  tableName?: string
  columns: ImportColumn[]
}

export interface ImportMapping {
  index: number
  name: string
  uiType: string
  options: Record<string, unknown>
  targetFieldId: string
  skip: boolean
  relink: boolean
}

export interface ImportResult {
  tableId?: string
  viewId?: string
  rows: number
  invalidCells: number
  convertedLinks?: string[]
  unavailableViews?: string[]
}

export interface ImportJob {
  id: string
  status: 'ready' | 'running' | 'done' | 'failed'
  result: ImportResult
  errorCode?: string
  hasReport: boolean
}

export type FilterNode =
  | { op: 'and' | 'or'; items: FilterNode[] }
  | { field: string; cmp: Op; value?: RowValue }

export interface RowQueryRequest {
  offset?: number
  limit?: number
  /** Fetch up to 100 specific rows while applying the current view filters. */
  ids?: string[]
  sort?: RowSort
  sorts?: RowSort[]
  includeCount?: boolean
  /** Local grid visibility override; explicit IDs may include system fields. */
  fieldIds?: string[]
  /** Transient filter override; never saved to the view config. */
  filter?: FilterNode | null
  /** Transient quick search; never saved to the view config. */
  search?: string
  /** Group values to apply before paging expanded group rows. */
  groupPath?: RowValue[]
}

export interface RowQueryResult {
  rows: DataRow[]
  /** Visible field UUIDs in view-config order. */
  fieldIds: string[]
  total: number | null
  schemaVersion: number
}

export interface RowGroupRequest {
  path: RowValue[]
  filter?: FilterNode | null
  search?: string
  fieldIds?: string[]
}

export interface RowGroupAggregate {
  fieldId: string
  aggregate: string
  value: RowValue
}

export interface RowGroupBucket {
  key: RowValue
  count: number
  aggregates: RowGroupAggregate[]
}

export interface RowGroupResult {
  groups: RowGroupBucket[]
  schemaVersion: number
}

export interface RowAggregateRequest {
  filter?: FilterNode | null
  search?: string
  /** Transient local grid visibility override. */
  fieldIds?: string[]
  /** Transient aggregate config override; never persisted by this endpoint. */
  aggregates?: ViewAggregate[]
}

export interface RowAggregate {
  fieldId: string
  aggregate: string
  value: RowValue
}

export interface RowAggregateResult {
  aggregates: RowAggregate[]
  schemaVersion: number
}

export interface RowInsertRequest {
  rows: { fields: Record<string, RowValue> }[]
}

export interface RowPatchRequest {
  rows: {
    id: string
    fields: Record<string, RowValue>
    expected?: Record<string, RowValue>
  }[]
}

export interface RowDeleteRequest {
  ids: string[]
}

export interface RowMutation extends AuthMutation {
  rows: DataRow[]
  previous?: { id: string; fields: Record<string, RowValue> }[]
}

export interface RelationDelta {
  tableId: string
  rowId: string
  fieldId: string
  targetId: string
  action: 'link' | 'unlink'
}

export interface LinkMutation extends AuthMutation {
  row: DataRow
  changes?: RelationDelta[]
}

export interface RelationMutation extends AuthMutation {
  relation: Relation
  field?: TableField
  schema: TableSchema
}

export interface RowPatchMutation extends AuthMutation {
  row: DataRow
  previous?: Record<string, RowValue>
}

export interface SystemMetric<T> {
  status: 'ready' | 'unavailable'
  reason?:
    | 'initializing'
    | 'unsupported'
    | 'notFound'
    | 'permission'
    | 'io'
    | 'limit'
    | 'timeout'
    | 'invalid'
    | 'query'
  collectedAt: string
  value: T | null
}

export interface SystemSnapshot {
  collectedAt: string
  cpuPercent: SystemMetric<number>
  load: SystemMetric<{ one: number; five: number; fifteen: number }>
  memory: SystemMetric<{
    totalBytes: number
    usedBytes: number
    availableBytes: number
  }>
  appRssBytes: SystemMetric<number>
  container: SystemMetric<{
    currentBytes: number
    limitBytes: number | null
    unlimited: boolean
  }>
  disk: SystemMetric<{
    totalBytes: number
    usedBytes: number
    freeBytes: number
  }>
  database: SystemMetric<{
    sizeBytes: number
    connections: number
    activeConnections: number
    sharedBuffersBytes: number
  }>
  filesBytes: SystemMetric<number>
  backupsBytes: SystemMetric<number>
  versions: {
    app: string
    go: string
    postgres: SystemMetric<string>
    migration: SystemMetric<number>
  }
  uptimeSeconds: number
}
