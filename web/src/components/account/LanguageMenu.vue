<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '../AppIcon.vue'
import { supportedLocales, type Locale } from '../../i18n'
import { usePreferencesStore } from '../../stores/preferences'

const { t } = useI18n()
const preferences = usePreferencesStore()
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const menu = ref<HTMLElement>()
const menuItems = ref<HTMLButtonElement[]>([])
const opened = ref(false)

function nativeLocaleName(locale: Locale) {
  switch (locale) {
    case 'en':
      return t('common.nativeLanguages.en')
    case 'zh-CN':
      return t('common.nativeLanguages.zhCN')
  }
}

function localizedLocaleName(locale: Locale) {
  switch (locale) {
    case 'en':
      return t('common.languages.en')
    case 'zh-CN':
      return t('common.languages.zhCN')
  }
}

function focusCurrentLocale() {
  const index = supportedLocales.indexOf(preferences.locale)
  menuItems.value[index]?.focus()
}

function closeMenu() {
  if (!opened.value) return
  opened.value = false
  void nextTick(() => trigger.value?.focus())
}

function handleOutsidePointerDown(event: PointerEvent) {
  if (!root.value?.contains(event.target as Node)) closeMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (!opened.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeMenu()
    return
  }
  if (!menu.value?.contains(event.target as Node)) return

  const activeIndex = menuItems.value.indexOf(
    document.activeElement as HTMLButtonElement,
  )
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const direction = event.key === 'ArrowDown' ? 1 : -1
    const nextIndex =
      (activeIndex + direction + supportedLocales.length) %
      supportedLocales.length
    menuItems.value[nextIndex]?.focus()
  }
}

async function selectLocale(locale: Locale) {
  closeMenu()
  await preferences.setLocale(locale)
}

watch(opened, (isOpen) => {
  if (isOpen) {
    document.addEventListener('pointerdown', handleOutsidePointerDown, true)
    document.addEventListener('keydown', handleKeydown, true)
    void nextTick(focusCurrentLocale)
  } else {
    document.removeEventListener('pointerdown', handleOutsidePointerDown, true)
    document.removeEventListener('keydown', handleKeydown, true)
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handleOutsidePointerDown, true)
  document.removeEventListener('keydown', handleKeydown, true)
})
</script>

<template>
  <div ref="root" class="language-menu">
    <button
      ref="trigger"
      class="icon-button language-menu-trigger"
      type="button"
      :aria-label="t('common.language')"
      :title="t('common.language')"
      aria-haspopup="menu"
      :aria-expanded="opened"
      @click="opened = !opened"
    >
      <AppIcon name="language" />
    </button>
    <div
      v-if="opened"
      ref="menu"
      class="language-menu-popup"
      role="menu"
      :aria-label="t('common.language')"
    >
      <button
        v-for="locale in supportedLocales"
        :key="locale"
        ref="menuItems"
        class="language-menu-item"
        type="button"
        role="menuitemradio"
        :aria-checked="preferences.locale === locale"
        @click="selectLocale(locale)"
      >
        <span class="language-menu-names">
          <span class="language-menu-native" :lang="locale">{{
            nativeLocaleName(locale)
          }}</span>
          <span :lang="preferences.locale">{{
            localizedLocaleName(locale)
          }}</span>
        </span>
        <span class="language-menu-check" aria-hidden="true">
          <AppIcon
            v-if="preferences.locale === locale"
            name="checkMark"
            :size="14"
          />
        </span>
      </button>
    </div>
  </div>
</template>
