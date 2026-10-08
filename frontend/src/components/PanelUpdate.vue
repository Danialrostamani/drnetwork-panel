<template>
  <v-row justify="center">
    <v-col
      cols="12"
      md="10"
      lg="8"
    >
      <v-alert
        v-if="info.unsupported"
        type="info"
        variant="tonal"
        density="compact"
        class="mb-4"
      >
        {{ $t('panelUpdate.unsupported.' + info.unsupported) }}
        <template v-if="manualCommand">
          <div class="mt-2">
            {{ $t('panelUpdate.manual') }}
          </div>
          <code
            dir="ltr"
            class="d-block mt-1 update-cmd"
          >{{ manualCommand }}</code>
        </template>
      </v-alert>
      <v-table
        density="compact"
        class="mb-4"
      >
        <tbody>
          <tr>
            <td>{{ $t('panelUpdate.current') }}</td>
            <td dir="ltr">
              {{ info.current || '—' }}
            </td>
          </tr>
          <tr>
            <td>{{ $t('panelUpdate.latest') }}</td>
            <td>
              <v-progress-circular
                v-if="checking"
                indeterminate
                size="16"
                width="2"
              />
              <template v-else-if="info.latest">
                <a
                  :href="info.latestUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  dir="ltr"
                >{{ info.latest }}</a>
                <v-chip
                  size="small"
                  class="ms-2"
                  :color="info.newer ? 'info' : 'success'"
                >
                  {{ info.newer ? $t('panelUpdate.available') : $t('panelUpdate.upToDate') }}
                </v-chip>
              </template>
              <span v-else>—</span>
            </td>
          </tr>
        </tbody>
      </v-table>
      <v-alert
        v-if="info.checkError && !checking"
        type="warning"
        variant="tonal"
        density="compact"
        class="mb-4"
      >
        {{ $t('panelUpdate.checkFailed') }}
        <span dir="ltr">{{ info.checkError }}</span>
      </v-alert>
      <div class="d-flex flex-wrap ga-2 mb-4">
        <v-btn
          variant="outlined"
          :loading="checking"
          :disabled="busy"
          @click="check"
        >
          {{ $t('panelUpdate.check') }}
        </v-btn>
        <v-btn
          color="primary"
          prepend-icon="mdi-update"
          :disabled="!canUpdate"
          :loading="phase === 'starting'"
          @click="confirm = true"
        >
          {{ info.newer ? $t('panelUpdate.updateTo', [info.latest]) : $t('panelUpdate.update') }}
        </v-btn>
      </div>
      <v-alert
        v-if="statusText"
        :type="statusType"
        variant="tonal"
        class="mb-4"
      >
        <div class="d-flex align-center ga-2">
          <v-progress-circular
            v-if="busy"
            indeterminate
            size="18"
            width="2"
          />
          <span>{{ statusText }}</span>
        </div>
      </v-alert>
      <template v-if="info.log">
        <div class="text-subtitle-2 mb-1">
          {{ $t('panelUpdate.log') }}
        </div>
        <pre
          ref="logBox"
          dir="ltr"
          class="update-log"
        >{{ info.log }}</pre>
      </template>
    </v-col>
  </v-row>
  <v-dialog
    v-model="confirm"
    max-width="480"
  >
    <v-card :title="$t('panelUpdate.confirmTitle')">
      <v-card-text>
        {{ $t('panelUpdate.confirmText', [info.latest]) }}
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="confirm = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          @click="start"
        >
          {{ $t('panelUpdate.start') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import api from '@/plugins/api'
import HttpUtils from '@/plugins/httputil'
import { i18n, locale } from '@/locales'

// What GET api/panelUpdate answers: the panel's version, the latest release
// and the last update started from the panel.
interface PanelUpdateInfo {
  current: string
  latest: string
  latestUrl: string
  published: number
  newer: boolean
  checkError?: string
  checkedAt: number
  unsupported?: string
  running: boolean
  target?: string
  from?: string
  started?: number
  exit?: number | null
  log?: string
}

const emit = defineEmits<{ available: [boolean] }>()

const info = ref<PanelUpdateInfo>({
  current: '', latest: '', latestUrl: '', published: 0, newer: false, checkedAt: 0, running: false,
})
const checking = ref(false)
const confirm = ref(false)
const logBox = ref<HTMLElement | null>(null)

// idle: nothing going on. starting: the server was asked to update. running:
// the install script runs. restarting: the panel does not answer while the new
// version replaces it. done, failed, lost: the update finished, failed, or
// the panel did not come back.
type Phase = 'idle' | 'starting' | 'running' | 'restarting' | 'done' | 'failed' | 'lost'
const phase = ref<Phase>('idle')
const target = ref('')
const lostAfter = 5 * 60 * 1000
let downSince = 0
let timer: ReturnType<typeof setTimeout> | undefined

const busy = computed(() => phase.value === 'starting' || phase.value === 'running' || phase.value === 'restarting')
const canUpdate = computed(() => info.value.newer && !info.value.unsupported && !busy.value && phase.value !== 'done')

const manualCommand = computed(() => {
  switch (info.value.unsupported) {
    case 'docker':
      return 'docker compose pull && docker compose up -d'
    case 'os':
      return ''
    default:
      return 'bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh)'
  }
})

const when = (unix?: number) => unix ? new Date(unix * 1000).toLocaleString(locale) : ''

const statusText = computed(() => {
  const t = i18n.global.t
  switch (phase.value) {
    case 'starting':
      return t('panelUpdate.starting')
    case 'running':
      return t('panelUpdate.running', [target.value])
    case 'restarting':
      return t('panelUpdate.restarting')
    case 'done':
      return t('panelUpdate.done', [info.value.current])
    case 'failed':
      return t('panelUpdate.failed')
    case 'lost':
      return t('panelUpdate.lost')
  }
  // What the last update did, if there was one.
  if (info.value.target && info.value.exit != null) {
    const key = info.value.exit === 0 ? 'panelUpdate.lastOk' : 'panelUpdate.lastFailed'
    return t(key, [info.value.target, when(info.value.started)])
  }
  return ''
})

const statusType = computed(() => {
  switch (phase.value) {
    case 'done':
      return 'success'
    case 'failed':
    case 'lost':
      return 'error'
    case 'idle':
      return info.value.exit === 0 ? 'success' : 'warning'
  }
  return 'info'
})

const apply = (got: PanelUpdateInfo) => {
  info.value = got
  emit('available', got.newer && !got.unsupported)
}

// fetchInfo asks quietly: while the panel restarts every request fails, and
// that is no error to show.
const fetchInfo = async (check = false): Promise<PanelUpdateInfo | null> => {
  try {
    const resp = await api.get('api/panelUpdate', { params: check ? { check: 1 } : {} })
    const data = resp.data as { success?: boolean, obj?: PanelUpdateInfo }
    if (data?.success && data.obj) return data.obj
  } catch {
    // Restarting, or the network is down: the caller tries again.
  }
  return null
}

const poll = async () => {
  clearTimeout(timer)
  const got = await fetchInfo()
  if (!got) {
    // The install script stops the panel while it puts the new version in place.
    if (phase.value === 'starting' || phase.value === 'running') phase.value = 'restarting'
    if (!downSince) downSince = Date.now()
    if (Date.now() - downSince > lostAfter) {
      phase.value = 'lost'
      return
    }
    timer = setTimeout(poll, 3000)
    return
  }
  downSince = 0
  apply(got)
  if (got.running) {
    phase.value = 'running'
    if (got.target) target.value = got.target
    timer = setTimeout(poll, 2000)
    return
  }
  // Done when the script says so, or when the panel that answers is the new
  // version (the log was lost on the way, say).
  const isTarget = target.value !== '' && target.value.replace(/^v/, '') === got.current
  if (got.exit === 0 || (got.exit == null && isTarget)) {
    phase.value = 'done'
    // This page still runs the old version's code: load the new one, back on
    // this tab (Settings.vue reads the mark) to show how the update went.
    sessionStorage.setItem('panelUpdate.reopen', '1')
    timer = setTimeout(() => window.location.reload(), 3000)
    return
  }
  phase.value = 'failed'
}

const check = async () => {
  checking.value = true
  const got = await fetchInfo(true)
  checking.value = false
  if (got) apply(got)
}

const start = async () => {
  confirm.value = false
  phase.value = 'starting'
  const msg = await HttpUtils.post<string>('api/panelUpdate', { lang: i18n.global.locale.value })
  if (!msg.success) {
    phase.value = 'idle'
    return
  }
  target.value = msg.obj || info.value.latest
  phase.value = 'running'
  timer = setTimeout(poll, 1500)
}

watch(() => info.value.log, async () => {
  await nextTick()
  if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight
})

onMounted(async () => {
  checking.value = true
  const got = await fetchInfo()
  checking.value = false
  if (!got) return
  apply(got)
  // An update started earlier, from here or another browser, is still going.
  if (got.running) {
    target.value = got.target ?? ''
    phase.value = 'running'
    timer = setTimeout(poll, 2000)
  }
})

onBeforeUnmount(() => clearTimeout(timer))
</script>

<style scoped>
.update-log {
  max-height: 320px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.5;
  padding: 8px 12px;
  border-radius: 4px;
  background: rgba(var(--v-theme-on-surface), 0.06);
  white-space: pre-wrap;
  word-break: break-word;
  text-align: left;
  /* Each line takes its own direction: the script writes the messages in
     the panel's language, Persian ones included. */
  unicode-bidi: plaintext;
}
.update-cmd {
  white-space: pre-wrap;
  word-break: break-all;
  text-align: left;
}
</style>
