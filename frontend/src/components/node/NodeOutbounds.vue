<template>
  <div class="d-flex align-center ga-2 mb-3">
    <span class="text-medium-emphasis text-caption">{{ $t('node.outboundsHint') }}</span>
    <v-spacer />
    <v-btn
      variant="tonal"
      size="small"
      prepend-icon="mdi-speedometer"
      :loading="checkingAll"
      :disabled="outbounds.length === 0"
      @click="checkAll"
    >
      {{ $t('actions.testAll') }}
    </v-btn>
    <v-btn
      icon="mdi-refresh"
      variant="tonal"
      size="small"
      :loading="loading"
      @click="load"
    />
  </div>
  <v-table
    v-if="outbounds.length > 0"
    density="compact"
  >
    <thead>
      <tr>
        <th>{{ $t('objects.tag') }}</th>
        <th>{{ $t('type') }}</th>
        <th>{{ $t('node.result') }}</th>
        <th />
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="o in outbounds"
        :key="o.kind + o.tag"
      >
        <td dir="auto">
          {{ o.tag }}
          <v-chip
            v-if="o.kind === 'endpoint'"
            size="x-small"
            class="ms-1"
          >
            endpoint
          </v-chip>
        </td>
        <td>{{ o.type }}</td>
        <td>
          <template v-if="results[o.tag]">
            <v-chip
              v-if="results[o.tag].ok"
              size="small"
              color="success"
            >
              {{ results[o.tag].delay }} ms
            </v-chip>
            <span
              v-else
              class="text-error text-caption node-error"
            >{{ results[o.tag].error || $t('failed') }}</span>
          </template>
          <span v-else>-</span>
        </td>
        <td class="text-end">
          <v-btn
            size="small"
            variant="text"
            icon="mdi-speedometer"
            :loading="checking[o.tag]"
            @click="check(o.tag)"
          />
        </td>
      </tr>
    </tbody>
  </v-table>
  <v-alert
    v-else-if="loaded"
    type="info"
    variant="tonal"
    density="compact"
  >
    {{ $t('node.noOutbounds') }}
  </v-alert>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'

interface NodeOutbound {
  tag: string
  type: string
  kind: string
}
// What a core answers when it tests an outbound.
interface CheckReply {
  OK: boolean
  Delay?: number
  Error?: string
}
interface Result {
  ok: boolean
  delay: number
  error: string
}

const props = defineProps<{ nodeId: number }>()
const outbounds = ref<NodeOutbound[]>([])
const loading = ref(false)
const loaded = ref(false)
const results = ref<Record<string, Result>>({})
const checking = ref<Record<string, boolean>>({})
const checkingAll = ref(false)

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<NodeOutbound[]>('api/nodeOutbounds', { id: props.nodeId })
  loading.value = false
  loaded.value = true
  outbounds.value = msg.success && Array.isArray(msg.obj) ? msg.obj : []
}
onMounted(load)

const check = async (tag: string) => {
  if (checking.value[tag]) return
  checking.value = { ...checking.value, [tag]: true }
  const msg = await HttpUtils.get<CheckReply>('api/nodeCheckOutbound', { id: props.nodeId, tag })
  checking.value = { ...checking.value, [tag]: false }
  const ok = msg.success && !!msg.obj?.OK
  results.value = {
    ...results.value,
    [tag]: { ok, delay: msg.obj?.Delay ?? 0, error: ok ? '' : (msg.obj?.Error || msg.msg || '') },
  }
}

// A few at a time, not to load the node.
const checkAll = async () => {
  checkingAll.value = true
  const tags = outbounds.value.map(o => o.tag)
  const workers = Array.from({ length: Math.min(3, tags.length) }, async () => {
    for (let tag = tags.shift(); tag !== undefined; tag = tags.shift()) await check(tag)
  })
  await Promise.all(workers)
  checkingAll.value = false
}
</script>

<style scoped>
.node-error {
  overflow-wrap: anywhere;
}
</style>
