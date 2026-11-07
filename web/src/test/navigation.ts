import type { AuthUser, Navigation, TableSchema } from '../api/types'

export const navUser: AuthUser = {
  id: 'alice',
  username: 'alice',
  email: null,
  displayName: 'Alice',
  role: 'user',
  mustChangePassword: false,
  locale: 'en',
  theme: 'light',
  timezone: null,
  effectiveTimezone: 'Asia/Shanghai',
  avatarColor: '#123456',
  mentionEmail: false,
}

export function navigation(): Navigation {
  return {
    workspaces: [
      {
        id: 'w',
        name: 'Workspace',
        icon: null,
        color: null,
        description: 'Workspace description',
        fallbackIcon: 'star',
        createdAt: '',
        lastOpenedAt: null,
        level: 'owner',
        pathOnly: false,
        books: [
          {
            id: 'b',
            name: 'Book',
            icon: null,
            fallbackIcon: 'book',
            lastOpenedAt: null,
            position: 1024,
            level: 'owner',
            pathOnly: false,
            tables: [
              {
                id: 't',
                name: 'Table',
                position: 1024,
                schemaVersion: 1,
                level: 'owner',
                views: [
                  { id: 'v', name: 'Grid', type: 'grid', scope: 'shared' },
                ],
              },
            ],
          },
        ],
      },
    ],
  }
}

export function tableSchema(): TableSchema {
  return {
    table: {
      id: 't',
      name: 'Table',
      bookId: 'b',
      displayFieldId: 'name',
      position: 1024,
      schemaVersion: 1,
      createdAt: '',
    },
    fields: [
      {
        id: 'name',
        name: 'Name',
        description: '',
        uiType: 'text',
        options: {},
        isSystem: false,
        isRequired: false,
        isUnique: false,
        default: null,
        position: 1024,
        version: 1,
      },
    ],
    views: [
      {
        id: 'v',
        name: 'Grid',
        type: 'grid',
        scope: 'shared',
        locked: false,
        position: 1024,
        version: 1,
        config: { fields: [], frozen: 0 },
      },
    ],
    relations: [],
    schemaVersion: 1,
    myLevel: 'owner',
  }
}
