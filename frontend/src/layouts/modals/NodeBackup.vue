<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="90%"
    max-width="500"
  >
    <v-card class="rounded-lg">
      <v-card-title>
        {{ nodeId > 0 ? `${$t('node.action.backup')} · ${nodeName}` : $t('node.action.backupAll') }}
      </v-card-title>
      <v-divider />
      <v-card-text>
        <p
          v-if="nodeId === 0"
          class="mb-2"
        >
          {{ $t('node.backupAllHint') }}
        </p>
        <v-checkbox
          v-model="exclude"
          :label="$t('main.backup.exclStats')"
          value="stats"
          hide-details
        />
        <v-checkbox
          v-model="exclude"
          :label="$t('main.backup.exclChanges')"
          value="changes"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          @click="emit('close')"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          prepend-icon="mdi-download"
          :loading="loading"
          @click="download"
        >
          {{ $t('main.backup.backup') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { downloadFile } from '@/plugins/download'

const props = defineProps<{ visible: boolean; nodeId: number; nodeName: string }>()
const emit = defineEmits<{ close: [] }>()
const exclude = ref<string[]>(['stats', 'changes'])
const loading = ref(false)

watch(() => props.visible, v => {
  if (v) exclude.value = ['stats', 'changes']
})

const download = async () => {
  loading.value = true
  const params: Record<string, string | number> = { exclude: exclude.value.join(',') }
  const ok = props.nodeId > 0
    ? await downloadFile('api/nodeBackup', { ...params, id: props.nodeId }, `s-ui_${props.nodeName}.db`)
    : await downloadFile('api/nodesBackup', params, 's-ui_cluster.zip')
  loading.value = false
  if (ok) emit('close')
}
</script>
