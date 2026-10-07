<template>
  <div class="d-flex align-center ga-2 mb-3">
    <v-text-field
      v-model="filter"
      density="compact"
      hide-details
      clearable
      prepend-inner-icon="mdi-magnify"
      :label="$t('node.search')"
      style="max-width: 320px"
    />
    <v-spacer />
    <span
      v-if="data?.checkedAt"
      class="text-caption text-medium-emphasis"
    >{{ $t('node.checkedAt') }}: {{ fmtTime(data.checkedAt) }}</span>
    <v-btn
      icon="mdi-refresh"
      variant="tonal"
      size="small"
      :loading="loading"
      @click="load"
    />
  </div>
  <v-alert
    v-if="!online"
    type="info"
    variant="tonal"
    density="compact"
  >
    {{ $t('node.notOnline') }}
  </v-alert>
  <template v-else>
    <div
      v-for="group in groups"
      :key="group.key"
      class="mb-4"
    >
      <div class="text-subtitle-2 mb-1">
        {{ $t(group.label) }} ({{ group.items.length }})
      </div>
      <div
        v-if="group.items.length > 0"
        class="d-flex flex-wrap ga-1"
      >
        <v-chip
          v-for="name in group.items"
          :key="name"
          size="small"
          :prepend-icon="group.icon"
          dir="auto"
        >
          {{ name }}
        </v-chip>
      </div>
      <div
        v-else
        class="text-medium-emphasis text-caption"
      >
        {{ $t('node.nobody') }}
      </div>
    </div>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { fmtTime } from './format'
import type { NodeOnlines } from '@/types/node'

const props = defineProps<{ nodeId: number; online: boolean }>()
const data = ref<NodeOnlines | null>(null)
const loading = ref(false)
const filter = ref('')

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<NodeOnlines>('api/nodeOnlines', { id: props.nodeId })
  loading.value = false
  if (msg.success) data.value = msg.obj
}

const groups = computed(() => {
  const q = (filter.value ?? '').trim().toLowerCase()
  const pick = (list?: string[]) => (list ?? []).filter(x => !q || x.toLowerCase().includes(q))
  return [
    { key: 'user', label: 'node.onlineUsers', icon: 'mdi-account', items: pick(data.value?.user) },
    { key: 'inbound', label: 'node.onlineInbounds', icon: 'mdi-import', items: pick(data.value?.inbound) },
    { key: 'outbound', label: 'node.onlineOutbounds', icon: 'mdi-export', items: pick(data.value?.outbound) },
  ]
})

// The master refreshes what it knows with every probe.
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  load()
  timer = setInterval(load, 10000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>
