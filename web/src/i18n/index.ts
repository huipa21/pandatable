import { createI18n } from 'vue-i18n'
import type { AccessLevel, ResourceKind, TrashKind } from '../api/types'

export const supportedLocales = ['en', 'zh-CN'] as const
export type Locale = (typeof supportedLocales)[number]

export const i18n = createI18n({
  legacy: false,
  locale: 'en',
  fallbackLocale: 'en',
  messages: {},
})

export async function loadLocale(locale: Locale) {
  if (i18n.global.availableLocales.includes(locale)) return
  const messages =
    locale === 'zh-CN'
      ? await import('./zh-CN.json')
      : await import('./en.json')
  i18n.global.setLocaleMessage(locale, messages.default)
}

export function createResourceLabel(kind: ResourceKind) {
  const { t } = i18n.global
  switch (kind) {
    case 'workspace':
      return t('nav.create.workspace')
    case 'book':
      return t('nav.create.book')
    case 'table':
      return t('nav.create.table')
  }
}

export function accessLevelLabel(level: AccessLevel) {
  const { t } = i18n.global
  switch (level) {
    case 'none':
      return t('nav.level.none')
    case 'read':
      return t('nav.level.read')
    case 'write':
      return t('nav.level.write')
    case 'owner':
      return t('nav.level.owner')
  }
}

export function trashKindLabel(kind: TrashKind) {
  const { t } = i18n.global
  switch (kind) {
    case 'workspace':
      return t('trash.kind.workspace')
    case 'book':
      return t('trash.kind.book')
    case 'table':
      return t('trash.kind.table')
    case 'field':
      return t('trash.kind.field')
    case 'view':
      return t('trash.kind.view')
  }
}
