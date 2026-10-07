<template>
  <v-dialog
    transition="dialog-bottom-transition"
    :fullscreen="smAndDown"
    max-width="1100"
    scrollable
  >
    <v-card
      v-if="visible && node"
      class="rounded-lg"
    >
      <v-card-title class="d-flex align-center ga-2">
        <span
          v-if="flagEmoji(node.country)"
          :title="node.country"
        >{{ flagEmoji(node.country) }}</span>
        <span class="text-truncate">{{ node.name }}</span>
        <v-chip
          size="small"
          label
          :color="viewColor[view]"
          :prepend-icon="viewIcon[view]"
        >
          {{ $t(viewLabelKey(view)) }}
        </v-chip>
        <v-spacer />
        <v-btn
          icon="mdi-close"
          variant="text"
          @click="emit('close')"
        />
      </v-card-title>
      <v-tabs
        v-model="tab"
        show-arrows
        density="compact"
        color="primary"
      >
        <v-tab
          v-for="t in tabs"
          :key="t.value"
          :value="t.value"
          :prepend-icon="t.icon"
        >
          {{ $t('node.tab.' + t.value) }}
        </v-tab>
      </v-tabs>
      <v-divider />
      <v-card-text class="node-details">
        <v-window
          v-model="tab"
          :touch="false"
        >
          <v-window-item value="overview">
            <NodeOverview
              :node="node"
              :status="status"
              :now="now"
            />
          </v-window-item>
          <v-window-item value="online">
            <NodeOnline
              :node-id="node.id"
              :online="online"
            />
          </v-window-item>
          <v-window-item value="charts">
            <NodeCharts :node-id="node.id" />
          </v-window-item>
          <v-window-item value="traffic">
            <NodeTraffic :node-id="node.id" />
          </v-window-item>
          <v-window-item value="uptime">
            <NodeOutages
              :node-id="node.id"
              :now="now"
            />
          </v-window-item>
          <v-window-item value="logs">
            <NodeLogs
              v-if="reachable"
              :node-id="node.id"
            />
            <v-alert
              v-else
              type="info"
              variant="tonal"
              density="compact"
            >
              {{ $t('node.notReachable') }}
            </v-alert>
          </v-window-item>
          <v-window-item value="changes">
            <NodeChanges
              v-if="reachable"
              :node-id="node.id"
            />
            <v-alert
              v-else
              type="info"
              variant="tonal"
              density="compact"
            >
              {{ $t('node.notReachable') }}
            </v-alert>
          </v-window-item>
          <v-window-item value="outbounds">
            <NodeOutbounds
              v-if="online"
              :node-id="node.id"
            />
            <v-alert
              v-else
              type="info"
              variant="tonal"
              density="compact"
            >
              {{ $t('node.notOnline') }}
            </v-alert>
          </v-window-item>
          <v-window-item value="sync">
            <NodeSync
              :node-id="node.id"
              :online="online"
            />
          </v-window-item>
        </v-window>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'
import Data from '@/store/modules/data'
import NodeOverview from '@/components/node/NodeOverview.vue'
import NodeOnline from '@/components/node/NodeOnline.vue'
import NodeCharts from '@/components/node/NodeCharts.vue'
import NodeTraffic from '@/components/node/NodeTraffic.vue'
import NodeOutages from '@/components/node/NodeOutages.vue'
import NodeLogs from '@/components/node/NodeLogs.vue'
import NodeChanges from '@/components/node/NodeChanges.vue'
import NodeOutbounds from '@/components/node/NodeOutbounds.vue'
import NodeSync from '@/components/node/NodeSync.vue'
import { flagEmoji, nodeView, viewColor, viewIcon, viewLabelKey } from '@/types/node'

const props = defineProps<{ visible: boolean; nodeId: number; initialTab: string }>()
const emit = defineEmits<{ close: [] }>()
const { smAndDown } = useDisplay()
const store = Data()

const tabs = [
  { value: 'overview', icon: 'mdi-information-outline' },
  { value: 'online', icon: 'mdi-account-multiple' },
  { value: 'charts', icon: 'mdi-chart-line' },
  { value: 'traffic', icon: 'mdi-chart-bar' },
  { value: 'uptime', icon: 'mdi-heart-pulse' },
  { value: 'logs', icon: 'mdi-text-box-outline' },
  { value: 'changes', icon: 'mdi-history' },
  { value: 'outbounds', icon: 'mdi-export' },
  { value: 'sync', icon: 'mdi-sync' },
]
const tab = ref('overview')
watch(() => props.visible, v => {
  if (v) tab.value = tabs.some(t => t.value === props.initialTab) ? props.initialTab : 'overview'
})

const node = computed(() => store.nodes.find(n => n.id === props.nodeId))
const status = computed(() => store.nodesStatus[props.nodeId])
const view = computed(() => (node.value ? nodeView(node.value, status.value) : 'pending'))
const online = computed(() => !!node.value?.enable && status.value?.state === 'online')
const reachable = computed(() => !!node.value?.enable && (status.value?.state === 'online' || status.value?.state === 'core-stopped'))

// A node deleted while its details are open takes them along.
watch(node, n => {
  if (props.visible && !n) emit('close')
})

const now = ref(Math.floor(Date.now() / 1000))
const ticker = setInterval(() => { now.value = Math.floor(Date.now() / 1000) }, 10000)
onBeforeUnmount(() => clearInterval(ticker))
</script>

<style scoped>
.node-details {
  min-height: 420px;
}
</style>
