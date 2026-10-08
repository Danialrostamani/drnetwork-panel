<template>
  <v-card :loading="loading">
    <v-tabs
      v-model="tab"
      color="primary"
      align-tabs="center"
      show-arrows
    >
      <v-tab value="t1">
        {{ $t('setting.interface') }}
      </v-tab>
      <v-tab value="t2">
        {{ $t('setting.sub') }}
      </v-tab>
      <v-tab value="t3">
        {{ $t('setting.jsonSub') }}
      </v-tab>
      <v-tab value="t4">
        {{ $t('setting.clashSub') }}
      </v-tab>
      <v-tab value="t5">
        {{ $t('setting.tgBot') }}
      </v-tab>
      <v-tab value="t6">
        {{ $t('setting.tools') }}
      </v-tab>
      <v-tab value="t7">
        {{ $t('panelUpdate.tab') }}
        <v-badge
          v-if="updateAvailable"
          dot
          inline
          color="info"
        />
      </v-tab>
    </v-tabs>
    <v-card-text>
      <v-row
        align="center"
        justify="center"
        style="margin-bottom: 10px;"
      >
        <v-col cols="auto">
          <v-btn
            color="primary"
            :loading="loading"
            :disabled="!stateChange"
            @click="save"
          >
            {{ $t('actions.save') }}
          </v-btn>
        </v-col>
        <v-col cols="auto">
          <v-btn
            variant="outlined"
            color="warning"
            :loading="loading"
            :disabled="stateChange"
            @click="restartApp"
          >
            {{ $t('actions.restartApp') }}
          </v-btn>
        </v-col>
        <!-- The core cannot be held down any other way: a plain stop is undone by
           the watchdog within five seconds. -->
        <v-col cols="auto">
          <v-btn
            variant="outlined"
            :color="maintenance ? 'success' : 'error'"
            :loading="loading"
            :disabled="stateChange"
            @click="toggleMaintenance"
          >
            {{ maintenance ? $t('actions.startCore') : $t('actions.stopCore') }}
          </v-btn>
        </v-col>
      </v-row>
      <v-alert
        v-if="maintenance"
        type="warning"
        variant="tonal"
        density="compact"
        class="mb-4"
        :text="$t('setting.maintenanceOnHint')"
      />
      <v-window v-model="tab">
        <v-window-item value="t1">
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webListen"
                :label="$t('setting.addr')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="webPort"
                min="1"
                type="number"
                :label="$t('setting.port')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webPath"
                :label="$t('setting.webPath')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webDomain"
                :label="$t('setting.domain')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webKeyFile"
                :label="$t('setting.sslKey')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webCertFile"
                :label="$t('setting.sslCert')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.webURI"
                :label="$t('setting.webUri')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="sessionMaxAge"
                type="number"
                min="0"
                :label="$t('setting.sessionAge')"
                :suffix="$t('date.m')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="trafficAge"
                type="number"
                min="0"
                :label="$t('setting.trafficAge')"
                :suffix="$t('date.d')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="statsBucketSeconds"
                v-tooltip:top="$t('setting.statsBucketSecondsHint')"
                type="number"
                min="1"
                :label="$t('setting.statsBucketSeconds')"
                :suffix="$t('date.s')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.timeLocation"
                :label="$t('setting.timeLoc')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.globalReset"
                v-tooltip:top="$t('setting.globalResetHint')"
                :label="$t('setting.globalReset')"
                hide-details
                placeholder="0 0 1 * *"
              />
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t2">
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subEncode"
                color="primary"
                :label="$t('setting.subEncode')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subShowInfo"
                color="primary"
                :label="$t('setting.subInfo')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subPage"
                color="primary"
                :label="$t('setting.subPage')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="subLoadOrder"
                color="primary"
                :label="$t('setting.subLoadOrder')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subListen"
                :label="$t('setting.addr')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="subPort"
                type="number"
                min="1"
                :label="$t('setting.port')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subKeyFile"
                :label="$t('setting.sslKey')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subCertFile"
                :label="$t('setting.sslCert')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subDomain"
                :label="$t('setting.domain')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subPath"
                :label="$t('setting.path')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-row>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="subUpdates"
                type="number"
                min="0"
                :label="$t('setting.update')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.subURI"
                :label="$t('setting.subUri')"
                hide-details
              />
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t3">
          <SubJsonExtVue :settings="settings" />
        </v-window-item>

        <v-window-item value="t4">
          <SubClashExtVue :settings="settings" />
        </v-window-item>

        <v-window-item value="t5">
          <v-row>
            <v-col cols="12">
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
              >
                {{ $t('setting.tgBotHint') }}
              </v-alert>
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="tgBotEnable"
                color="primary"
                :label="$t('setting.tgBotEnable')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="tgBotNotify"
                color="primary"
                :label="$t('setting.tgBotNotify')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-select
                v-model="settings.tgBotLang"
                :items="[{ title: 'فارسی', value: 'fa' }, { title: 'English', value: 'en' }]"
                :label="$t('setting.tgBotLang')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-select
                v-model="settings.tgBotSkin"
                :items="[{ title: $t('setting.tgBotSkinColorful'), value: 'colorful' }, { title: $t('setting.tgBotSkinClassic'), value: 'classic' }]"
                :label="$t('setting.tgBotSkin')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-text-field
                v-model="settings.tgBotToken"
                :label="$t('setting.tgBotToken')"
                placeholder="123456789:AA..."
                autocomplete="off"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-text-field
                v-model="settings.tgBotAdmins"
                :label="$t('setting.tgBotAdmins')"
                :hint="$t('setting.tgBotAdminsHint')"
                persistent-hint
                placeholder="12345678, 87654321"
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-text-field
                v-model="settings.tgBotOwner"
                :label="$t('setting.tgBotOwner')"
                :hint="$t('setting.tgBotOwnerHint')"
                persistent-hint
                placeholder="12345678"
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-textarea
                v-model="settings.tgBotScopes"
                :label="$t('setting.tgBotScopes')"
                :hint="$t('setting.tgBotScopesHint')"
                persistent-hint
                rows="2"
                auto-grow
                placeholder="12345678=Sales"
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-textarea
                v-model="settings.tgBotPerms"
                :label="$t('setting.tgBotPerms')"
                :hint="$t('setting.tgBotPermsHint')"
                persistent-hint
                rows="2"
                auto-grow
                placeholder="12345678=clients,stats"
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-text-field
                v-model="settings.tgBotProxy"
                :label="$t('setting.tgBotProxy')"
                :hint="$t('setting.tgBotProxyHint')"
                persistent-hint
                placeholder="http://127.0.0.1:8080"
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-text-field
                v-model="settings.tgBotReport"
                :label="$t('setting.tgBotReport')"
                :hint="$t('setting.tgBotReportHint')"
                persistent-hint
                placeholder="@daily"
              />
            </v-col>
            <v-col
              cols="12"
              md="6"
            >
              <v-switch
                v-model="tgBotReportBackup"
                color="primary"
                :label="$t('setting.tgBotReportBackup')"
                hide-details
              />
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t6">
          <v-row>
            <v-col cols="12">
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
              >
                {{ $t('setting.filterHint') }}
              </v-alert>
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="filterCheck"
                type="number"
                min="0"
                :label="$t('setting.filterCheck')"
                :suffix="$t('date.m')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-switch
                v-model="filterHide"
                color="primary"
                :label="$t('setting.filterHide')"
                hide-details
              />
            </v-col>
          </v-row>
          <v-divider class="my-4" />
          <v-row>
            <v-col cols="12">
              <v-alert
                type="info"
                variant="tonal"
                density="compact"
              >
                {{ $t('setting.backupHint') }}
              </v-alert>
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-select
                v-model="settings.backupKind"
                :items="backupKinds"
                :label="$t('setting.backupKind')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="8"
            >
              <v-text-field
                v-model="settings.backupUrl"
                :disabled="!settings.backupKind"
                :label="$t('setting.backupUrl')"
                :placeholder="settings.backupKind == 's3' ? 'https://s3.example.com/bucket/folder' : 'https://cloud.example.com/remote.php/dav/files/me/backups'"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.backupUser"
                :disabled="!settings.backupKind"
                :label="settings.backupKind == 's3' ? 'Access key' : $t('login.username')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.backupPass"
                :disabled="!settings.backupKind"
                type="password"
                autocomplete="new-password"
                :label="settings.backupKind == 's3' ? 'Secret key' : $t('login.password')"
                hide-details
              />
            </v-col>
            <v-col
              v-if="settings.backupKind == 's3'"
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model="settings.backupRegion"
                label="Region"
                placeholder="us-east-1"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-text-field
                v-model.number="backupEvery"
                :disabled="!settings.backupKind"
                type="number"
                min="0"
                :label="$t('setting.backupEvery')"
                :suffix="$t('date.h')"
                hide-details
              />
            </v-col>
            <v-col
              cols="12"
              sm="6"
              md="4"
            >
              <v-btn
                :disabled="!settings.backupKind || stateChange"
                :loading="sendingBackup"
                color="primary"
                variant="tonal"
                prepend-icon="mdi-cloud-upload"
                @click="sendBackup"
              >
                {{ $t('setting.backupNow') }}
              </v-btn>
            </v-col>
          </v-row>
        </v-window-item>
        <!-- eager: it looks for a new release as soon as the page opens, so the tab
           can show that one is out. -->
        <v-window-item
          value="t7"
          eager
        >
          <PanelUpdateVue @available="updateAvailable = $event" />
        </v-window-item>
      </v-window>
    </v-card-text>
  </v-card>
</template>

<script lang="ts" setup>
import { i18n } from '@/locales'
import { Ref, computed, inject, onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { FindDiff } from '@/plugins/utils'
import SubJsonExtVue from '@/components/SubJsonExt.vue'
import SubClashExtVue from '@/components/SubClashExt.vue'
import PanelUpdateVue from '@/components/PanelUpdate.vue'
import { push } from 'notivue'
import Data from '@/store/modules/data'
// The Update tab reloads the page into the new version once an update is
// done, and marks that it wants to be open again.
const tab = ref(sessionStorage.getItem('panelUpdate.reopen') ? 't7' : 't1')
sessionStorage.removeItem('panelUpdate.reopen')
// A release newer than this panel is out (the Update tab shows a dot).
const updateAvailable = ref(false)
const loading:Ref = inject('loading')?? ref(false)
const oldSettings = ref<Record<string, string>>({})

const settings = ref({
	webListen: "",
	webDomain: "",
	webPort: "2095",
	webCertFile: "",
	webKeyFile: "",
  webPath: "/app/",
  webURI: "",
	sessionMaxAge: "0",
  trafficAge: "30",
  statsBucketSeconds: "60",
	timeLocation: "Asia/Tehran",
  subListen: "",
	subPort: "2096",
	subPath: "/sub/",
	subDomain: "",
	subCertFile: "",
	subKeyFile: "",
	subUpdates: "12",
	subEncode: "true",
	subShowInfo: "false",
	subPage: "true",
	subLoadOrder: "false",
	filterCheck: "0",
	filterHide: "false",
	backupKind: "",
	backupUrl: "",
	backupUser: "",
	backupPass: "",
	backupRegion: "",
	backupEvery: "0",
	subURI: "",
  subJsonExt: "",
  subClashExt: "",
  subClashNoDefGrp: "false",
  subClashSprtAll: "false",
  subClashUdp: "false",
  globalReset: "",
  tgBotEnable: "false",
  tgBotToken: "",
  tgBotAdmins: "",
  tgBotOwner: "",
  tgBotScopes: "",
  tgBotPerms: "",
  tgBotProxy: "",
  tgBotLang: "fa",
  tgBotSkin: "colorful",
  tgBotNotify: "true",
  tgBotReport: "",
  tgBotReportBackup: "false",
})

// The panel settings, exactly as the block above spells them out.
type PanelSettings = typeof settings.value

onMounted(async () => {
  loading.value = true
  await loadData()
  loading.value = false
})

const loadData = async () => {
  loading.value = true
  const msg = await HttpUtils.get<PanelSettings>('api/settings')
  loading.value = false
  if (msg.success) {
    setData(msg.obj)
  }
}

const setData = (data: PanelSettings) => {
  settings.value = data
  oldSettings.value = { ...data }
}

const save = async () => {
  loading.value = true
  const msg = await HttpUtils.post<{ settings: PanelSettings }>('api/save', { object: 'settings', action: 'set', data: JSON.stringify(settings.value) })
  if (msg.success) {
    push.success({
      title: i18n.global.t('success'),
      duration: 5000,
      message: i18n.global.t('actions.set') + " " + i18n.global.t('pages.settings')
    })
    setData(msg.obj.settings)
  }
  loading.value = false
}

const maintenance = computed((): boolean => Data().maintenance)

// Stopping the core is deliberate downtime for every user, so it is confirmed
// rather than done on a single click.
const toggleMaintenance = async () => {
  const turningOn = !maintenance.value
  if (turningOn && !confirm(i18n.global.t('setting.maintenanceConfirm'))) return

  loading.value = true
  const msg = await HttpUtils.post('api/maintenance', { enable: turningOn })
  if (msg.success) {
    Data().maintenance = turningOn
    push.success({
      title: i18n.global.t('success'),
      duration: 5000,
      message: i18n.global.t(turningOn ? 'setting.maintenanceOn' : 'setting.maintenanceOff')
    })
  }
  loading.value = false
}

const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

const restartApp = async () => {
  loading.value = true
  const msg = await HttpUtils.post('api/restartApp',{})
  if (msg.success) {
    let url = settings.value.webURI
    if (url !== "") {
      const isTLS = settings.value.webCertFile !== "" || settings.value.webKeyFile !== ""
      url = buildURL(settings.value.webDomain,settings.value.webPort.toString(),isTLS, settings.value.webPath)
    }
    await sleep(3000)
    window.location.replace(url)
  }
  loading.value = false
}

const buildURL = (host: string, port: string, isTLS: boolean, path: string) => {
  if (!host || host.length == 0) host = window.location.hostname
  if (!port || port.length == 0) port = window.location.port

  const protocol = isTLS ? "https:" : "http:"

  if (port === "" || (isTLS && port === "443") || (!isTLS && port === "80")) {
      port = ""
  } else {
      port = `:${port}`
  }

  return `${protocol}//${host}${port}${path}settings`
}

const subEncode = computed({
  get: () => { return settings.value.subEncode == "true" },
  set: (v:boolean) => { settings.value.subEncode = v ? "true" : "false" }
})

const tgBotEnable = computed({
  get: () => { return settings.value.tgBotEnable == "true" },
  set: (v:boolean) => { settings.value.tgBotEnable = v ? "true" : "false" }
})
const tgBotReportBackup = computed({
  get: () => { return settings.value.tgBotReportBackup == "true" },
  set: (v:boolean) => { settings.value.tgBotReportBackup = v ? "true" : "false" }
})
const tgBotNotify = computed({
  get: () => { return settings.value.tgBotNotify == "true" },
  set: (v:boolean) => { settings.value.tgBotNotify = v ? "true" : "false" }
})
const filterCheck = computed({
  get: () => { return parseInt(settings.value.filterCheck) || 0 },
  set: (v:number) => { settings.value.filterCheck = v>0 ? Math.floor(v).toString() : "0" }
})

const filterHide = computed({
  get: () => { return settings.value.filterHide == "true" },
  set: (v:boolean) => { settings.value.filterHide = v ? "true" : "false" }
})

const backupEvery = computed({
  get: () => { return parseInt(settings.value.backupEvery) || 0 },
  set: (v:number) => { settings.value.backupEvery = v>0 ? Math.floor(v).toString() : "0" }
})

const backupKinds = [
  { title: '-', value: '' },
  { title: 'S3 (AWS, R2, MinIO, Arvan...)', value: 's3' },
  { title: 'WebDAV (Nextcloud, NAS...)', value: 'webdav' },
]

const sendingBackup = ref(false)
const sendBackup = async () => {
  sendingBackup.value = true
  await HttpUtils.post('api/remoteBackup', {})
  sendingBackup.value = false
}

const subLoadOrder = computed({
  get: () => { return settings.value.subLoadOrder == "true" },
  set: (v:boolean) => { settings.value.subLoadOrder = v ? "true" : "false" }
})

const subPage = computed({
  get: () => { return settings.value.subPage != "false" },
  set: (v:boolean) => { settings.value.subPage = v ? "true" : "false" }
})

const subShowInfo = computed({
  get: () => { return settings.value.subShowInfo == "true" },
  set: (v:boolean) => { settings.value.subShowInfo = v ? "true" : "false" }
})

const webPort = computed({
  get: () => { return settings.value.webPort.length>0 ? parseInt(settings.value.webPort) : 2095 },
  set: (v:number) => { settings.value.webPort = v>0 ? v.toString() : "2095" }
})

const sessionMaxAge = computed({
  get: () => { return settings.value.sessionMaxAge.length>0 ? parseInt(settings.value.sessionMaxAge) : 0 },
  set: (v:number) => { settings.value.sessionMaxAge = v>0 ? v.toString() : "0" }
})

const trafficAge = computed({
  get: () => { return settings.value.trafficAge.length>0 ? parseInt(settings.value.trafficAge) : 0 },
  set: (v:number) => { settings.value.trafficAge = v>0 ? v.toString() : "0" }
})

const statsBucketSeconds = computed({
  get: () => { return settings.value.statsBucketSeconds.length>0 ? parseInt(settings.value.statsBucketSeconds) : 60 },
  set: (v:number) => { settings.value.statsBucketSeconds = v>0 ? v.toString() : "60" }
})

const subPort = computed({
  get: () => { return settings.value.subPort.length>0 ? parseInt(settings.value.subPort) : 2096 },
  set: (v:number) => { settings.value.subPort = v>0 ? v.toString() : "2096" }
})

const subUpdates = computed({
  get: () => { return settings.value.subUpdates.length>0 ? parseInt(settings.value.subUpdates) : 12 },
  set: (v:number) => { settings.value.subUpdates = v>0 ? v.toString() : "12" }
})

const stateChange = computed(() => {
  return !FindDiff.deepCompare(settings.value,oldSettings.value)
})
</script>
