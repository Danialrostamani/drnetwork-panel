// The Telegram bot's settings, shared by every tab of the panel's bot page.
// Only the settings that start with "tgBot" are held here, and a save sends
// just the ones that changed, so it can never put back an older value of a
// setting another screen (or the bot itself) has changed since.
import { computed, ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { i18n } from '@/locales'
import Data from '@/store/modules/data'

export type BotSettings = Record<string, string>

const settings = ref<BotSettings>({})
const oldSettings = ref<BotSettings>({})
const loaded = ref(false)

const pickBot = (data: Record<string, unknown>): BotSettings => {
  const out: BotSettings = {}
  for (const [k, v] of Object.entries(data ?? {})) {
    if (k.startsWith('tgBot')) out[k] = String(v ?? '')
  }
  return out
}

const setData = (data: Record<string, unknown>) => {
  settings.value = pickBot(data)
  oldSettings.value = { ...settings.value }
  loaded.value = true
}

/** The settings that differ from what was loaded. */
export const changedSettings = (cur: BotSettings, old: BotSettings): BotSettings => {
  const out: BotSettings = {}
  for (const [k, v] of Object.entries(cur)) {
    if (old[k] !== v) out[k] = v
  }
  return out
}

const botChanged = computed(() => Object.keys(changedSettings(settings.value, oldSettings.value)).length > 0)

const loadBotSettings = async () => {
  const msg = await HttpUtils.get<Record<string, unknown>>('api/settings')
  if (msg.success && msg.obj) setData(msg.obj)
}

const saveBotSettings = async (): Promise<boolean> => {
  const diff = changedSettings(settings.value, oldSettings.value)
  if (Object.keys(diff).length === 0) return true
  const msg = await HttpUtils.post<{ settings: Record<string, unknown> }>('api/save', { object: 'settings', action: 'set', data: JSON.stringify(diff) })
  if (msg.success && msg.obj?.settings) setData(msg.obj.settings)
  return msg.success
}

const discardBotSettings = () => {
  settings.value = { ...oldSettings.value }
}

// The settings the sellers tab changes on the server itself.
const accessKeys = ['tgBotOwner', 'tgBotAdmins', 'tgBotScopes', 'tgBotPerms']

/** Takes the access settings from the server again, keeping other edits. */
const refreshAccess = async () => {
  const msg = await HttpUtils.get<Record<string, unknown>>('api/settings')
  if (!msg.success || !msg.obj) return
  const fresh = pickBot(msg.obj)
  const next = { ...settings.value }
  const old = { ...oldSettings.value }
  for (const k of accessKeys) {
    next[k] = fresh[k] ?? ''
    old[k] = fresh[k] ?? ''
  }
  settings.value = next
  oldSettings.value = old
}

// ---- switches ----

const flag = (key: string, def = false) => computed({
  get: () => settings.value[key] === undefined ? def : settings.value[key] === 'true',
  set: (v: boolean) => { settings.value = { ...settings.value, [key]: v ? 'true' : 'false' } },
})

// ---- the shop ----

export type ShopPlan = { id: string, name: string, days: number, gb: number, price: string }
export type ShopRef = { id: string, word: string, group: string, admin: string }
export type ShopDesk = { payCard: string, payWallet: string, payHint: string, plans: ShopPlan[], support: string, channel: string }
export type Shop = {
  welcomeFa: string
  guideFa: string
  payCard: string
  payWallet: string
  payHint: string
  trialGB: number
  trialHours: number
  plans: ShopPlan[]
  refs: ShopRef[]
  desks: Record<string, ShopDesk>
}

export const emptyDesk = (): ShopDesk => ({ payCard: '', payWallet: '', payHint: '', plans: [], support: '', channel: '' })
const emptyShop = (): Shop => ({
  welcomeFa: '', guideFa: '', payCard: '', payWallet: '', payHint: '',
  trialGB: 0, trialHours: 0, plans: [], refs: [], desks: {},
})

const parsePlans = (plans: unknown): ShopPlan[] => {
  if (!Array.isArray(plans)) return []
  return plans.map((p: { id?: string, name?: string, days?: number, gb?: number, price?: string }, i: number) => ({
    id: String(p?.id || String(i + 1)),
    name: String(p?.name || ''),
    days: Math.max(0, Math.trunc(Number(p?.days) || 0)),
    gb: Math.max(0, Math.trunc(Number(p?.gb) || 0)),
    price: String(p?.price || ''),
  }))
}

type RawDesk = { payCard?: string, payWallet?: string, payHint?: string, plans?: unknown, support?: string, channel?: string }
const parseDesk = (o: RawDesk | undefined): ShopDesk => ({
  payCard: String(o?.payCard || ''),
  payWallet: String(o?.payWallet || ''),
  payHint: String(o?.payHint || ''),
  plans: parsePlans(o?.plans),
  support: String(o?.support || ''),
  channel: String(o?.channel || ''),
})

export const parseShop = (raw: string): Shop => {
  try {
    const o = JSON.parse(raw || '{}')
    const desksIn = o.desks && typeof o.desks === 'object' ? o.desks as Record<string, RawDesk> : {}
    const desks: Record<string, ShopDesk> = {}
    for (const [id, d] of Object.entries(desksIn)) {
      if (/^\d+$/.test(id)) desks[id] = parseDesk(d)
    }
    const refs = Array.isArray(o.refs) ? o.refs : []
    return {
      welcomeFa: String(o.welcomeFa || ''),
      guideFa: String(o.guideFa || ''),
      payCard: String(o.payCard || ''),
      payWallet: String(o.payWallet || ''),
      payHint: String(o.payHint || ''),
      trialGB: Math.max(0, Math.trunc(Number(o.trialGB) || 0)),
      trialHours: Math.max(0, Math.trunc(Number(o.trialHours) || 0)),
      plans: parsePlans(o.plans),
      refs: refs.map((r: { id?: string, word?: string, group?: string, admin?: number | string }, i: number) => ({
        id: String(r?.id || String(i + 1)),
        word: String(r?.word || ''),
        group: String(r?.group || ''),
        admin: r?.admin ? String(r.admin) : '',
      })),
      desks,
    }
  } catch {
    return emptyShop()
  }
}

/** The shop setting with s written over it, keeping keys this page does not edit. */
export const shopJSON = (raw: string, s: Shop): string => {
  let cur: Record<string, unknown>
  try { cur = JSON.parse(raw || '{}') || {} } catch { cur = {} }
  const refs = s.refs.map((r) => {
    const admin = Number(r.admin)
    return admin > 0 ? { id: r.id, word: r.word, group: r.group, admin } : { id: r.id, word: r.word, group: r.group }
  })
  // A desk keeps what this page does not edit (the bot's trial settings).
  const curDesks = cur.desks && typeof cur.desks === 'object' ? cur.desks as Record<string, Record<string, unknown>> : {}
  const desks: Record<string, Record<string, unknown>> = {}
  for (const [id, d] of Object.entries(s.desks)) {
    const { support, channel, ...rest } = d
    const merged: Record<string, unknown> = { ...(curDesks[id] || {}), ...rest }
    delete merged.support
    delete merged.channel
    if (support.trim()) merged.support = support.trim()
    if (channel.trim()) merged.channel = channel.trim()
    desks[id] = merged
  }
  return JSON.stringify({
    ...cur,
    welcomeFa: s.welcomeFa, guideFa: s.guideFa,
    payCard: s.payCard, payWallet: s.payWallet, payHint: s.payHint,
    trialGB: s.trialGB, trialHours: s.trialHours,
    plans: s.plans,
    refs,
    desks,
  })
}

const shop = computed(() => parseShop(settings.value.tgBotShop || '{}'))
const writeShop = (s: Shop) => {
  settings.value = { ...settings.value, tgBotShop: shopJSON(settings.value.tgBotShop || '{}', s) }
}
const nextId = (list: { id: string }[]) => String((list.reduce((m, p) => Math.max(m, Number(p.id) || 0), 0) || 0) + 1)

const patchShop = (partial: Partial<Shop>) => writeShop({ ...shop.value, ...partial })
const patchShopPlan = (i: number, partial: Partial<ShopPlan>) => {
  writeShop({ ...shop.value, plans: shop.value.plans.map((p, idx) => idx === i ? { ...p, ...partial } : p) })
}
const addShopPlan = () => {
  const s = shop.value
  writeShop({ ...s, plans: [...s.plans, { id: nextId(s.plans), name: '', days: 30, gb: 50, price: '' }] })
}
const removeShopPlan = (i: number) => writeShop({ ...shop.value, plans: shop.value.plans.filter((_, idx) => idx !== i) })
const patchShopRef = (i: number, partial: Partial<ShopRef>) => {
  writeShop({ ...shop.value, refs: shop.value.refs.map((r, idx) => idx === i ? { ...r, ...partial } : r) })
}
const addShopRef = () => {
  const s = shop.value
  writeShop({ ...s, refs: [...s.refs, { id: nextId(s.refs), word: '', group: '', admin: '' }] })
}
const removeShopRef = (i: number) => writeShop({ ...shop.value, refs: shop.value.refs.filter((_, idx) => idx !== i) })

const deskOf = (id: string | number): ShopDesk => shop.value.desks[String(id)] || emptyDesk()
const hasDesk = (id: string | number): boolean => !!shop.value.desks[String(id)]
const patchShopDesk = (id: string | number, partial: Partial<ShopDesk>) => {
  writeShop({ ...shop.value, desks: { ...shop.value.desks, [String(id)]: { ...deskOf(id), ...partial } } })
}
const patchShopDeskPlan = (id: string | number, i: number, partial: Partial<ShopPlan>) => {
  patchShopDesk(id, { plans: deskOf(id).plans.map((p, idx) => idx === i ? { ...p, ...partial } : p) })
}
const addShopDeskPlan = (id: string | number) => {
  const d = deskOf(id)
  patchShopDesk(id, { plans: [...d.plans, { id: nextId(d.plans), name: '', days: 30, gb: 50, price: '' }] })
}
const removeShopDeskPlan = (id: string | number, i: number) => {
  patchShopDesk(id, { plans: deskOf(id).plans.filter((_, idx) => idx !== i) })
}
const removeShopDesk = (id: string | number) => {
  const desks = { ...shop.value.desks }
  delete desks[String(id)]
  writeShop({ ...shop.value, desks })
}

// ---- the bot's connection ----

type OutboundChoice = { title: string, value: string }
const outboundChoices = computed((): OutboundChoice[] => {
  const items: OutboundChoice[] = [{ title: i18n.global.t('setting.tgBotOutboundNone'), value: '' }]
  const seen = new Set<string>()
  for (const o of (Data().outbounds ?? [])) {
    const tag = String((o as { tag?: string }).tag || '').trim()
    if (!tag || seen.has(tag)) continue
    seen.add(tag)
    const typ = String((o as { type?: string }).type || '')
    items.push({ title: typ ? `${tag} (${typ})` : tag, value: tag })
  }
  const cur = String(settings.value.tgBotOutbound || '').trim()
  if (cur && !seen.has(cur)) items.push({ title: cur, value: cur })
  return items
})
const tgBotOutbound = computed({
  get: () => settings.value.tgBotOutbound || '',
  set: (v: unknown) => {
    const val = v && typeof v === 'object' && 'value' in v ? String((v as { value: string }).value || '') : String(v || '')
    settings.value = { ...settings.value, tgBotOutbound: val }
  },
})

const tgBotEnable = flag('tgBotEnable')
const tgBotNotify = flag('tgBotNotify', true)
const tgBotPanel = flag('tgBotPanel')
const tgBotReportBackup = flag('tgBotReportBackup')

export const useBotSettings = () => ({
  settings, oldSettings, loaded, botChanged,
  loadBotSettings, saveBotSettings, discardBotSettings, refreshAccess,
  shop, patchShop, patchShopPlan, addShopPlan, removeShopPlan,
  patchShopRef, addShopRef, removeShopRef,
  deskOf, hasDesk, patchShopDesk, patchShopDeskPlan, addShopDeskPlan, removeShopDeskPlan, removeShopDesk,
  outboundChoices, tgBotOutbound, tgBotEnable, tgBotNotify, tgBotPanel, tgBotReportBackup,
})
