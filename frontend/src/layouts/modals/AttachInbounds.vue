<template>
  <v-dialog
    transition="dialog-bottom-transition"
    width="560"
    scrollable
  >
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row>
          <v-col cols="auto">
            {{ $t('bulk.attachToAllTitle') }}
          </v-col>
          <v-spacer />
          <v-col cols="auto">
            <v-icon
              icon="mdi-close"
              @click="$emit('close')"
            />
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <div class="text-body-2 mb-3">
          {{ $t('bulk.attachToAllHint') }}
        </div>
        <v-alert
          v-if="eligible.length == 0"
          type="info"
          variant="outlined"
          :text="$t('bulk.attachToAllNone')"
        />
        <template v-else>
          <div
            v-if="servers.length > 2"
            class="d-flex flex-wrap ga-2 mb-2"
          >
            <v-chip
              v-for="s in servers"
              :key="s.key"
              size="small"
              :color="sameSelection(s.ids) ? 'primary' : undefined"
              :variant="sameSelection(s.ids) ? 'flat' : 'outlined'"
              @click="selected = [...s.ids]"
            >
              {{ s.title }}
            </v-chip>
          </div>
          <v-checkbox
            v-for="i in eligible"
            :key="i.id"
            v-model="selected"
            :value="i.id"
            density="compact"
            hide-details
          >
            <template #label>
              <span dir="ltr">{{ i.tag }}</span>
              <v-chip
                v-if="i.node_id"
                size="x-small"
                color="primary"
                class="ms-2"
              >
                {{ nodeName(i.node_id) }}
              </v-chip>
              <span class="text-caption text-medium-emphasis ms-2">
                {{ missing(i.id) > 0 ? $t('bulk.missingOn', { n: missing(i.id) }) : $t('bulk.everyoneHas') }}
              </span>
            </template>
          </v-checkbox>
          <v-alert
            class="mt-3"
            density="compact"
            variant="tonal"
            :type="pending > 0 ? 'info' : 'success'"
            :text="pending > 0 ? $t('bulk.attachToAllConfirm', { clients: pending, inbounds: selected.length }) : $t('bulk.attachToAllNothing')"
          />
        </template>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          color="primary"
          variant="flat"
          :loading="loading"
          :disabled="pending == 0"
          @click="submit"
        >
          {{ $t('bulk.attachToAll') }}
        </v-btn>
        <v-btn
          variant="outlined"
          @click="$emit('close')"
        >
          {{ $t('actions.close') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import Data from '@/store/modules/data'
import { computed, ref, watch } from 'vue'
import { Inbound } from '@/types/inbounds'
import { i18n } from '@/locales'

// Adds the chosen inbounds -- one, the inbounds of a new node, or all -- to
// every client that lacks them ("attachall" limited to a list).
const props = defineProps<{ visible: boolean; preselect: number[] }>()
const emit = defineEmits<{ close: [] }>()

// The clients a master pushed to this node belong to it, and the server
// leaves them alone, so they are not counted here either.
const clusterGroup = '@cluster'

// The inbounds that take clients are the ones the list gives a "users" field.
const eligible = computed((): Inbound[] => {
  return (Data().inbounds ?? []).filter(i => i.tag != '' && (i as { users?: unknown }).users)
})

const clients = computed(() => (Data().clients ?? []).filter(c => c.group !== clusterGroup))

const nodeName = (id: number): string => Data().nodes.find(n => n.id === id)?.name ?? `#${id}`

const missing = (id: number): number => clients.value.filter(c => !(c.inbounds ?? []).includes(id)).length

// Shortcuts that select the inbounds of one server: all of them, this one,
// or one node.
const servers = computed((): { key: string, title: string, ids: number[] }[] => {
  const out = [{ key: 'all', title: i18n.global.t('bulk.allServers'), ids: eligible.value.map(i => i.id) }]
  const local = eligible.value.filter(i => !i.node_id).map(i => i.id)
  if (local.length > 0) out.push({ key: 'local', title: i18n.global.t('bulk.thisServer'), ids: local })
  const byNode = new Map<number, number[]>()
  for (const i of eligible.value) {
    if (!i.node_id) continue
    byNode.set(i.node_id, [...(byNode.get(i.node_id) ?? []), i.id])
  }
  for (const [id, ids] of byNode) out.push({ key: `node-${id}`, title: nodeName(id), ids })
  return out
})

const selected = ref<number[]>([])

const sameSelection = (ids: number[]): boolean => {
  return ids.length === selected.value.length && ids.every(id => selected.value.includes(id))
}

const pending = computed((): number => {
  return clients.value.filter(c => selected.value.some(id => !(c.inbounds ?? []).includes(id))).length
})

// Opened from an inbound's card, that inbound is chosen; otherwise every
// inbound some client still lacks, which after adding a node is its inbounds.
watch(() => props.visible, v => {
  if (!v) return
  selected.value = props.preselect.length > 0
    ? [...props.preselect]
    : eligible.value.filter(i => missing(i.id) > 0).map(i => i.id)
})

const loading = ref(false)

const submit = async () => {
  loading.value = true
  const success = await Data().save('clients', 'attachall', { inbounds: selected.value }, undefined, i18n.global.t('bulk.attachToAllDone'))
  loading.value = false
  if (success) emit('close')
}
</script>
