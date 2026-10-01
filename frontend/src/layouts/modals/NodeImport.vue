<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="600"
  >
    <v-card
      class="rounded-lg"
      :loading="loading || importing"
    >
      <v-card-title>{{ $t('node.import') }} · {{ nodeName }}</v-card-title>
      <v-divider />
      <v-card-text>
        <v-skeleton-loader
          v-if="loading"
          type="list-item-three-line@3"
        />
        <v-alert
          v-else-if="inbounds.length === 0"
          type="info"
          variant="tonal"
        >
          {{ $t('node.noRemoteInbounds') }}
        </v-alert>
        <v-list
          v-else
          lines="two"
        >
          <v-list-item
            v-for="ib in inbounds"
            :key="ib.tag"
            :title="ib.tag"
            :subtitle="ib.type"
            :disabled="ib.adopted"
            @click="toggle(ib.tag, ib.adopted)"
          >
            <template #prepend>
              <v-checkbox-btn
                :model-value="selected.has(ib.tag)"
                :disabled="ib.adopted"
              />
            </template>
            <template #append>
              <v-chip
                v-if="ib.adopted"
                color="success"
                size="small"
              >
                {{ $t('node.adopted') }}
              </v-chip>
            </template>
          </v-list-item>
        </v-list>
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
          :disabled="selected.size === 0"
          :loading="importing"
          @click="adopt"
        >
          {{ $t('node.import') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import Data from '@/store/modules/data'

interface RemoteInbound { id: number; type: string; tag: string; adopted: boolean }
const props = defineProps<{ visible: boolean; nodeId: number; nodeName: string }>()
const emit = defineEmits<{ close: [] }>()
const inbounds = ref<RemoteInbound[]>([])
const selected = ref(new Set<string>())
const loading = ref(false)
const importing = ref(false)

const load = async () => {
  loading.value = true
  selected.value = new Set()
  const msg = await HttpUtils.get<RemoteInbound[]>('api/nodeInbounds', { id: props.nodeId })
  inbounds.value = msg.success ? (msg.obj ?? []) : []
  loading.value = false
}
watch(() => props.visible, v => { if (v) load() })
const toggle = (tag: string, adopted: boolean) => {
  if (adopted) return
  const next = new Set(selected.value)
  if (next.has(tag)) next.delete(tag)
  else next.add(tag)
  selected.value = next
}
const adopt = async () => {
  importing.value = true
  const msg = await HttpUtils.post('api/adoptInbounds', { id: props.nodeId, tags: JSON.stringify([...selected.value]) })
  importing.value = false
  if (msg.success) {
    Data().lastLoad = 0
    await Data().loadData()
    emit('close')
  }
}
</script>
