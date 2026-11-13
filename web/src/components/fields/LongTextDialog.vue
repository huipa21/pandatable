<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { codePoints } from '../../fields/common'
import AppIcon, { type IconName } from '../AppIcon.vue'
import { applyCommand, isActive, type MdCommand } from './markdownCommands'

const props = withDefaults(
  defineProps<{
    open: boolean
    value: string
    fieldName: string
    readOnly?: boolean
    maxLength?: number
  }>(),
  {
    readOnly: false,
    maxLength: 100000,
  },
)
const emit = defineEmits<{
  apply: [text: string]
  cancel: []
}>()
const { t } = useI18n()
const host = ref<HTMLElement>()
const dialog = ref<HTMLDialogElement>()
const textarea = ref<HTMLTextAreaElement>()
const portalDestination = ref<HTMLElement | string>('body')
const draft = ref('')
const confirmDiscard = ref(false)
const selection = ref({ start: 0, end: 0 })
let initialValue = ''
let opener: HTMLElement | null = null
let pointerStartedOnBackdrop = false

const toolbarGroups: {
  commands: { command: MdCommand; icon: IconName; label: CommandLabel }[]
}[] = [
  {
    commands: [
      { command: 'bold', icon: 'bold', label: 'bold' },
      { command: 'italic', icon: 'italic', label: 'italic' },
      { command: 'underline', icon: 'underline', label: 'underline' },
      { command: 'strike', icon: 'strikethrough', label: 'strike' },
      { command: 'code', icon: 'code', label: 'code' },
    ],
  },
  {
    commands: [
      { command: 'h1', icon: 'heading-1', label: 'heading1' },
      { command: 'h2', icon: 'heading-2', label: 'heading2' },
      { command: 'h3', icon: 'heading-3', label: 'heading3' },
    ],
  },
  {
    commands: [
      { command: 'quote', icon: 'quote', label: 'quote' },
      { command: 'bullet', icon: 'list-bullet', label: 'bullet' },
      { command: 'ordered', icon: 'list-ordered', label: 'ordered' },
      { command: 'check', icon: 'list-check', label: 'check' },
    ],
  },
  {
    commands: [
      { command: 'link', icon: 'link', label: 'link' },
      { command: 'image', icon: 'image', label: 'image' },
    ],
  },
]
type CommandLabel =
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'code'
  | 'heading1'
  | 'heading2'
  | 'heading3'
  | 'quote'
  | 'bullet'
  | 'ordered'
  | 'check'
  | 'link'
  | 'image'

const maximum = computed(() =>
  Number.isFinite(props.maxLength)
    ? Math.max(0, Math.trunc(props.maxLength))
    : 100000,
)
const characterCount = computed(() => codePoints(draft.value))
const showCounter = computed(() => characterCount.value > maximum.value * 0.9)
const dirty = computed(() => draft.value !== initialValue)
const shortcutModifier = computed(() =>
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
    ? '⌘'
    : 'Ctrl+',
)

function commandLabel(label: CommandLabel, command: MdCommand) {
  const shortcut =
    command === 'bold'
      ? `${shortcutModifier.value}B`
      : command === 'italic'
        ? `${shortcutModifier.value}I`
        : command === 'underline'
          ? `${shortcutModifier.value}U`
          : ''
  const name =
    label === 'bold'
      ? t('cellEditors.ces04.commands.bold')
      : label === 'italic'
        ? t('cellEditors.ces04.commands.italic')
        : label === 'underline'
          ? t('cellEditors.ces04.commands.underline')
          : label === 'strike'
            ? t('cellEditors.ces04.commands.strike')
            : label === 'code'
              ? t('cellEditors.ces04.commands.code')
              : label === 'heading1'
                ? t('cellEditors.ces04.commands.heading1')
                : label === 'heading2'
                  ? t('cellEditors.ces04.commands.heading2')
                  : label === 'heading3'
                    ? t('cellEditors.ces04.commands.heading3')
                    : label === 'quote'
                      ? t('cellEditors.ces04.commands.quote')
                      : label === 'bullet'
                        ? t('cellEditors.ces04.commands.bullet')
                        : label === 'ordered'
                          ? t('cellEditors.ces04.commands.ordered')
                          : label === 'check'
                            ? t('cellEditors.ces04.commands.check')
                            : label === 'link'
                              ? t('cellEditors.ces04.commands.link')
                              : t('cellEditors.ces04.commands.image')
  return shortcut
    ? t('cellEditors.ces04.commandWithShortcut', { label: name, shortcut })
    : name
}

function syncSelection() {
  const input = textarea.value
  if (!input) return
  selection.value = {
    start: input.selectionStart,
    end: input.selectionEnd,
  }
}

function isCommandActive(command: MdCommand) {
  return isActive(command, {
    value: draft.value,
    ...selection.value,
  })
}

function portalTarget() {
  const openDialog = host.value?.closest('dialog[open]')
  if (openDialog instanceof HTMLElement) return openDialog
  const fullscreen = document.fullscreenElement
  return fullscreen instanceof HTMLElement ? fullscreen : document.body
}

async function updatePortalTarget() {
  portalDestination.value = portalTarget()
  await nextTick()
}

function restoreFocus() {
  if (opener?.isConnected) opener.focus({ preventScroll: true })
  opener = null
}

function seedAndOpen() {
  const element = dialog.value
  if (!element || element.open) return
  opener =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null
  initialValue = Array.from(props.value.replace(/\r\n?/g, '\n'))
    .slice(0, maximum.value)
    .join('')
  draft.value = initialValue
  confirmDiscard.value = false
  pointerStartedOnBackdrop = false
  selection.value = { start: 0, end: 0 }
  element.showModal()
  void nextTick(() => textarea.value?.focus({ preventScroll: true }))
}

function closeNativeDialog() {
  pointerStartedOnBackdrop = false
  if (dialog.value?.open) dialog.value.close()
  confirmDiscard.value = false
  restoreFocus()
}

function apply() {
  const text = draft.value
  closeNativeDialog()
  emit('apply', text)
}

function cancel() {
  closeNativeDialog()
  emit('cancel')
}

function handleEscape() {
  if (dirty.value) confirmDiscard.value = true
  else cancel()
}

function onKeydown(event: KeyboardEvent) {
  const commandKey = event.ctrlKey || event.metaKey
  if (commandKey && event.key === 'Enter') {
    event.preventDefault()
    apply()
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    handleEscape()
    return
  }
  if (!commandKey || props.readOnly) return
  const command =
    event.key.toLowerCase() === 'b'
      ? 'bold'
      : event.key.toLowerCase() === 'i'
        ? 'italic'
        : event.key.toLowerCase() === 'u'
          ? 'underline'
          : null
  if (command) {
    event.preventDefault()
    runCommand(command)
  }
}

function onInput(event: Event) {
  const input = event.target as HTMLTextAreaElement
  const normalized = input.value.replace(/\r\n?/g, '\n')
  const value = Array.from(normalized).slice(0, maximum.value).join('')
  if (input.value !== value) {
    const caret = Math.min(input.selectionStart, value.length)
    input.value = value
    input.setSelectionRange(caret, caret)
  }
  draft.value = value
  syncSelection()
}

function runCommand(command: MdCommand) {
  const input = textarea.value
  if (!input || props.readOnly) return
  const before = draft.value
  const result = applyCommand(
    command,
    {
      value: before,
      start: input.selectionStart,
      end: input.selectionEnd,
    },
    {
      text: t('cellEditors.ces04.placeholderText'),
      url: t('cellEditors.ces04.placeholderUrl'),
    },
  )
  if (result.value === before || codePoints(result.value) > maximum.value) {
    input.focus({ preventScroll: true })
    syncSelection()
    return
  }

  let prefix = 0
  while (
    prefix < before.length &&
    prefix < result.value.length &&
    before[prefix] === result.value[prefix]
  )
    prefix++
  let suffix = 0
  while (
    suffix < before.length - prefix &&
    suffix < result.value.length - prefix &&
    before[before.length - 1 - suffix] ===
      result.value[result.value.length - 1 - suffix]
  )
    suffix++
  if (
    prefix > 0 &&
    /[\uD800-\uDBFF]/.test(before[prefix - 1] ?? '') &&
    /[\uDC00-\uDFFF]/.test(before[prefix] ?? '')
  )
    prefix--
  if (
    suffix > 0 &&
    /[\uD800-\uDBFF]/.test(before[before.length - suffix - 1] ?? '') &&
    /[\uDC00-\uDFFF]/.test(before[before.length - suffix] ?? '')
  )
    suffix--
  const oldEnd = before.length - suffix
  const replacement = result.value.slice(prefix, result.value.length - suffix)

  input.focus({ preventScroll: true })
  input.setSelectionRange(prefix, oldEnd)
  const usedNativeUndo =
    typeof document.execCommand === 'function' &&
    document.execCommand('insertText', false, replacement)
  if (!usedNativeUndo) {
    input.setRangeText(replacement, prefix, oldEnd, 'preserve')
  }
  if (input.value !== result.value) input.value = result.value
  input.setSelectionRange(result.start, result.end)
  draft.value = input.value
  syncSelection()
}

function isBackdrop(event: MouseEvent) {
  const element = dialog.value
  if (!element || event.target !== element) return false
  const bounds = element.getBoundingClientRect()
  return (
    event.clientX < bounds.left ||
    event.clientX >= bounds.right ||
    event.clientY < bounds.top ||
    event.clientY >= bounds.bottom
  )
}

function onDialogPointerDown(event: PointerEvent) {
  pointerStartedOnBackdrop = isBackdrop(event)
}

function onDialogClick(event: MouseEvent) {
  // Selection drags can retarget clicks to the dialog, just like its backdrop.
  const clickedBackdrop = pointerStartedOnBackdrop && isBackdrop(event)
  pointerStartedOnBackdrop = false
  if (clickedBackdrop) apply()
}

watch(
  () => props.open,
  async (open) => {
    await nextTick()
    if (open) {
      await updatePortalTarget()
      seedAndOpen()
    } else if (dialog.value?.open) closeNativeDialog()
  },
)

onMounted(async () => {
  if (props.open) {
    await updatePortalTarget()
    seedAndOpen()
  } else {
    portalDestination.value = portalTarget()
  }
})

onBeforeUnmount(() => {
  if (dialog.value?.open) dialog.value.close()
  restoreFocus()
})

function contains(node: Node | null) {
  return (
    !!node && (!!host.value?.contains(node) || !!dialog.value?.contains(node))
  )
}

function focus() {
  textarea.value?.focus({ preventScroll: true })
}

defineExpose({ contains, focus })
</script>

<template>
  <span ref="host" class="long-text-dialog-host"></span>
  <Teleport :to="portalDestination">
    <dialog
      ref="dialog"
      class="long-text-dialog"
      :aria-label="
        t('cellEditors.ces04.dialogLabel', { fieldName: props.fieldName })
      "
      @pointerdown="onDialogPointerDown"
      @pointercancel="pointerStartedOnBackdrop = false"
      @click="onDialogClick"
      @keydown.stop="onKeydown"
      @cancel.prevent="handleEscape"
    >
      <header class="long-text-dialog__header">
        <div class="long-text-dialog__field">
          <AppIcon name="long-text" />
          <h2>{{ props.fieldName }}</h2>
        </div>
        <div
          v-if="!props.readOnly"
          class="long-text-dialog__toolbar"
          role="toolbar"
          :aria-label="t('cellEditors.ces04.toolbar')"
        >
          <div
            v-for="(group, index) in toolbarGroups"
            :key="index"
            class="long-text-dialog__group"
          >
            <button
              v-for="item in group.commands"
              :key="item.command"
              type="button"
              :aria-label="commandLabel(item.label, item.command)"
              :title="commandLabel(item.label, item.command)"
              :aria-pressed="isCommandActive(item.command)"
              @mousedown.prevent
              @click="runCommand(item.command)"
            >
              <AppIcon :name="item.icon" />
              <span v-if="item.command === 'link'">
                {{ t('cellEditors.ces04.commands.link') }}
              </span>
            </button>
          </div>
        </div>
        <button
          type="button"
          class="long-text-dialog__close"
          :aria-label="t('cellEditors.ces04.close')"
          :title="t('cellEditors.ces04.close')"
          @click="apply"
        >
          <AppIcon name="close" />
        </button>
      </header>
      <div v-if="confirmDiscard" class="long-text-dialog__confirm" role="alert">
        <span>{{ t('cellEditors.ces04.discardPrompt') }}</span>
        <button type="button" @click="cancel">
          {{ t('cellEditors.ces04.discard') }}
        </button>
        <button type="button" @click="confirmDiscard = false">
          {{ t('cellEditors.ces04.keep') }}
        </button>
      </div>
      <textarea
        ref="textarea"
        class="long-text-dialog__textarea"
        :aria-label="
          t('cellEditors.ces04.textareaLabel', { fieldName: props.fieldName })
        "
        :value="draft"
        :readonly="props.readOnly"
        @input="onInput"
        @select="syncSelection"
        @keyup="syncSelection"
        @mouseup="syncSelection"
      ></textarea>
      <div v-if="showCounter" class="long-text-dialog__counter" role="status">
        {{
          t('cellEditors.ces04.counter', {
            count: characterCount,
            max: maximum,
          })
        }}
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.long-text-dialog {
  box-sizing: border-box;
  flex-direction: column;
  width: min(1180px, 94vw);
  height: min(760px, 90vh);
  max-width: 94vw;
  max-height: 90vh;
  padding: 0;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  box-shadow: 0 16px 48px #0003;
}
.long-text-dialog[open] {
  display: flex;
}
.long-text-dialog::backdrop {
  background: #0008;
}
.long-text-dialog__header {
  box-sizing: border-box;
  display: flex;
  flex: 0 0 44px;
  align-items: center;
  gap: 8px;
  min-width: 0;
  height: 44px;
  padding: 0 10px 0 14px;
  border-bottom: 1px solid var(--border);
}
.long-text-dialog__field {
  display: flex;
  flex: 1 1 auto;
  align-items: center;
  gap: 8px;
  min-width: 100px;
}
.long-text-dialog__field h2 {
  overflow: hidden;
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.long-text-dialog__toolbar,
.long-text-dialog__group {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 2px;
}
.long-text-dialog__group {
  padding-inline: 5px;
  border-inline-start: 1px solid var(--border);
}
.long-text-dialog__group:first-child {
  border-inline-start: 0;
}
.long-text-dialog button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 30px;
  height: 30px;
  padding: 4px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  white-space: nowrap;
}
.long-text-dialog button:hover,
.long-text-dialog button[aria-pressed='true'] {
  border-color: var(--border);
  background: color-mix(in srgb, var(--text) 8%, transparent);
}
.long-text-dialog button:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 1px;
}
.long-text-dialog__close {
  flex: 0 0 auto;
  margin-inline-start: 4px;
}
.long-text-dialog__confirm {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
}
.long-text-dialog__confirm span {
  margin-inline-end: auto;
}
.long-text-dialog__textarea {
  box-sizing: border-box;
  flex: 1 1 auto;
  min-height: 0;
  margin: 12px;
  padding: 12px;
  resize: none;
  border: 1px solid var(--border);
  border-radius: 4px;
  background: var(--surface);
  color: var(--text);
  font: inherit;
  line-height: 1.55;
}
.long-text-dialog__textarea:focus {
  border-color: var(--accent);
  outline: 1px solid var(--accent);
  outline-offset: 1px;
}
.long-text-dialog__counter {
  flex: 0 0 auto;
  padding: 0 14px 10px;
  color: color-mix(in srgb, var(--text) 65%, transparent);
  font-size: 12px;
  text-align: end;
}
@media (max-width: 900px) {
  .long-text-dialog__header {
    gap: 4px;
    padding-inline: 8px;
  }
  .long-text-dialog__field {
    flex: 0 1 auto;
    min-width: 48px;
  }
  .long-text-dialog__toolbar {
    flex: 1 1 auto;
    min-width: 0;
    gap: 0;
    overflow-x: auto;
  }
  .long-text-dialog__group {
    padding-inline: 2px;
  }
  .long-text-dialog button {
    min-width: 28px;
  }
}
</style>
