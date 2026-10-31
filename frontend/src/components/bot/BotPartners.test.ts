import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createVuetify } from 'vuetify'
import * as components from 'vuetify/components'
import * as directives from 'vuetify/directives'
import { createMemoryHistory, createRouter } from 'vue-router'
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import { useBotSettings } from '@/plugins/botSettings'
import BotPartners from './BotPartners.vue'
vi.mock('@/plugins/httputil', () => ({ default: { get: vi.fn(), post: vi.fn() } }))
vi.mock('@/store/modules/data', () => ({ default: () => ({ outbounds: [] }) }))
vi.mock('notivue', () => ({ push: { success: vi.fn() } }))
const bot = useBotSettings()
let owner = 1000
const vuetify = createVuetify({ components, directives })
const setup = async () => {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { render: () => null } }] })
  await router.push('/')
  await router.isReady()
  await bot.loadBotSettings()
  const wrapper = mount(BotPartners, { global: { plugins: [vuetify, router, i18n] } })
  await flushPromises()
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  owner = 1000
  i18n.global.locale.value = 'en'
  vi.stubGlobal('confirm', vi.fn(() => true))
  vi.mocked(HttpUtils.get).mockImplementation(async url => ({ success: true, msg: '', obj: url === 'api/settings' ? { tgBotOwner: String(owner), tgBotPrivate: 'true' } : { owner, admins: [{ id: 42, role: 'full', name: 'Partner' }, { id: 77, role: 'group', name: 'Seller' }] } } as never))
  vi.mocked(HttpUtils.post).mockResolvedValue({ success: true, msg: '', obj: null })
})
describe('trusted partner box', () => {
  it('shows only full partners and starts with an unusable empty ID', async () => {
    const w = await setup()
    expect(w.text()).toContain('Partner')
    expect(w.text()).not.toContain('Seller')
    expect(w.get('button[type="submit"]').attributes()).toHaveProperty('disabled')
    w.unmount()
  })
  it('normalizes Persian digits and uses one atomic grant request', async () => {
    const w = await setup()
    await w.findAll('input')[2].setValue('۱۲۳۴۵۶۷۸۹')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(HttpUtils.post).toHaveBeenCalledExactlyOnceWith('api/botPartner', { id: '123456789' })
    w.unmount()
  })
  it('does not grant access to the owner, bad IDs, or without a saved owner', async () => {
    const w = await setup()
    const input = w.findAll('input')[2]
    for (const id of ['abc', '0', '-42', '1000']) { await input.setValue(id); await w.get('form').trigger('submit') }
    expect(HttpUtils.post).not.toHaveBeenCalled()
    w.unmount()
    owner = 0
    const noOwner = await setup()
    await noOwner.findAll('input')[2].setValue('42')
    await noOwner.get('form').trigger('submit')
    expect(HttpUtils.post).not.toHaveBeenCalled()
    noOwner.unmount()
  })
  it('does not submit when the trust confirmation is canceled', async () => {
    const w = await setup()
    vi.mocked(window.confirm).mockReturnValue(false)
    await w.findAll('input')[2].setValue('888')
    await w.get('form').trigger('submit')
    expect(HttpUtils.post).not.toHaveBeenCalled()
    w.unmount()
  })
})
