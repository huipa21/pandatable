<script setup lang="ts">
// Field list and editor on the real table schema shell (D-130). Lazy-loaded;
// every mutation returns or reloads the authoritative schema.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { request, RequestError } from '../../api/client'
import type { FieldMutation, TableField, TableSchema } from '../../api/types'
import { useAuthStore } from '../../stores/auth'
import { useNavStore } from '../../stores/nav'
import { CHOICE_COLORS } from '../../fields/choice'
import { fieldKindLabel } from '../../fields/messages'
import { isKind, kindConstraints } from '../../fields/registry'
import type { FieldEnv } from '../../fields/types'
import {
  colorLabel,
  createBody,
  dateFormatLabel,
  displayEligible,
  draftFrom,
  MAX_DESCRIPTION,
  mutationMessage,
  newChoice,
  patchBody,
  sizeLabel,
  STORAGE_OPTIONS,
  timeFormatLabel,
  timezoneLabel,
  USER_KINDS,
  type FieldDraft,
} from './fieldForm'

const MAX_CHOICES = 500

const props = defineProps<{ schema: TableSchema }>()
const emit = defineEmits<{
  update: [schema: TableSchema]
  reload: []
}>()

const { t, locale } = useI18n()
const auth = useAuthStore()
const nav = useNavStore()
const env = computed<FieldEnv>(() => ({
  timeZone: auth.user?.effectiveTimezone,
  locale: locale.value,
}))

const owner = computed(() => props.schema.myLevel === 'owner')
const fields = computed(() =>
  [...props.schema.fields].sort((a, b) => a.position - b.position),
)
const userFields = computed(() => fields.value.filter((f) => !f.isSystem))

const busy = ref(false)
const error = ref<unknown>(null)
const status = ref('')
let controller: AbortController | undefined

const editor = ref<HTMLDialogElement>()
const remover = ref<HTMLDialogElement>()
/** The field being edited (null while creating); version is captured at open. */
const editing = ref<TableField | null>(null)
const creating = ref(false)
const draft = ref<FieldDraft>(draftFrom(null, env.value))
const conflict = ref(false)
const deleting = ref<TableField | null>(null)

const constraints = computed(() => kindConstraints(draft.value.kind))
const storageFixed = computed(() => editing.value !== null)
const systemEdit = computed(() => editing.value?.isSystem ?? false)
const sizes = computed(() =>
  draft.value.kind === 'float' ? ['real', 'double'] : ['small', 'int', 'big'],
)
const selectKind = computed(
  () =>
    draft.value.kind === 'single_select' || draft.value.kind === 'multi_select',
)
const showFill = computed(
  () =>
    !creating.value &&
    draft.value.isRequired &&
    !(editing.value?.isRequired ?? false),
)

function stable(field: TableField) {
  return props.schema.fields.find((item) => item.id === field.id) ?? null
}

const canSetDisplay = (field: TableField) =>
  displayEligible(field) && field.id !== props.schema.table.displayFieldId

async function mutate<T>(
  path: string,
  method: 'POST' | 'PATCH' | 'DELETE',
  body: unknown,
  version?: number,
): Promise<T | null> {
  if (busy.value) return null
  busy.value = true
  error.value = null
  status.value = ''
  controller?.abort()
  const own = new AbortController()
  controller = own
  try {
    return await request<T>(
      path,
      method,
      body,
      AbortSignal.any([own.signal, AbortSignal.timeout(10_000)]),
      version === undefined ? {} : { ifMatch: version },
    )
  } catch (cause) {
    if (own.signal.aborted && controller !== own) return null
    error.value = cause
    if (!(cause instanceof RequestError) || cause.status >= 500) {
      // The outcome is uncertain; reconcile with the authoritative schema.
      emit('reload')
    } else if (cause.code === 'ERR_VERSION_CONFLICT') {
      conflict.value = true
      emit('reload')
    } else if ([401, 403, 404].includes(cause.status)) {
      emit('reload')
      if (cause.status !== 401) void nav.refresh(true)
    }
    return null
  } finally {
    if (controller === own) {
      controller = undefined
      busy.value = false
    }
  }
}

async function openEditor(field: TableField | null) {
  error.value = null
  status.value = ''
  conflict.value = false
  editing.value = field
  creating.value = field === null
  draft.value = draftFrom(field, env.value)
  await nextTick()
  editor.value?.showModal()
}

function closeEditor() {
  if (busy.value) return
  editor.value?.close()
  editing.value = null
  creating.value = false
  error.value = null
}

// After a version conflict, reseed the open editor from the latest metadata.
watch(
  () => props.schema,
  () => {
    if (!conflict.value) return
    conflict.value = false
    if (!editing.value) return
    const latest = stable(editing.value)
    if (!latest) {
      editor.value?.close()
      editing.value = null
      return
    }
    editing.value = latest
    draft.value = draftFrom(latest, env.value)
  },
)

async function save() {
  if (busy.value) return
  const field = editing.value
  const body = field
    ? patchBody(field, draft.value, env.value)
    : createBody(draft.value, env.value)
  if (!body.ok) {
    error.value = body.message
    return
  }
  const result = field
    ? await mutate<FieldMutation>(
        `/fields/${field.id}`,
        'PATCH',
        body.value,
        field.version,
      )
    : await mutate<FieldMutation>(
        `/tables/${props.schema.table.id}/fields`,
        'POST',
        body.value,
      )
  if (!result) return
  emit('update', result.schema)
  editor.value?.close()
  editing.value = null
  creating.value = false
  status.value = t('field.panel.saved')
}

async function move(field: TableField, offset: number) {
  const index = userFields.value.findIndex((item) => item.id === field.id)
  if (index < 0) return
  const result = await mutate<FieldMutation>(
    `/fields/${field.id}`,
    'PATCH',
    { index: index + offset },
    field.version,
  )
  if (result) emit('update', result.schema)
}

async function setDisplay(field: TableField) {
  const result = await mutate<{ eventId: string }>(
    `/tables/${props.schema.table.id}`,
    'PATCH',
    { displayFieldId: field.id },
  )
  if (!result) return
  status.value = t('field.panel.displaySet')
  emit('reload')
}

async function askDelete(field: TableField) {
  error.value = null
  deleting.value = field
  await nextTick()
  remover.value?.showModal()
}

function closeDelete() {
  if (busy.value) return
  remover.value?.close()
  deleting.value = null
  error.value = null
}

async function confirmDelete() {
  const field = deleting.value
  if (!field) return
  const result = await mutate<FieldMutation>(
    `/fields/${field.id}`,
    'DELETE',
    undefined,
    field.version,
  )
  if (!result) return
  emit('update', result.schema)
  remover.value?.close()
  deleting.value = null
  status.value = t('field.panel.deleted')
}

function closeMenu(event: Event) {
  ;(event.currentTarget as HTMLElement | null)
    ?.closest('details')
    ?.removeAttribute('open')
}

function addChoice() {
  if (draft.value.choices.length < MAX_CHOICES)
    draft.value.choices.push(newChoice())
}

function removeChoice(index: number) {
  const [removed] = draft.value.choices.splice(index, 1)
  if (removed)
    draft.value.defaultChoices = draft.value.defaultChoices.filter(
      (id) => id !== removed.id,
    )
}

function toggleDefaultChoice(id: string, checked: boolean) {
  const others = draft.value.defaultChoices.filter((item) => item !== id)
  draft.value.defaultChoices = checked ? [...others, id] : others
}

function changeKind() {
  const kind = draft.value.kind
  const name = draft.value.name
  const description = draft.value.description
  draft.value = { ...draftFrom(null, env.value), kind, name, description }
  if (kind === 'float') draft.value.size = 'double'
}

const message = computed(() =>
  typeof error.value === 'string' ? error.value : mutationMessage(error.value),
)
const dialogOpen = computed(
  () => editing.value !== null || creating.value || deleting.value !== null,
)

onBeforeUnmount(() => {
  const own = controller
  controller = undefined
  own?.abort()
})
</script>

<template>
  <section class="field-panel" :aria-label="t('field.panel.title')">
    <div class="field-panel-head">
      <h2>{{ t('field.panel.title') }}</h2>
      <button
        v-if="owner"
        type="button"
        :disabled="busy"
        @click="openEditor(null)"
      >
        {{ t('field.panel.add') }}
      </button>
    </div>
    <p v-if="!owner">{{ t('field.panel.readOnly') }}</p>
    <p class="field-status" role="status">{{ status }}</p>
    <p v-if="error && !dialogOpen" role="alert">{{ message }}</p>
    <ul class="field-list">
      <li
        v-for="field in fields"
        :key="field.id"
        :data-field-id="field.id"
        :data-system="field.isSystem ? 'true' : undefined"
      >
        <div class="field-line">
          <span class="field-name">{{ field.name }}</span>
          <span class="field-kind">{{
            isKind(field.uiType) ? fieldKindLabel(field.uiType) : field.uiType
          }}</span>
          <span v-if="field.isSystem" class="field-badge">
            {{ t('field.panel.system') }}
          </span>
          <span
            v-if="field.id === schema.table.displayFieldId"
            class="field-badge"
          >
            {{ t('field.panel.display') }}
          </span>
          <span v-if="field.isRequired" class="field-badge">
            {{ t('field.panel.required') }}
          </span>
          <span v-if="field.isUnique" class="field-badge">
            {{ t('field.panel.unique') }}
          </span>
          <details v-if="owner" class="node-actions field-actions">
            <summary>
              {{ t('field.panel.actions', { name: field.name }) }}
            </summary>
            <div class="field-menu">
              <button
                type="button"
                :disabled="busy"
                @click="
                  (event) => {
                    closeMenu(event)
                    openEditor(field)
                  }
                "
              >
                {{ t('field.panel.edit') }}
              </button>
              <template v-if="!field.isSystem">
                <button
                  type="button"
                  :disabled="busy || userFields[0]?.id === field.id"
                  @click="
                    (event) => {
                      closeMenu(event)
                      move(field, -1)
                    }
                  "
                >
                  {{ t('field.panel.moveUp') }}
                </button>
                <button
                  type="button"
                  :disabled="busy || userFields.at(-1)?.id === field.id"
                  @click="
                    (event) => {
                      closeMenu(event)
                      move(field, 1)
                    }
                  "
                >
                  {{ t('field.panel.moveDown') }}
                </button>
                <button
                  v-if="canSetDisplay(field)"
                  type="button"
                  :disabled="busy"
                  @click="
                    (event) => {
                      closeMenu(event)
                      setDisplay(field)
                    }
                  "
                >
                  {{ t('field.panel.setDisplay') }}
                </button>
                <button
                  type="button"
                  :disabled="busy"
                  @click="
                    (event) => {
                      closeMenu(event)
                      askDelete(field)
                    }
                  "
                >
                  {{ t('field.panel.delete') }}
                </button>
              </template>
            </div>
          </details>
        </div>
        <p v-if="field.description" class="field-description">
          {{ field.description }}
        </p>
      </li>
    </ul>

    <dialog
      ref="editor"
      class="field-dialog"
      :aria-label="
        creating ? t('field.panel.createTitle') : t('field.panel.editTitle')
      "
      @cancel.prevent="closeEditor"
    >
      <form v-if="creating || editing" @submit.prevent="save">
        <h2>
          {{
            creating ? t('field.panel.createTitle') : t('field.panel.editTitle')
          }}
        </h2>
        <label>
          <span>{{ t('field.panel.name') }}</span>
          <input
            v-model="draft.name"
            autofocus
            maxlength="400"
            :disabled="busy"
          />
        </label>
        <label v-if="creating">
          <span>{{ t('field.panel.kind') }}</span>
          <select v-model="draft.kind" :disabled="busy" @change="changeKind">
            <option v-for="kind in USER_KINDS" :key="kind" :value="kind">
              {{ fieldKindLabel(kind) }}
            </option>
          </select>
        </label>
        <dl v-else class="field-kind-fixed">
          <dt>{{ t('field.panel.kind') }}</dt>
          <dd>{{ fieldKindLabel(draft.kind) }}</dd>
        </dl>
        <label>
          <span>{{ t('field.panel.description') }}</span>
          <textarea
            v-model="draft.description"
            rows="2"
            :maxlength="MAX_DESCRIPTION * 2"
            :disabled="busy"
          />
        </label>
        <p v-if="systemEdit">{{ t('field.panel.systemNotice') }}</p>
        <fieldset v-else class="field-settings">
          <legend>{{ t('field.panel.settings') }}</legend>
          <p v-if="storageFixed && STORAGE_OPTIONS[draft.kind]">
            {{ t('field.panel.storageFixed') }}
          </p>
          <label v-if="draft.kind === 'text'">
            <span>{{ t('field.panel.maxLength') }}</span>
            <input
              v-model="draft.maxLength"
              inputmode="numeric"
              :disabled="busy || storageFixed"
            />
          </label>
          <label v-if="draft.kind === 'integer' || draft.kind === 'float'">
            <span>{{ t('field.panel.size') }}</span>
            <select v-model="draft.size" :disabled="busy || storageFixed">
              <option v-for="size in sizes" :key="size" :value="size">
                {{ sizeLabel(size) }}
              </option>
            </select>
          </label>
          <template v-if="draft.kind === 'decimal'">
            <label>
              <span>{{ t('field.panel.precision') }}</span>
              <input
                v-model="draft.precision"
                inputmode="numeric"
                :disabled="busy || storageFixed"
              />
            </label>
            <label>
              <span>{{ t('field.panel.scale') }}</span>
              <input
                v-model="draft.scale"
                inputmode="numeric"
                :disabled="busy || storageFixed"
              />
            </label>
          </template>
          <label v-if="draft.kind === 'float'">
            <span>{{ t('field.panel.displayDecimals') }}</span>
            <input
              v-model="draft.displayDecimals"
              inputmode="numeric"
              :disabled="busy"
            />
          </label>
          <label
            v-if="['integer', 'decimal', 'float'].includes(draft.kind)"
            class="field-check"
          >
            <input
              v-model="draft.thousandSep"
              type="checkbox"
              :disabled="busy"
            />
            <span>{{ t('field.panel.thousandSep') }}</span>
          </label>
          <label v-if="draft.kind === 'date' || draft.kind === 'datetime'">
            <span>{{ t('field.panel.dateFormat') }}</span>
            <select v-model="draft.dateFormat" :disabled="busy">
              <option
                v-for="format in ['iso', 'slash', 'us', 'eu']"
                :key="format"
                :value="format"
              >
                {{ dateFormatLabel(format) }}
              </option>
            </select>
          </label>
          <template v-if="draft.kind === 'datetime'">
            <label>
              <span>{{ t('field.panel.timeFormat') }}</span>
              <select v-model="draft.timeFormat" :disabled="busy">
                <option
                  v-for="format in ['24h', '12h']"
                  :key="format"
                  :value="format"
                >
                  {{ timeFormatLabel(format) }}
                </option>
              </select>
            </label>
            <label>
              <span>{{ t('field.panel.timezone') }}</span>
              <select v-model="draft.timezone" :disabled="busy">
                <option
                  v-for="zone in ['user', 'fixed']"
                  :key="zone"
                  :value="zone"
                >
                  {{ timezoneLabel(zone) }}
                </option>
              </select>
            </label>
            <label v-if="draft.timezone === 'fixed'">
              <span>{{ t('field.panel.fixedTz') }}</span>
              <input v-model="draft.fixedTz" maxlength="64" :disabled="busy" />
            </label>
          </template>
          <fieldset v-if="selectKind" class="field-choices">
            <legend>{{ t('field.panel.choices') }}</legend>
            <div
              v-for="(choice, index) in draft.choices"
              :key="choice.id"
              class="field-choice"
            >
              <input
                v-model="choice.label"
                maxlength="400"
                :aria-label="t('field.panel.choiceLabel', { n: index + 1 })"
                :disabled="busy"
              />
              <select
                v-model="choice.color"
                :aria-label="t('field.panel.choiceColor', { n: index + 1 })"
                :disabled="busy"
              >
                <option
                  v-for="color in CHOICE_COLORS"
                  :key="color"
                  :value="color"
                >
                  {{ colorLabel(color) }}
                </option>
              </select>
              <button
                type="button"
                :aria-label="t('field.panel.removeChoice', { n: index + 1 })"
                :disabled="busy"
                @click="removeChoice(index)"
              >
                {{ t('field.panel.remove') }}
              </button>
            </div>
            <button
              type="button"
              :disabled="busy || draft.choices.length >= MAX_CHOICES"
              @click="addChoice"
            >
              {{ t('field.panel.addChoice') }}
            </button>
          </fieldset>
          <label v-if="constraints.required" class="field-check">
            <input
              v-model="draft.isRequired"
              type="checkbox"
              :disabled="busy"
            />
            <span>{{ t('field.panel.required') }}</span>
          </label>
          <label v-if="constraints.unique" class="field-check">
            <input v-model="draft.isUnique" type="checkbox" :disabled="busy" />
            <span>{{ t('field.panel.unique') }}</span>
          </label>
          <template v-if="constraints.default">
            <label v-if="draft.kind === 'checkbox'">
              <span>{{ t('field.panel.defaultValue') }}</span>
              <select v-model="draft.defaultBool" :disabled="busy">
                <option value="">{{ t('field.panel.noDefault') }}</option>
                <option value="true">{{ t('field.panel.checked') }}</option>
                <option value="false">{{ t('field.panel.unchecked') }}</option>
              </select>
            </label>
            <label v-else-if="draft.kind === 'single_select'">
              <span>{{ t('field.panel.defaultValue') }}</span>
              <select
                :value="draft.defaultChoices[0] ?? ''"
                :disabled="busy"
                @change="
                  (event) => {
                    const id = (event.target as HTMLSelectElement).value
                    draft.defaultChoices = id ? [id] : []
                  }
                "
              >
                <option value="">{{ t('field.panel.noDefault') }}</option>
                <option
                  v-for="choice in draft.choices"
                  :key="choice.id"
                  :value="choice.id"
                >
                  {{ choice.label }}
                </option>
              </select>
            </label>
            <fieldset
              v-else-if="draft.kind === 'multi_select'"
              class="field-choices"
            >
              <legend>{{ t('field.panel.defaultValue') }}</legend>
              <label
                v-for="choice in draft.choices"
                :key="choice.id"
                class="field-check"
              >
                <input
                  type="checkbox"
                  :checked="draft.defaultChoices.includes(choice.id)"
                  :disabled="busy"
                  @change="
                    (event) =>
                      toggleDefaultChoice(
                        choice.id,
                        (event.target as HTMLInputElement).checked,
                      )
                  "
                />
                <span>{{ choice.label }}</span>
              </label>
            </fieldset>
            <label v-else>
              <span>{{ t('field.panel.defaultValue') }}</span>
              <input
                v-model="draft.defaultText"
                maxlength="20000"
                :placeholder="
                  draft.kind === 'date'
                    ? t('field.editor.datePlaceholder')
                    : draft.kind === 'datetime'
                      ? t('field.editor.datetimePlaceholder')
                      : undefined
                "
                :disabled="busy"
              />
            </label>
          </template>
          <label v-if="showFill" class="field-check">
            <input v-model="draft.fillEmpty" type="checkbox" :disabled="busy" />
            <span>{{ t('field.panel.fillEmpty') }}</span>
          </label>
        </fieldset>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="busy">
            {{
              busy
                ? t('field.panel.saving')
                : creating
                  ? t('field.panel.create')
                  : t('field.panel.save')
            }}
          </button>
          <button type="button" :disabled="busy" @click="closeEditor">
            {{ t('field.panel.cancel') }}
          </button>
        </div>
      </form>
    </dialog>

    <dialog
      ref="remover"
      class="field-dialog"
      :aria-label="t('field.panel.confirm')"
      @cancel.prevent="closeDelete"
    >
      <form v-if="deleting" @submit.prevent="confirmDelete">
        <h2>{{ t('field.panel.confirm') }}</h2>
        <p>{{ t('field.panel.confirmDelete', { name: deleting.name }) }}</p>
        <p v-if="error" role="alert">{{ message }}</p>
        <div class="nav-actions">
          <button type="submit" :disabled="busy">
            {{ t('field.panel.confirm') }}
          </button>
          <button type="button" :disabled="busy" @click="closeDelete">
            {{ t('field.panel.cancel') }}
          </button>
        </div>
      </form>
    </dialog>
  </section>
</template>
