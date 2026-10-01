<template>
  <NodeModal
    v-model="editor.visible"
    :visible="editor.visible"
    :item="editor.item"
    @close="editor.visible = false"
  />
  <NodeImport
    v-model="importer.visible"
    :visible="importer.visible"
    :node-id="importer.id"
    :node-name="importer.name"
    @close="importer.visible = false"
  />

  <v-row
    justify="center"
    class="mb-2"
  >
    <v-col cols="auto">
      <v-btn
        color="primary"
        prepend-icon="mdi-server-plus"
        @click="edit(null)"
      >
        {{ $t('actions.add') }} {{ $t('objects.node') }}
      </v-btn>
    </v-col>
  </v-row>

  <v-row
    v-if="nodes.length === 0"
    justify="center"
  >
    <v-col
      cols="12"
      md="7"
    >
      <v-alert
        type="info"
        variant="tonal"
        :title="$t('node.emptyTitle')"
        :text="$t('node.emptyDesc')"
      />
    </v-col>
  </v-row>

  <v-row>
    <v-col
      v-for="node in nodes"
      :key="node.id"
      cols="12"
      md="6"
      lg="4"
    >
      <v-card
        rounded="xl"
        elevation="4"
      >
        <v-card-title class="d-flex align-center ga-2">
          <v-icon icon="mdi-server-network" />
          <span class="text-truncate">{{ node.name }}</span>
          <v-spacer />
          <v-chip
            size="small"
            label
            :color="statusColor(node)"
            :prepend-icon="statusIcon(node)"
          >
            {{ statusText(node) }}
          </v-chip>
        </v-card-title>
        <v-card-subtitle
          class="text-truncate"
          dir="ltr"
        >
          {{ node.baseUrl }}{{ node.webPath }}
        </v-card-subtitle>
        <v-card-text>
          <v-row dense>
            <v-col cols="6">
              <div class="text-medium-emphasis">
                {{ $t('node.latency') }}
              </div><strong>{{ status(node)?.latency ?? 0 }} ms</strong>
            </v-col>
            <v-col cols="6">
              <div class="text-medium-emphasis">
                CPU / RAM
              </div><strong>{{ cpu(node) }} / {{ memory(node) }}</strong>
            </v-col>
            <v-col cols="6">
              <div class="text-medium-emphasis">
                {{ $t('node.panelVersion') }}
              </div><strong>{{ status(node)?.appVersion || '-' }}</strong>
            </v-col>
            <v-col cols="6">
              <div class="text-medium-emphasis">
                {{ $t('node.coreVersion') }}
              </div><strong>{{ status(node)?.coreVersion || '-' }}</strong>
            </v-col>
            <v-col cols="6">
              <div class="text-medium-emphasis">
                {{ $t('node.lastSeen') }}
              </div><span>{{ time(status(node)?.lastOnline || node.lastSeen) }}</span>
            </v-col>
            <v-col cols="6">
              <div class="text-medium-emphasis">
                {{ $t('node.lastSync') }}
              </div><span>{{ time(node.lastSync) }}</span>
            </v-col>
          </v-row>
          <v-alert
            v-if="status(node)?.error"
            type="error"
            variant="tonal"
            density="compact"
            class="mt-3"
          >
            {{ status(node)?.error }}
          </v-alert>
          <v-chip
            v-if="node.dirty"
            color="warning"
            size="small"
            class="mt-3"
          >
            {{ $t('node.syncDirty') }}
          </v-chip>
          <div
            v-if="node.desc"
            class="text-medium-emphasis mt-3"
          >
            {{ node.desc }}
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-btn
            icon="mdi-file-edit"
            @click="edit(node)"
          >
            <v-icon />
            <v-tooltip
              activator="parent"
              location="top"
              :text="$t('actions.edit')"
            />
          </v-btn>
          <v-btn
            icon="mdi-download"
            @click="openImport(node)"
          >
            <v-icon />
            <v-tooltip
              activator="parent"
              location="top"
              :text="$t('node.import')"
            />
          </v-btn>
          <v-btn
            icon="mdi-sync"
            :loading="syncing[node.id]"
            :disabled="status(node)?.state !== 'online'"
            @click="reconcile(node)"
          >
            <v-icon />
            <v-tooltip
              activator="parent"
              location="top"
              :text="$t('node.reconcile')"
            />
          </v-btn>
          <v-spacer />
          <v-btn
            icon="mdi-delete"
            color="error"
            @click="removeTarget = node"
          >
            <v-icon />
            <v-tooltip
              activator="parent"
              location="top"
              :text="$t('actions.del')"
            />
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>

  <v-dialog
    :model-value="!!removeTarget"
    width="auto"
    @update:model-value="v => { if (!v) removeTarget = null }"
  >
    <v-card
      rounded="lg"
      :title="$t('actions.del')"
    >
      <v-card-text>
        {{ $t('confirm') }}<div class="font-weight-bold mt-2">
          {{ removeTarget?.name }}
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer /><v-btn
          variant="outlined"
          @click="removeTarget = null"
        >
          {{ $t('no') }}
        </v-btn><v-btn
          color="error"
          variant="tonal"
          :loading="deleting"
          @click="remove"
        >
          {{ $t('yes') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import Data from '@/store/modules/data'
import HttpUtils from '@/plugins/httputil'
import NodeModal from '@/layouts/modals/Node.vue'
import NodeImport from '@/layouts/modals/NodeImport.vue'
import type { Node, NodeStatus } from '@/types/node'
import { i18n, locale } from '@/locales'

const store = Data()
const nodes = computed(() => store.nodes)
const editor = reactive<{ visible: boolean; item: Node | null }>({ visible: false, item: null })
const importer = reactive({ visible: false, id: 0, name: '' })
const syncing = ref<Record<number, boolean>>({})
const removeTarget = ref<Node | null>(null)
const deleting = ref(false)
const status = (node: Node): NodeStatus | undefined => store.nodesStatus[node.id]
const state = (node: Node) => !node.enable ? 'disabled' : status(node)?.state ?? 'pending'
const statusColor = (node: Node) => ({ online: 'success', offline: 'error', 'core-stopped': 'warning', disabled: 'default', pending: 'default' }[state(node)] ?? 'default')
const statusIcon = (node: Node) => ({ online: 'mdi-check-circle', offline: 'mdi-alert-circle', 'core-stopped': 'mdi-pause-circle', disabled: 'mdi-cancel', pending: 'mdi-clock-outline' }[state(node)])
const statusText = (node: Node) => state(node) === 'disabled' ? i18n.global.t('disable') : i18n.global.t(`node.status.${state(node) === 'core-stopped' ? 'coreStopped' : state(node)}`)
const cpu = (node: Node) => status(node) ? `${Math.round(status(node)!.cpu)}%` : '-'
const memory = (node: Node) => status(node)?.mem?.total ? `${Math.round(status(node)!.mem.current * 100 / status(node)!.mem.total)}%` : '-'
const time = (timestamp?: number) => timestamp ? new Date(timestamp * 1000).toLocaleString(locale) : '-'
const edit = (node: Node | null) => { editor.item = node; editor.visible = true }
const openImport = (node: Node) => { importer.id = node.id; importer.name = node.name; importer.visible = true }
const reconcile = async (node: Node) => {
  syncing.value = { ...syncing.value, [node.id]: true }
  const msg = await HttpUtils.post('api/reconcileNode', { id: node.id })
  syncing.value = { ...syncing.value, [node.id]: false }
  if (msg.success) { store.lastLoad = 0; await store.loadData() }
}
const remove = async () => {
  if (!removeTarget.value) return
  deleting.value = true
  const ok = await store.save('nodes', 'del', removeTarget.value.id)
  deleting.value = false
  if (ok) removeTarget.value = null
}
</script>
