<template>
  <v-card subtitle="Wireguard">
    <v-row>
      <v-col
        cols="12"
        sm="8"
      >
        <v-text-field
          v-model="data.private_key"
          :label="$t('types.wg.privKey')"
          append-icon="mdi-key-star"
          hide-details
          @click:append="newKey()"
        />
      </v-col>
      <v-col
        cols="12"
        sm="8"
      >
        <v-text-field
          v-model="public_key"
          readonly
          :label="$t('tls.pubKey')"
          append-icon="mdi-refresh"
          hide-details
          @click:append="getWgPubKey()"
        />
      </v-col>
      <v-col
        cols="12"
        sm="8"
      >
        <v-text-field
          v-model="address"
          :label="$t('types.wg.localIp') + ' ' + $t('commaSeparated')"
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
          v-model.number="data.listen_port"
          :label="$t('in.port')"
          hide-details
          type="number"
          min="1"
        />
      </v-col>
      <v-col
        v-if="data.udp_timeout != undefined"
        cols="12"
        sm="6"
        md="4"
      >
        <v-text-field
          v-model.number="udp_timeout"
          label="UDP Timeout"
          hide-details
          type="number"
          min="0"
          :suffix="$t('date.m')"
        />
      </v-col>
    </v-row>
    <v-row>
      <v-col
        v-if="data.workers != undefined"
        cols="12"
        sm="6"
        md="4"
      >
        <v-text-field
          v-model.number="data.workers"
          :label="$t('types.wg.worker')"
          hide-details
          type="number"
          min="1"
        />
      </v-col>
      <v-col
        v-if="data.mtu != undefined"
        cols="12"
        sm="6"
        md="4"
      >
        <v-text-field
          v-model.number="data.mtu"
          label="MTU"
          hide-details
          type="number"
          min="0"
        />
      </v-col>
    </v-row>
    <v-row>
      <v-col
        cols="12"
        sm="8"
      >
        <v-text-field
          v-model="data.ext.server"
          :label="$t('types.wg.server')"
          hide-details
        />
      </v-col>
      <v-col
        cols="12"
        sm="8"
      >
        <v-text-field
          v-model="data.ext.dns"
          :label="$t('dns.title') + ' ' + $t('commaSeparated')"
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
        <v-switch
          v-model="data.system"
          color="primary"
          :label="$t('types.wg.sysIf')"
          hide-details
        />
      </v-col>
      <v-col
        v-if="data.system"
        cols="12"
        sm="6"
        md="4"
      >
        <v-text-field
          v-model="ifName"
          :label="$t('types.wg.ifName')"
          hide-details
        />
      </v-col>
    </v-row>
    <v-card-actions>
      <v-btn
        variant="tonal"
        prepend-icon="mdi-file-import"
        @click="importOpen = true"
      >
        {{ $t('types.wg.import') }}
      </v-btn>
      <v-spacer />
      <v-menu
        v-model="menu"
        :close-on-content-click="false"
        location="start"
      >
        <template #activator="{ props }">
          <v-btn
            v-bind="props"
            hide-details
            variant="tonal"
          >
            {{ $t('types.wg.options') }}
          </v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item>
              <v-switch
                v-model="optionUdp"
                color="primary"
                label="UDP Timeout"
                hide-details
              />
            </v-list-item>
            <v-list-item>
              <v-switch
                v-model="optionWorker"
                color="primary"
                :label="$t('types.wg.worker')"
                hide-details
              />
            </v-list-item>
            <v-list-item>
              <v-switch
                v-model="optionMtu"
                color="primary"
                label="MTU"
                hide-details
              />
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
  <v-dialog
    v-model="importOpen"
    max-width="560"
  >
    <v-card :title="$t('types.wg.import')">
      <v-card-text>
        <p style="margin-bottom: .75rem;">
          {{ $t('types.wg.importHint') }}
        </p>
        <v-file-input
          :label="$t('types.wg.importFile')"
          accept=".conf,text/plain"
          prepend-icon="mdi-file-document-outline"
          density="compact"
          hide-details
          @update:model-value="readImportFile"
        />
        <v-textarea
          v-model="importText"
          :label="$t('types.wg.importPaste')"
          rows="8"
          style="margin-top: 1rem; font-family: monospace;"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="importOpen = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :disabled="importText.trim().length == 0"
          @click="applyImport"
        >
          {{ $t('types.wg.importApply') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
  <v-card v-if="data.peers != undefined">
    <v-card-subtitle>
      {{ $t('types.wg.peers') }}
      <v-chip
        color="primary"
        density="compact"
        variant="elevated"
        @click="addPeer"
      >
        <v-icon icon="mdi-plus" />
      </v-chip>
    </v-card-subtitle>
    <template
      v-for="(p, index) in data.peers"
      :key="index"
    >
      <v-card style="margin-top: 1rem;">
        <v-card-subtitle>
          {{ $t('types.wg.peer') + ' ' + (Number(index)+1) }} <v-icon
            color="error"
            icon="mdi-delete"
            @click="delPeer(Number(index))"
          />
        </v-card-subtitle>
        <Peer
          :data="p"
          :ext="data.ext"
          @refresh-peer-key="refreshPeerKey(index)"
        />
      </v-card>
    </template>
  </v-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { push } from 'notivue'
import { i18n } from '@/locales'
import Peer from '@/components/WgPeer.vue'
import { Endpoint, WgPeer } from '@/types/endpoints'
import { parseWgConf, WgConfError } from '@/plugins/wgConf'

// The panel-only extras carried beside the endpoint's sing-box options: the
// public key derived from the private one, the key store the peers read their
// private keys from, and the hints handed to the generated client config.
interface WgExt {
  public_key?: string
  server?: string
  dns?: string
  keys: { public_key: string, private_key: string }[]
}

// The WireGuard endpoint this form edits. An option that is switched off is
// removed rather than written out, so those fields are optional here.
interface WireguardData {
  private_key: string
  listen_port: number
  address?: string[]
  name?: string
  system?: boolean
  mtu?: number
  workers?: number
  udp_timeout?: string
  peers?: WgPeer[]
  ext: WgExt
}

// The parent owns the object and this component edits it in place, so it is a
// model rather than a plain prop.
const model = defineModel<Endpoint>('data', { required: true })

// The parent holds a union and picks this component with a v-if on the type,
// which the template checker cannot follow, so narrow once here. The computed
// returns the same object, so edits still reach the parent.
const data = computed(() => model.value as unknown as WireguardData)

const emit = defineEmits<{
  newWgKey: []
  getWgPubKey: [privKey: string]
  addPeer: []
  delPeer: [id: number]
  refreshPeerKey: [id: number]
}>()

const menu = ref(false)

// Importing a .conf: pasted, or read from a file into the same box so it can
// be checked before it replaces anything.
const importOpen = ref(false)
const importText = ref('')

async function readImportFile(files: File | File[] | null | undefined) {
  const file = Array.isArray(files) ? files[0] : files
  if (!file) return
  if (file.size > 64 * 1024) {
    push.error({ message: i18n.global.t('types.wg.importTooBig'), duration: 5000 })
    return
  }
  importText.value = await file.text()
}

// applyImport replaces the interface and the peers with the file's. The
// panel's stored peer keys belong to the peers being replaced, so they go too.
function applyImport() {
  let conf
  try {
    conf = parseWgConf(importText.value)
  } catch (e) {
    const key = e instanceof WgConfError ? e.key : 'importNoInterface'
    push.error({ message: i18n.global.t('types.wg.' + key), duration: 5000 })
    return
  }
  if ((data.value.peers?.length ?? 0) > 0 && !window.confirm(i18n.global.t('types.wg.importReplace'))) return
  data.value.private_key = conf.private_key
  data.value.address = conf.address
  data.value.listen_port = conf.listen_port ?? 0
  data.value.mtu = conf.mtu
  data.value.peers = conf.peers
  if (!data.value.ext) data.value.ext = { keys: [] }
  data.value.ext.keys = []
  data.value.ext.public_key = ''
  emit('getWgPubKey', conf.private_key)
  importOpen.value = false
  importText.value = ''
  push.success({ message: i18n.global.t('types.wg.importDone', { n: conf.peers.length }), duration: 5000 })
}

const optionUdp = computed({
  get: (): boolean => data.value.udp_timeout != undefined,
  set: (v: boolean) => { data.value.udp_timeout = v ? "5m" : undefined },
})

const optionWorker = computed({
  get: (): boolean => data.value.workers != undefined,
  set: (v: boolean) => { data.value.workers = v ? 2 : undefined },
})

const optionMtu = computed({
  get: (): boolean => data.value.mtu != undefined,
  set: (v: boolean) => { data.value.mtu = v ? 1408 : undefined },
})

const ifName = computed({
  get: () => data.value.name ?? '',
  set: (v: string) => { data.value.name = v.length > 0 ? v : undefined },
})

const address = computed({
  get: () => data.value.address?.join(','),
  set: (v: string) => { data.value.address = v.length > 0 ? v.split(',') : undefined },
})

const udp_timeout = computed({
  get: () => data.value.udp_timeout ? parseInt(data.value.udp_timeout.replace('m', '')) : 5,
  set: (v: number) => { data.value.udp_timeout = v > 0 ? v + 'm' : '5m' },
})

const public_key = computed({
  get: () => data.value.ext?.public_key ?? '',
  set: (v: string) => { data.value.ext.public_key = v },
})

function addPeer() {
  emit('addPeer')
}

function delPeer(id: number) {
  emit('delPeer', id)
}

function refreshPeerKey(id: number) {
  emit('refreshPeerKey', id)
}

function newKey() {
  emit('newWgKey')
}

function getWgPubKey() {
  const privKey = data.value.private_key
  if (privKey.length == 0) return
  emit('getWgPubKey', privKey)
}
</script>
