<template>
  <v-dialog
    v-model="autoDialog"
    max-width="520"
  >
    <v-card rounded="xl">
      <v-card-title>{{ $t('botAdmins.auto') }}</v-card-title>
      <v-card-text>
        <p class="mb-3">
          {{ $t('botAdmins.autoText') }}
        </p>
        <v-alert
          type="warning"
          variant="tonal"
          density="compact"
          class="mb-3"
        >
          {{ $t('botAdmins.limitWarn') }}
        </v-alert>
        <v-checkbox
          v-model="autoMove"
          :label="$t('botAdmins.moveToo')"
          hide-details
        />
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn
          variant="text"
          @click="autoDialog = false"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          :loading="saving"
          @click="runAuto"
        >
          {{ $t('actions.apply') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-alert
    type="info"
    variant="tonal"
    density="compact"
    class="mb-3"
  >
    {{ $t('botAdmins.ownerNote') }}
  </v-alert>
  <div class="d-flex flex-wrap ga-2 mb-3">
    <v-btn
      color="primary"
      prepend-icon="mdi-account-group"
      :disabled="admins.filter(a => a.role != 'owner').length == 0"
      @click="autoDialog = true"
    >
      {{ $t('botAdmins.auto') }}
    </v-btn>
    <v-btn
      prepend-icon="mdi-refresh"
      variant="text"
      @click="load"
    >
      {{ $t('botMembers.reload') }}
    </v-btn>
  </div>
  <v-card
    rounded="xl"
    elevation="5"
  >
    <v-data-table
      :items="admins"
      :headers="headers"
      :loading="busy"
      :mobile="smAndDown"
      item-value="id"
      items-per-page="-1"
      hide-default-footer
      density="compact"
    >
      <template #item.admin="{ item }">
        <div class="py-1">
          <b>{{ item.name || '-' }}</b>
          <div class="text-caption">
            <bdi>{{ item.id }}</bdi>
          </div>
        </div>
      </template>
      <template #item.role="{ item }">
        <v-chip
          size="small"
          :color="roleColor(item.role)"
        >
          {{ $t('botAdmins.role.' + item.role) }}
        </v-chip>
      </template>
      <template #item.group="{ item }">
        <span v-if="item.role == 'owner'">{{ $t('botAdmins.allGroups') }}</span>
        <div
          v-else
          class="d-flex align-center ga-1"
          style="min-width: 220px;"
        >
          <v-combobox
            v-model="edit[item.id]"
            :items="groups"
            :placeholder="$t('botAdmins.noGroup')"
            density="compact"
            hide-details
            clearable
          />
          <v-btn
            icon="mdi-content-save"
            size="small"
            variant="text"
            color="primary"
            :disabled="(edit[item.id] ?? '').trim() == item.group"
            :loading="saving"
            @click="saveGroup(item)"
          >
            <v-icon />
            <v-tooltip
              activator="parent"
              location="top"
              :text="$t('actions.save')"
            />
          </v-btn>
        </div>
      </template>
      <template #item.revenue="{ item }">
        {{ item.sales }}
        <span
          v-if="item.revenue"
          class="text-caption"
        > ({{ fmtNum(item.revenue) }})</span>
      </template>
      <template #item.outside="{ item }">
        <v-btn
          v-if="item.role == 'group' && item.outside > 0"
          size="small"
          variant="tonal"
          color="warning"
          prepend-icon="mdi-folder-move"
          :loading="saving"
          @click="moveClients(item)"
        >
          {{ $t('botAdmins.move', { n: item.outside }) }}
        </v-btn>
        <span v-else>-</span>
      </template>
    </v-data-table>
  </v-card>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import { push } from 'notivue'
import { computed, onMounted, ref } from 'vue'
import { useDisplay } from 'vuetify'

// One bot administrator as api/botAdmins answers it.
interface BotAdmin {
  id: number
  name: string
  role: 'owner' | 'full' | 'sections' | 'group' | 'locked'
  group: string
  sections: string[]
  clients: number
  customers: number
  sales: number
  revenue: number
  outside: number
}

const { smAndDown } = useDisplay()
const t = (key: string, args: Record<string, unknown> = {}) => i18n.global.t(key, args)
const fmtNum = (n: number): string => (n ?? 0).toLocaleString('en-US')

const admins = ref<BotAdmin[]>([])
const groups = ref<string[]>([])
const edit = ref<Record<number, string>>({})
const busy = ref(false)
const saving = ref(false)
const autoDialog = ref(false)
const autoMove = ref(true)

const headers = computed(() => [
  { title: t('botAdmins.admin'), key: 'admin', sortable: false },
  { title: t('botAdmins.roleTitle'), key: 'role' },
  { title: t('botAdmins.group'), key: 'group', sortable: false },
  { title: t('botAdmins.clients'), key: 'clients' },
  { title: t('botAdmins.customers'), key: 'customers' },
  { title: t('botAdmins.sales'), key: 'revenue' },
  { title: t('botAdmins.outside'), key: 'outside', sortable: false },
])

const roleColor = (r: string) => ({ owner: 'primary', full: 'success', sections: 'info', group: 'secondary', locked: 'error' } as Record<string, string>)[r] ?? ''

const load = async () => {
  busy.value = true
  const msg = await HttpUtils.get<{ admins: BotAdmin[], groups: string[] }>('api/botAdmins')
  busy.value = false
  if (msg.success && msg.obj) {
    admins.value = msg.obj.admins ?? []
    groups.value = msg.obj.groups ?? []
    edit.value = Object.fromEntries(admins.value.map(a => [a.id, a.group]))
  }
}

onMounted(load)

const saveGroup = async (a: BotAdmin) => {
  const group = (edit.value[a.id] ?? '').trim()
  if (group != '' && !confirm(t('botAdmins.limitConfirm', { group }))) return
  if (group == '' && !confirm(t('botAdmins.freeConfirm'))) return
  saving.value = true
  const msg = await HttpUtils.post<{ moved: number }>('api/botAdminGroup', { id: a.id, group, move: group != '' })
  saving.value = false
  if (msg.success) {
    push.success({ title: t('botAdmins.saved', { n: msg.obj?.moved ?? 0 }) })
    await load()
  }
}

const moveClients = async (a: BotAdmin) => {
  saving.value = true
  const msg = await HttpUtils.post<{ moved: number }>('api/botAdminMove', { id: a.id })
  saving.value = false
  if (msg.success) {
    push.success({ title: t('botAdmins.saved', { n: msg.obj?.moved ?? 0 }) })
    await load()
  }
}

const runAuto = async () => {
  saving.value = true
  const msg = await HttpUtils.post<{ id: number, group: string, new: boolean, moved: number }[]>('api/botAdminsAuto', { move: autoMove.value })
  saving.value = false
  if (msg.success) {
    const list = msg.obj ?? []
    push.success({
      title: t('botAdmins.autoDone', { n: list.filter(x => x.new).length, m: list.reduce((n, x) => n + x.moved, 0) }),
    })
    autoDialog.value = false
    await load()
  }
}
</script>
