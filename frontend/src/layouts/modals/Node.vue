<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="700"
    scrollable
  >
    <v-card
      class="rounded-lg"
      :loading="loading"
    >
      <v-card-title>{{ isNew ? $t('actions.add') : $t('actions.edit') }} {{ $t('objects.node') }}</v-card-title>
      <v-tabs
        v-model="tab"
        show-arrows
        density="compact"
        color="primary"
      >
        <v-tab
          v-for="tb in tabs"
          :key="tb.value"
          :value="tb.value"
        >
          {{ $t('node.tab.' + tb.value) }}
          <v-icon
            v-if="!tb.valid"
            icon="mdi-alert-circle"
            color="error"
            size="small"
            class="ms-1"
          />
        </v-tab>
      </v-tabs>
      <v-divider />
      <v-card-text>
        <v-window
          v-model="tab"
          :touch="false"
        >
          <v-window-item value="general">
            <v-container>
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-text-field
                    v-model.trim="node.name"
                    :label="$t('node.name')"
                    placeholder="tokyo-1"
                    :rules="[nameRule]"
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-switch
                    v-model="node.enable"
                    color="primary"
                    :label="$t('enable')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col cols="12">
                  <v-text-field
                    v-model.trim="node.baseUrl"
                    dir="ltr"
                    :label="$t('node.baseUrl')"
                    :hint="$t('node.baseUrlHint')"
                    persistent-hint
                    placeholder="https://1.2.3.4:2095"
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col
                  cols="12"
                  sm="5"
                >
                  <v-text-field
                    v-model.trim="node.webPath"
                    dir="ltr"
                    :label="$t('node.webPath')"
                    placeholder="/app/"
                    hide-details
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="7"
                >
                  <v-text-field
                    v-model="node.token"
                    type="password"
                    autocomplete="new-password"
                    dir="ltr"
                    :label="$t('node.token')"
                    :placeholder="!isNew && node.tokenSet ? $t('node.tokenKeep') : ''"
                    :hint="$t('node.tokenHint')"
                    persistent-hint
                  />
                </v-col>
              </v-row>
              <template v-if="isHttps">
                <v-row>
                  <v-col cols="12">
                    <v-switch
                      v-model="node.insecure"
                      color="warning"
                      :label="$t('node.insecure')"
                      hide-details
                    />
                  </v-col>
                </v-row>
                <v-row>
                  <v-col cols="12">
                    <v-text-field
                      v-model.trim="node.certPin"
                      dir="ltr"
                      :label="$t('node.certPin')"
                      :hint="$t('node.certPinHint')"
                      persistent-hint
                    />
                  </v-col>
                </v-row>
              </template>
              <v-alert
                v-else-if="node.baseUrl.startsWith('http://')"
                type="warning"
                variant="tonal"
                density="compact"
                class="mt-4"
              >
                {{ $t('node.httpWarn') }}
              </v-alert>
              <v-row>
                <v-col cols="12">
                  <v-combobox
                    v-model="node.tags"
                    :items="knownTags"
                    :label="$t('node.tags')"
                    :hint="$t('node.tagsHint')"
                    persistent-hint
                    multiple
                    chips
                    closable-chips
                    :rules="[tagsRule]"
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col
                  cols="6"
                  sm="4"
                >
                  <v-text-field
                    v-model.trim="node.country"
                    dir="ltr"
                    maxlength="2"
                    :label="$t('node.country')"
                    :hint="$t('node.countryHint')"
                    persistent-hint
                    :prefix="flagEmoji(node.country)"
                    :rules="[countryRule]"
                  />
                </v-col>
                <v-col
                  cols="6"
                  sm="4"
                >
                  <v-text-field
                    v-model.number="node.sortOrder"
                    type="number"
                    :label="$t('node.sortOrder')"
                    :hint="$t('node.sortOrderHint')"
                    persistent-hint
                  />
                </v-col>
              </v-row>
              <v-row>
                <v-col cols="12">
                  <v-text-field
                    v-model="node.desc"
                    :label="$t('node.desc')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-row align="center">
                <v-col cols="auto">
                  <v-btn
                    variant="tonal"
                    color="primary"
                    :loading="testing"
                    prepend-icon="mdi-connection"
                    :disabled="!node.baseUrl"
                    @click="testConnection"
                  >
                    {{ $t('node.test') }}
                  </v-btn>
                </v-col>
                <v-col v-if="testResult">
                  <v-chip
                    :color="statusColor(testResult.state)"
                    size="small"
                    label
                  >
                    {{ statusLabel(testResult.state) }}
                    <template v-if="testResult.state === 'online'">
                      · {{ testResult.latency }} ms · {{ nodeVersion(testResult) }} / {{ testResult.coreVersion }}
                    </template>
                  </v-chip>
                  <div
                    v-if="testResult.error"
                    class="text-error text-caption mt-1"
                  >
                    {{ testResult.error }}
                  </div>
                </v-col>
              </v-row>
            </v-container>
          </v-window-item>

          <v-window-item value="alerts">
            <v-container>
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                class="mb-4"
              >
                {{ $t('node.alert.hint') }}
              </v-alert>
              <v-row>
                <v-col
                  v-for="key in alertKeys"
                  :key="key"
                  cols="12"
                  sm="6"
                >
                  <v-text-field
                    v-model="alertForm[key]"
                    type="number"
                    min="0"
                    :max="alertMax[key]"
                    :label="$t('node.alert.' + key)"
                    :placeholder="$t('node.alert.default', { value: alertDefaults[key] || $t('node.alert.off') })"
                    persistent-placeholder
                    :rules="[alertRule(key)]"
                  />
                </v-col>
              </v-row>
              <v-switch
                v-model="versionAlert"
                color="primary"
                :label="$t('node.alert.version')"
                hide-details
              />
            </v-container>
          </v-window-item>

          <v-window-item value="cap">
            <v-container>
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                class="mb-4"
              >
                {{ $t('node.cap.hint') }}
              </v-alert>
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-text-field
                    v-model="capGb"
                    type="number"
                    min="0"
                    step="any"
                    :label="$t('node.cap.limit')"
                    :hint="$t('node.cap.limitHint')"
                    persistent-hint
                    suffix="GB"
                    :rules="[capRule]"
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-select
                    v-model="node.cap.day"
                    :items="days"
                    :label="$t('node.cap.day')"
                    :hint="$t('node.cap.dayHint')"
                    persistent-hint
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-select
                    v-model="node.cap.mode"
                    :items="capModes"
                    :label="$t('node.cap.counts')"
                    hide-details
                  />
                </v-col>
              </v-row>
              <v-switch
                v-model="node.cap.hide"
                color="warning"
                :label="$t('node.cap.hide')"
                hide-details
              />
            </v-container>
          </v-window-item>

          <v-window-item value="subscription">
            <v-container>
              <v-switch
                v-model="node.hideDown"
                color="warning"
                :label="$t('node.hideDown')"
                hide-details
              />
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                class="mt-2"
              >
                {{ $t('node.hideDownHint') }}
              </v-alert>
            </v-container>
          </v-window-item>

          <v-window-item value="access">
            <v-container>
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                class="mb-4"
              >
                {{ $t('node.access.hint') }}
              </v-alert>
              <v-combobox
                v-model="node.access.groups"
                :items="groups"
                :label="$t('node.access.groups')"
                multiple
                chips
                closable-chips
              />
              <v-autocomplete
                v-model="node.access.clients"
                :items="clientItems"
                :label="$t('node.access.clients')"
                multiple
                chips
                closable-chips
                clearable
              />
              <div class="text-caption text-medium-emphasis">
                {{ restricted ? $t('node.access.only') : $t('node.access.everyone') }}
              </div>
            </v-container>
          </v-window-item>
        </v-window>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          @click="$emit('close')"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          :disabled="!canSubmit"
          @click="save"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Data from '@/store/modules/data'
import HttpUtils from '@/plugins/httputil'
import {
  alertDefaults, alertMax, alertValue, bytesToGb, editableNode, flagEmoji, gbToBytes, nodePayload, nodeTags,
  nodeVersion, type AlertKey, type EditableNode, type Node, type NodeStatus,
} from '@/types/node'
import { i18n } from '@/locales'

const props = defineProps<{ visible: boolean; item?: Node | null }>()
const emit = defineEmits<{ close: [] }>()
const store = Data()
const node = ref<EditableNode>(editableNode(null))
const loading = ref(false)
const testing = ref(false)
const testResult = ref<NodeStatus | null>(null)
const tab = ref('general')
const alertKeys: AlertKey[] = ['cpu', 'mem', 'disk', 'ping', 'certDays']
const alertForm = ref<Record<AlertKey, string>>({ cpu: '', mem: '', disk: '', ping: '', certDays: '' })
const versionAlert = ref(true)
const capGb = ref<string | number>('')
const isNew = computed(() => !node.value.id)
const isHttps = computed(() => node.value.baseUrl.toLowerCase().startsWith('https://'))

watch(() => props.visible, (visible) => {
  if (!visible) return
  node.value = editableNode(props.item)
  const a = node.value.alerts ?? {}
  alertForm.value = {
    cpu: a.cpu == null ? '' : String(a.cpu),
    mem: a.mem == null ? '' : String(a.mem),
    disk: a.disk == null ? '' : String(a.disk),
    ping: a.ping == null ? '' : String(a.ping),
    certDays: a.certDays == null ? '' : String(a.certDays),
  }
  versionAlert.value = a.version !== false
  capGb.value = node.value.cap?.limit ? bytesToGb(node.value.cap.limit) : ''
  testResult.value = null
  tab.value = 'general'
})

const t = (key: string, values?: Record<string, unknown>) => (values ? i18n.global.t(key, values) : i18n.global.t(key))
const nameRule = (v: string) => !/[[\]]/.test(v ?? '') || t('node.nameRule')
const countryRule = (v: string) => !v || /^[A-Za-z]{2}$/.test(v) || t('node.countryHint')
const tagsRule = (v: string[]) => {
  const list = (v ?? []).map(x => String(x).trim()).filter(Boolean)
  if (list.length > 10) return t('node.tagsMax', { n: 10 })
  if (list.some(x => [...x].length > 24)) return t('node.tagLong', { n: 24 })
  return true
}
const alertRule = (key: AlertKey) => (v: string | number) => {
  if (v === '' || v == null) return true
  const n = Number(v)
  return (Number.isInteger(n) && n >= 0 && n <= alertMax[key]) || t('node.alert.range', { max: alertMax[key] })
}
const capRule = (v: string | number) => v === '' || v == null || (Number(v) >= 0 && Number.isFinite(Number(v))) || t('node.cap.limitHint')
const ok = (r: true | string) => r === true

const generalValid = computed(() => ok(nameRule(node.value.name)) && ok(countryRule(node.value.country ?? '')) && ok(tagsRule(node.value.tags ?? [])))
const alertsValid = computed(() => alertKeys.every(k => ok(alertRule(k)(alertForm.value[k]))))
const capValid = computed(() => ok(capRule(capGb.value)))
const tabs = computed(() => [
  { value: 'general', valid: generalValid.value },
  { value: 'alerts', valid: alertsValid.value },
  { value: 'cap', valid: capValid.value },
  { value: 'subscription', valid: true },
  { value: 'access', valid: true },
])
const canSubmit = computed(() => !!node.value.name.trim() && !!node.value.baseUrl.trim() && (!isNew.value || !!node.value.token) &&
  generalValid.value && alertsValid.value && capValid.value)

const knownTags = computed(() => nodeTags(store.nodes))
const days = Array.from({ length: 31 }, (_, i) => i + 1)
const capModes = computed(() => (['total', 'up', 'down'] as const).map(m => ({ title: t('node.cap.mode.' + m), value: m })))
const groups = computed(() => {
  const all = new Set<string>()
  for (const c of store.clients) {
    const g = (c.group ?? '').trim()
    if (g && g !== '@cluster') all.add(g)
  }
  for (const g of node.value.access?.groups ?? []) all.add(g)
  return [...all].sort((a, b) => a.localeCompare(b))
})
const clientItems = computed(() => store.clients.map(c => ({ title: c.name, value: c.id })))
const restricted = computed(() => (node.value.access?.groups?.length ?? 0) + (node.value.access?.clients?.length ?? 0) > 0)

const statusColor = (state: string) => state === 'online' ? 'success' : state === 'core-stopped' ? 'warning' : 'error'
const statusLabel = (state: string) => t(`node.status.${state === 'core-stopped' ? 'coreStopped' : state}`)

const payload = (): Node => {
  const n = node.value
  return nodePayload({
    ...n,
    country: (n.country ?? '').trim().toUpperCase(),
    tags: [...new Set((n.tags ?? []).map(x => String(x).trim()).filter(Boolean))],
    sortOrder: Number.isFinite(Number(n.sortOrder)) ? Math.trunc(Number(n.sortOrder)) : 0,
    alerts: {
      cpu: alertValue(alertForm.value.cpu),
      mem: alertValue(alertForm.value.mem),
      disk: alertValue(alertForm.value.disk),
      ping: alertValue(alertForm.value.ping),
      certDays: alertValue(alertForm.value.certDays),
      version: versionAlert.value,
    },
    cap: { limit: gbToBytes(Number(capGb.value) || 0), day: n.cap?.day || 1, mode: n.cap?.mode || 'total', hide: !!n.cap?.hide },
    access: {
      groups: [...new Set((n.access?.groups ?? []).map(x => String(x).trim()).filter(Boolean))],
      clients: [...new Set(n.access?.clients ?? [])],
    },
  })
}

const testConnection = async () => {
  testing.value = true
  testResult.value = null
  const msg = await HttpUtils.post<NodeStatus>('api/testNode', { data: JSON.stringify(payload()) })
  testing.value = false
  if (msg.success) testResult.value = msg.obj
}

const save = async () => {
  loading.value = true
  const success = await store.save('nodes', isNew.value ? 'new' : 'edit', payload())
  loading.value = false
  if (success) emit('close')
}
</script>
