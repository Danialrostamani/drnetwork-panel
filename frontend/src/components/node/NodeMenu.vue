<template>
  <v-menu location="bottom end">
    <template #activator="{ props: activator }">
      <v-btn
        v-bind="activator"
        icon="mdi-dots-vertical"
        variant="text"
        :loading="busy"
      />
    </template>
    <v-list
      density="compact"
      nav
    >
      <template
        v-for="item in items"
        :key="item.key"
      >
        <v-divider
          v-if="item.divider"
          class="my-1"
        />
        <v-list-item
          v-else
          :prepend-icon="item.icon"
          :title="item.title"
          :disabled="item.disabled"
          :base-color="item.color"
          @click="emit('pick', item.key)"
        />
      </template>
    </v-list>
  </v-menu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { i18n } from '@/locales'
import type { Node, NodeStatus } from '@/types/node'

const props = defineProps<{ node: Node; status?: NodeStatus; busy?: boolean }>()
const emit = defineEmits<{ pick: [key: string] }>()

interface MenuItem {
  key: string
  icon?: string
  title?: string
  disabled?: boolean
  color?: string
  divider?: boolean
}

const items = computed((): MenuItem[] => {
  const t = (k: string) => i18n.global.t(k)
  const enabled = props.node.enable
  const online = enabled && props.status?.state === 'online'
  // The panel of a node whose core is stopped still answers.
  const reachable = enabled && (props.status?.state === 'online' || props.status?.state === 'core-stopped')
  const resting = !!props.status?.maintenance
  return [
    { key: 'details', icon: 'mdi-chart-box-outline', title: t('node.details') },
    { key: 'probe', icon: 'mdi-radar', title: t('node.action.probe'), disabled: !enabled },
    { key: 'sync', icon: 'mdi-sync', title: t('node.action.sync'), disabled: !online },
    { key: 'fullSync', icon: 'mdi-sync-alert', title: t('node.action.fullSync'), disabled: !online },
    { key: 'd1', divider: true },
    { key: 'restartSb', icon: 'mdi-restart', title: t('node.action.restartSb'), disabled: !reachable },
    { key: 'restartApp', icon: 'mdi-power', title: t('node.action.restartApp'), disabled: !reachable },
    { key: 'updatePanel', icon: 'mdi-update', title: t('node.action.updatePanel'), disabled: !reachable },
    resting
      ? { key: 'maintenanceOff', icon: 'mdi-wrench-check', title: t('node.action.maintenanceOff'), disabled: !reachable }
      : { key: 'maintenanceOn', icon: 'mdi-wrench-clock', title: t('node.action.maintenanceOn'), disabled: !reachable },
    { key: 'd2', divider: true },
    { key: 'import', icon: 'mdi-download', title: t('node.import'), disabled: !reachable },
    { key: 'backup', icon: 'mdi-database-export', title: t('node.action.backup'), disabled: !reachable },
    { key: 'clone', icon: 'mdi-content-copy', title: t('node.action.clone') },
    enabled
      ? { key: 'disable', icon: 'mdi-toggle-switch-off-outline', title: t('node.action.disable') }
      : { key: 'enable', icon: 'mdi-toggle-switch', title: t('node.action.enable') },
    { key: 'd3', divider: true },
    { key: 'edit', icon: 'mdi-file-edit', title: t('actions.edit') },
    { key: 'del', icon: 'mdi-delete', title: t('actions.del'), color: 'error' },
  ]
})
</script>
