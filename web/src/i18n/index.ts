import { createI18n } from 'vue-i18n'

export type Locale = 'en' | 'zh-CN'

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
