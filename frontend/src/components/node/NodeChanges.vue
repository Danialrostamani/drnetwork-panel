<template>
  <v-row align="center">
    <v-col
      cols="12"
      sm="4"
      md="3"
    >
      <v-text-field
        v-model.trim="actor"
        density="compact"
        hide-details
        clearable
        :label="$t('admin.actor')"
        @keyup.enter="load"
        @click:clear="actor = ''; load()"
      />
    </v-col>
    <v-col
      cols="6"
      sm="4"
      md="3"
    >
      <v-select
        v-model="key"
        density="compact"
        hide-details
        :label="$t('admin.key')"
        :items="keys"
        @update:model-value="load"
      />
    </v-col>
    <v-col
      cols="6"
      sm="2"
    >
      <v-select
        v-model.number="count"
        density="compact"
        hide-details
        :label="$t('count')"
        :items="[10, 20, 50, 100]"
        @update:model-value="load"
      />
    </v-col>
    <v-col cols="auto">
      <v-btn
        icon="mdi-refresh"
        variant="tonal"
        size="small"
        :loading="loading"
        @click="load"
      />
    </v-col>
  </v-row>
  <v-data-table
    :headers="headers"
    :items="changes"
    item-value="id"
    density="compact"
    show-expand
    :items-per-page="10"
    class="mt-2"
  >
    <template #item.dateTime="{ value }">
      <span
        dir="ltr"
        class="text-no-wrap"
      >{{ fmtTime(value) }}</span>
    </template>
    <template #item.action="{ value }">
      <v-chip density="compact">
        {{ $te('actions.' + value) ? $t('actions.' + value) : value }}
      </v-chip>
    </template>
    <template #expanded-row="{ columns, item }">
      <tr>
        <td :colspan="columns.length">
          <v-card
            color="background"
            dir="ltr"
          >
            <pre class="node-change">{{ item.obj }}</pre>
          </v-card>
        </td>
      </tr>
    </template>
  </v-data-table>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { fmtTime } from './format'

interface Change {
  id: number
  dateTime: number
  actor: string
  key: string
  action: string
  obj?: unknown
}

const props = defineProps<{ nodeId: number }>()
const changes = ref<Change[]>([])
const loading = ref(false)
const actor = ref('')
const key = ref('')
const count = ref(20)
const keys = computed(() => [
  { title: i18n.global.t('all'), value: '' },
  ...['clients', 'inbounds', 'outbounds', 'endpoints', 'services', 'tls', 'config', 'settings', 'nodes'].map(k => ({ title: k, value: k })),
])
const headers = computed(() => [
  { title: 'ID', key: 'id' },
  { title: i18n.global.t('admin.date') + ' - ' + i18n.global.t('admin.time'), key: 'dateTime' },
  { title: i18n.global.t('admin.actor'), key: 'actor' },
  { title: i18n.global.t('admin.key'), key: 'key' },
  { title: i18n.global.t('admin.action'), key: 'action' },
])

const load = async () => {
  if (loading.value) return
  loading.value = true
  const msg = await HttpUtils.get<Change[]>('api/nodeChanges', { id: props.nodeId, a: actor.value ?? '', k: key.value, c: count.value })
  loading.value = false
  changes.value = msg.success && Array.isArray(msg.obj) ? msg.obj : []
}
onMounted(load)
</script>

<style scoped>
.node-change {
  margin: 0;
  padding: .5rem;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: .8rem;
}
</style>
