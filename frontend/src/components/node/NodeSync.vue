<template>
  <div class="d-flex align-center flex-wrap ga-2 mb-3">
    <v-btn
      color="primary"
      variant="tonal"
      prepend-icon="mdi-sync"
      :loading="running === 'sync'"
      :disabled="!online || !!running"
      @click="run('sync')"
    >
      {{ $t('node.action.sync') }}
    </v-btn>
    <v-btn
      color="warning"
      variant="tonal"
      prepend-icon="mdi-sync-alert"
      :loading="running === 'fullSync'"
      :disabled="!online || !!running"
      @click="confirmFull = true"
    >
      {{ $t('node.action.fullSync') }}
    </v-btn>
    <v-spacer />
    <v-btn
      icon="mdi-refresh"
      variant="tonal"
      size="small"
      :loading="loading"
      @click="load"
    />
  </div>

  <div class="text-subtitle-2 mb-1">
    {{ $t('node.syncPreview') }}
  </div>
  <v-alert
    v-if="!online"
    type="info"
    variant="tonal"
    density="compact"
    class="mb-4"
  >
    {{ $t('node.notOnline') }}
  </v-alert>
  <template v-else-if="preview">
    <div class="d-flex flex-wrap ga-2 mb-2">
      <v-chip
        label
        color="success"
        variant="tonal"
      >
        {{ $t('node.syncAdd') }}: {{ preview.add.length }}
      </v-chip>
      <v-chip
        label
        color="info"
        variant="tonal"
      >
        {{ $t('node.syncEdit') }}: {{ preview.edit.length }}
      </v-chip>
      <v-chip
        label
        color="error"
        variant="tonal"
      >
        {{ $t('node.syncDel') }}: {{ preview.del.length }}
      </v-chip>
      <v-chip
        label
        variant="tonal"
      >
        {{ $t('node.syncSame') }}: {{ preview.same }}
      </v-chip>
      <v-chip
        label
        variant="tonal"
      >
        {{ $t('node.syncReplicas') }}: {{ preview.replicas }}
      </v-chip>
    </div>
    <v-alert
      v-if="preview.restricted"
      type="info"
      variant="tonal"
      density="compact"
      class="mb-2"
    >
      {{ $t('node.syncRestricted', { n: preview.skipped }) }}
    </v-alert>
    <v-alert
      v-if="preview.missing.length > 0"
      type="warning"
      variant="tonal"
      density="compact"
      class="mb-2"
    >
      {{ $t('node.syncMissing') }}: <span dir="ltr">{{ preview.missing.join(', ') }}</span>
    </v-alert>
    <v-alert
      v-if="preview.add.length + preview.edit.length + preview.del.length === 0"
      type="success"
      variant="tonal"
      density="compact"
      class="mb-2"
    >
      {{ $t('node.syncNothing') }}
    </v-alert>
    <v-expansion-panels
      v-else
      variant="accordion"
      class="mb-4"
    >
      <v-expansion-panel
        v-for="list in previewLists"
        :key="list.key"
        :title="`${list.title} (${list.names.length})`"
        :disabled="list.names.length === 0"
      >
        <v-expansion-panel-text>
          <div class="d-flex flex-wrap ga-1">
            <v-chip
              v-for="name in list.names"
              :key="name"
              size="small"
              dir="auto"
            >
              {{ name }}
            </v-chip>
          </div>
        </v-expansion-panel-text>
      </v-expansion-panel>
    </v-expansion-panels>
  </template>

  <div class="text-subtitle-2 mb-1 mt-2">
    {{ $t('node.lastReport') }}
  </div>
  <v-alert
    v-if="!report"
    type="info"
    variant="tonal"
    density="compact"
  >
    {{ $t('node.noReport') }}
  </v-alert>
  <template v-else>
    <div class="mb-2">
      {{ fmtTime(report.at) }} · {{ $t('node.trigger.' + report.trigger) }} · {{ report.duration }} ms
    </div>
    <div class="d-flex flex-wrap ga-2 mb-2">
      <v-chip
        label
        color="success"
        variant="tonal"
      >
        {{ $t('node.syncAdd') }}: {{ report.addedCount }}
      </v-chip>
      <v-chip
        label
        color="info"
        variant="tonal"
      >
        {{ $t('node.syncEdit') }}: {{ report.editedCount }}
      </v-chip>
      <v-chip
        label
        color="error"
        variant="tonal"
      >
        {{ $t('node.syncDel') }}: {{ report.deletedCount }}
      </v-chip>
      <v-chip
        label
        variant="tonal"
      >
        {{ $t('node.syncSame') }}: {{ report.unchanged }}
      </v-chip>
      <v-chip
        v-if="report.skipped > 0"
        label
        variant="tonal"
      >
        {{ $t('node.syncSkipped') }}: {{ report.skipped }}
      </v-chip>
    </div>
    <v-alert
      v-if="report.error"
      type="error"
      variant="tonal"
      density="compact"
      class="mb-2"
    >
      {{ report.error }}
    </v-alert>
    <div
      v-for="list in reportLists"
      :key="list.key"
      class="mb-2"
    >
      <div class="text-caption text-medium-emphasis">
        {{ list.title }}{{ list.more > 0 ? ` (+${list.more})` : '' }}
      </div>
      <div class="d-flex flex-wrap ga-1">
        <v-chip
          v-for="name in list.names"
          :key="name"
          size="small"
          dir="auto"
        >
          {{ name }}
        </v-chip>
      </div>
    </div>
  </template>

  <v-dialog
    v-model="confirmFull"
    max-width="460"
  >
    <v-card
      rounded="lg"
      :title="$t('node.action.fullSync')"
    >
      <v-card-text>{{ $t('node.fullSyncHint') }}</v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="outlined"
          @click="confirmFull = false"
        >
          {{ $t('no') }}
        </v-btn>
        <v-btn
          color="warning"
          variant="tonal"
          @click="confirmFull = false; run('fullSync')"
        >
          {{ $t('yes') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { fmtTime } from './format'
import { runNodeAction } from './actions'
import type { NodeSyncPreview, NodeSyncReport } from '@/types/node'

const props = defineProps<{ nodeId: number; online: boolean }>()
const preview = ref<NodeSyncPreview | null>(null)
const report = ref<NodeSyncReport | null>(null)
const loading = ref(false)
const running = ref<'' | 'sync' | 'fullSync'>('')
const confirmFull = ref(false)

const load = async () => {
  if (loading.value) return
  loading.value = true
  const [r, p] = await Promise.all([
    HttpUtils.get<NodeSyncReport | null>('api/nodeSyncReport', { id: props.nodeId }),
    props.online ? HttpUtils.get<NodeSyncPreview>('api/nodeSyncPreview', { id: props.nodeId }) : Promise.resolve(null),
  ])
  loading.value = false
  if (r.success) report.value = r.obj ?? null
  preview.value = p?.success ? p.obj : null
}
onMounted(load)

const run = async (action: 'sync' | 'fullSync') => {
  running.value = action
  await runNodeAction(action, [props.nodeId])
  running.value = ''
  await load()
}

const previewLists = computed(() => {
  const p = preview.value
  if (!p) return []
  return [
    { key: 'add', title: i18n.global.t('node.syncAdd'), names: p.add },
    { key: 'edit', title: i18n.global.t('node.syncEdit'), names: p.edit },
    { key: 'del', title: i18n.global.t('node.syncDel'), names: p.del },
  ]
})
const reportLists = computed(() => {
  const r = report.value
  if (!r) return []
  return [
    { key: 'added', title: i18n.global.t('node.syncAdd'), names: r.added ?? [], more: r.addedCount - (r.added?.length ?? 0) },
    { key: 'edited', title: i18n.global.t('node.syncEdit'), names: r.edited ?? [], more: r.editedCount - (r.edited?.length ?? 0) },
    { key: 'deleted', title: i18n.global.t('node.syncDel'), names: r.deleted ?? [], more: r.deletedCount - (r.deleted?.length ?? 0) },
  ].filter(l => l.names.length > 0)
})
</script>
