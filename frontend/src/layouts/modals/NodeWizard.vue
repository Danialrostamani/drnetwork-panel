<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="760"
    scrollable
    persistent
  >
    <v-card class="rounded-lg">
      <v-card-title class="d-flex align-center">
        {{ $t('node.wizard.title') }}
        <v-spacer />
        <span
          v-if="step > 1"
          class="text-caption text-medium-emphasis"
        >{{ $t('node.wizard.step', { n: step - 1, total: 3 }) }}</span>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <div ref="copyHost">
          <v-window
            v-model="step"
            :touch="false"
          >
            <v-window-item :value="1">
              <p class="mb-4">
                {{ $t('node.wizard.intro') }}
              </p>
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-card
                    variant="outlined"
                    rounded="lg"
                    class="pa-4 h-100"
                    @click="startNew"
                  >
                    <v-icon
                      icon="mdi-server-plus"
                      size="large"
                      color="primary"
                    />
                    <div class="font-weight-bold mt-2">
                      {{ $t('node.wizard.newServer') }}
                    </div>
                    <div class="text-medium-emphasis text-caption mt-1">
                      {{ $t('node.wizard.newServerDesc') }}
                    </div>
                  </v-card>
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-card
                    variant="outlined"
                    rounded="lg"
                    class="pa-4 h-100"
                    @click="emit('manual')"
                  >
                    <v-icon
                      icon="mdi-link-variant"
                      size="large"
                      color="primary"
                    />
                    <div class="font-weight-bold mt-2">
                      {{ $t('node.wizard.existing') }}
                    </div>
                    <div class="text-medium-emphasis text-caption mt-1">
                      {{ $t('node.wizard.existingDesc') }}
                    </div>
                  </v-card>
                </v-col>
              </v-row>
            </v-window-item>

            <v-window-item :value="2">
              <v-row>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-text-field
                    v-model.trim="form.name"
                    :label="$t('node.name')"
                    placeholder="tokyo-1"
                    :rules="[nameRule]"
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-text-field
                    v-model.trim="form.host"
                    dir="ltr"
                    :label="$t('node.wizard.host')"
                    :hint="$t('node.wizard.hostHint')"
                    persistent-hint
                    placeholder="203.0.113.10"
                    :rules="[hostRule]"
                  />
                </v-col>
                <v-col
                  cols="6"
                  sm="3"
                >
                  <v-text-field
                    v-model.number="form.port"
                    type="number"
                    dir="ltr"
                    :label="$t('node.wizard.port')"
                    :rules="[portRule]"
                  />
                </v-col>
                <v-col
                  cols="6"
                  sm="3"
                >
                  <v-text-field
                    v-model.trim="form.path"
                    dir="ltr"
                    :label="$t('node.webPath')"
                    :rules="[pathRule]"
                  />
                </v-col>
                <v-col
                  cols="12"
                  sm="6"
                >
                  <v-text-field
                    v-model.trim="form.token"
                    dir="ltr"
                    readonly
                    :label="$t('node.token')"
                    :hint="$t('node.wizard.tokenHint')"
                    persistent-hint
                  >
                    <template #append-inner>
                      <v-icon
                        icon="mdi-refresh"
                        :title="$t('actions.generate')"
                        @click="form.token = randomToken(32)"
                      />
                    </template>
                  </v-text-field>
                </v-col>
              </v-row>
              <div class="text-subtitle-2 mt-4 mb-1">
                {{ $t('node.wizard.command') }}
              </div>
              <v-card
                color="background"
                dir="ltr"
                class="d-flex align-start"
              >
                <pre class="node-command flex-grow-1">{{ command || '…' }}</pre>
                <v-btn
                  icon="mdi-content-copy"
                  variant="text"
                  :disabled="!command"
                  :title="$t('copyToClipboard')"
                  @click="copy"
                />
              </v-card>
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
                class="mt-3"
              >
                {{ $t('node.wizard.runHint') }}
              </v-alert>
              <v-alert
                type="warning"
                variant="tonal"
                density="compact"
                class="mt-2"
                icon="mdi-shield-lock-outline"
              >
                {{ $t('node.wizard.noSsh') }}
              </v-alert>
            </v-window-item>

            <v-window-item :value="3">
              <p class="mb-2">
                {{ $t('node.wizard.waitHint') }}
              </p>
              <div
                class="mb-3"
                dir="ltr"
              >
                <code>{{ draft.baseUrl }}{{ draft.webPath }}</code>
              </div>
              <div class="d-flex align-center flex-wrap ga-2">
                <v-btn
                  color="primary"
                  variant="tonal"
                  prepend-icon="mdi-connection"
                  :loading="testing && !waiting"
                  :disabled="waiting"
                  @click="test"
                >
                  {{ $t('node.test') }}
                </v-btn>
                <v-btn
                  :color="waiting ? 'warning' : 'primary'"
                  variant="outlined"
                  :prepend-icon="waiting ? 'mdi-stop' : 'mdi-timer-sand'"
                  @click="waiting ? stopWaiting() : startWaiting()"
                >
                  {{ waiting ? $t('node.wizard.stopWaiting') : $t('node.wizard.wait') }}
                </v-btn>
                <v-progress-circular
                  v-if="waiting"
                  indeterminate
                  size="20"
                  width="2"
                />
                <span
                  v-if="waiting"
                  class="text-caption"
                >{{ $t('node.wizard.attempt', { n: attempts }) }}</span>
              </div>
              <v-alert
                v-if="result"
                :type="answered ? 'success' : 'error'"
                variant="tonal"
                density="compact"
                class="mt-3"
              >
                <template v-if="answered">
                  {{ $t('node.wizard.answered') }} · {{ result.latency }} ms · {{ result.appFull || result.appVersion }} / {{ result.coreVersion }}
                </template>
                <template v-else>
                  {{ result.error || $t('node.status.offline') }}
                </template>
              </v-alert>
              <v-alert
                v-if="draft.baseUrl.startsWith('http://')"
                type="warning"
                variant="tonal"
                density="compact"
                class="mt-3"
              >
                {{ $t('node.wizard.httpNote') }}
              </v-alert>
            </v-window-item>

            <v-window-item :value="4">
              <v-alert
                type="success"
                variant="tonal"
                :title="$t('node.wizard.added', { name: draft.name })"
                :text="$t('node.wizard.importHint')"
              />
            </v-window-item>
          </v-window>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-btn
          v-if="step === 2 || step === 3"
          variant="text"
          prepend-icon="mdi-chevron-left"
          @click="back"
        >
          {{ $t('node.wizard.back') }}
        </v-btn>
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          @click="close"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          v-if="step === 2"
          color="primary"
          variant="tonal"
          append-icon="mdi-chevron-right"
          :disabled="!formValid"
          @click="toConnect"
        >
          {{ $t('node.wizard.next') }}
        </v-btn>
        <v-btn
          v-if="step === 3"
          :color="answered ? 'primary' : 'warning'"
          variant="tonal"
          :loading="saving"
          @click="save"
        >
          {{ answered ? $t('node.wizard.add') : $t('node.wizard.addAnyway') }}
        </v-btn>
        <v-btn
          v-if="step === 4 && savedId > 0"
          color="primary"
          variant="tonal"
          prepend-icon="mdi-download"
          @click="emit('import', savedId, draft.name)"
        >
          {{ $t('node.import') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { push } from 'notivue'
import Data from '@/store/modules/data'
import HttpUtils from '@/plugins/httputil'
import { copyText } from '@/plugins/clipboard'
import { i18n } from '@/locales'
import {
  installCommand, newNode, nodePayload, panelUrl, randomToken, validHost, validPath, validPort, webPathOf,
  type NodeStatus,
} from '@/types/node'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: []; manual: []; import: [id: number, name: string] }>()
const store = Data()
const step = ref(1)
const copyHost = ref<HTMLElement | null>(null)

// A random port and path keep a new panel off the addresses scanners try.
const randomPort = () => 20000 + (crypto.getRandomValues(new Uint32Array(1))[0] % 40000)
const form = reactive({ name: '', host: '', port: 2095, path: '/', token: '' })
const reset = () => {
  step.value = 1
  Object.assign(form, { name: '', host: '', port: randomPort(), path: `/${randomToken(10)}/`, token: randomToken(32) })
  result.value = null
  savedId.value = 0
  stopWaiting()
}

const t = (key: string, values?: Record<string, unknown>) => (values ? i18n.global.t(key, values) : i18n.global.t(key))
const nameRule = (v: string) => {
  if (!v) return t('node.wizard.required')
  if (/[[\]]/.test(v)) return t('node.nameRule')
  if (store.nodes.some(n => n.name === v)) return t('node.wizard.nameTaken')
  return true
}
const hostRule = (v: string) => (!!v && validHost(v)) || t('node.wizard.hostHint')
const portRule = (v: number) => validPort(Number(v)) || t('node.wizard.portRule')
const pathRule = (v: string) => validPath(webPathOf(v ?? '')) || t('node.wizard.pathRule')
const formValid = computed(() => [nameRule(form.name), hostRule(form.host), portRule(form.port), pathRule(form.path)].every(r => r === true))
const command = computed(() => (formValid.value ? installCommand({ token: form.token, port: Number(form.port), path: webPathOf(form.path) }) : ''))

const draft = computed(() => {
  const n = newNode()
  n.name = form.name
  n.baseUrl = form.host ? panelUrl(form.host, Number(form.port)) : ''
  n.webPath = webPathOf(form.path || '/')
  n.token = form.token
  return n
})

const startNew = () => { step.value = 2 }
const back = () => {
  stopWaiting()
  step.value -= 1
}
const toConnect = () => {
  result.value = null
  step.value = 3
}

const copy = async () => {
  const ok = await copyText(command.value, copyHost.value)
  if (ok) push.success({ message: t('success') + ': ' + t('copyToClipboard'), duration: 3000 })
  else push.error({ message: t('failed') + ': ' + t('copyToClipboard'), duration: 5000 })
}

// ---- waiting for the node ----
const testing = ref(false)
const waiting = ref(false)
const attempts = ref(0)
const result = ref<NodeStatus | null>(null)
const answered = computed(() => result.value?.state === 'online' || result.value?.state === 'core-stopped')
let timer: ReturnType<typeof setTimeout> | undefined
const maxAttempts = 120

const test = async () => {
  if (testing.value) return
  testing.value = true
  const msg = await HttpUtils.post<NodeStatus>('api/testNode', { data: JSON.stringify(nodePayload(draft.value)) })
  testing.value = false
  if (msg.success) result.value = msg.obj
}
// Each wait has its own number, so a probe still running when the wait is
// stopped and started again cannot start a second chain of probes.
let generation = 0
const tick = async (gen: number) => {
  if (!waiting.value || gen !== generation) return
  attempts.value++
  await test()
  if (!waiting.value || gen !== generation) return
  if (answered.value || attempts.value >= maxAttempts) {
    waiting.value = false
    return
  }
  timer = setTimeout(() => tick(gen), 5000)
}
const startWaiting = () => {
  stopWaiting()
  attempts.value = 0
  waiting.value = true
  tick(generation)
}
function stopWaiting() {
  generation++
  waiting.value = false
  clearTimeout(timer)
}

// ---- adding it ----
const saving = ref(false)
const savedId = ref(0)
const save = async () => {
  stopWaiting()
  saving.value = true
  const ok = await store.save('nodes', 'new', nodePayload(draft.value))
  saving.value = false
  if (!ok) return
  savedId.value = store.nodes.find(n => n.name === draft.value.name)?.id ?? 0
  step.value = 4
}

const close = () => {
  stopWaiting()
  emit('close')
}

watch(() => props.visible, v => {
  if (v) reset()
  else stopWaiting()
})
onBeforeUnmount(stopWaiting)
</script>

<style scoped>
.node-command {
  margin: 0;
  padding: .75rem;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-family: monospace;
  font-size: .8rem;
}
</style>
