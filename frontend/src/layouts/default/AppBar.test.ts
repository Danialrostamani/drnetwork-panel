import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createVuetify } from 'vuetify'
import * as vuetifyComponents from 'vuetify/components'
import * as directives from 'vuetify/directives'
import { VApp } from 'vuetify/components'
import { createPinia, setActivePinia } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import { i18n } from '@/locales'
import Data from '@/store/modules/data'
import { defaultNode, type Node, type NodeState, type NodeStatus } from '@/types/node'
import AppBar from './AppBar.vue'

vi.mock('@/plugins/httputil', () => ({ default: { get: vi.fn(), post: vi.fn() } }))
vi.mock('notivue', () => ({ push: { error: vi.fn(), success: vi.fn() } }))

const vuetify = createVuetify({ components: vuetifyComponents, directives })
const empty = { render: () => null }

const node = (id: number, enable = true): Node => ({ ...defaultNode, id, enable, name: `n${id}` })
const status = (state: NodeState): NodeStatus => ({
  state, latency: 1, cpu: 0, mem: { current: 0, total: 0 }, appVersion: '', coreVersion: '', checkedAt: 0, lastOnline: 0,
})

async function mountBar() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'pages.home', component: empty },
      { path: '/nodes', name: 'pages.nodes', component: empty },
    ],
  })
  await router.push('/')
  await router.isReady()
  return mount({ render: () => h(VApp, () => h(AppBar, { isMobile: false })) }, {
    global: { plugins: [vuetify, router, i18n] },
  })
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('the top bar', () => {
  it('shows how many enabled nodes are online, and leads to the Nodes page', async () => {
    const store = Data()
    store.nodes = [node(1), node(2), node(3), node(4, false)]
    store.nodesStatus = { 1: status('online'), 2: status('online'), 3: status('core-stopped') }
    const wrapper = await mountBar()
    const chip = wrapper.find('a.v-chip')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toBe('2/3')
    expect(chip.attributes('href')).toBe('/nodes')
    expect(chip.classes()).toContain('text-warning')
  })

  it('turns red when a node is down and green when all answer', async () => {
    const store = Data()
    store.nodes = [node(1), node(2)]
    store.nodesStatus = { 1: status('online'), 2: status('offline') }
    const wrapper = await mountBar()
    expect(wrapper.find('a.v-chip').classes()).toContain('text-error')

    store.nodesStatus = { 1: status('online'), 2: status('online') }
    await wrapper.vm.$nextTick()
    const chip = wrapper.find('a.v-chip')
    expect(chip.text()).toBe('2/2')
    expect(chip.classes()).toContain('text-success')
  })

  it('has no node badge without nodes', async () => {
    const wrapper = await mountBar()
    expect(wrapper.find('a.v-chip').exists()).toBe(false)
    Data().nodes = [node(1, false)]
    await wrapper.vm.$nextTick()
    expect(wrapper.find('a.v-chip').exists()).toBe(false)
  })

  it('no longer carries a donation link', async () => {
    Data().nodes = [node(1)]
    const wrapper = await mountBar()
    expect(wrapper.html()).not.toContain('donate')
    expect(wrapper.html()).not.toContain('alireza')
    expect(wrapper.find('a[target="_blank"]').exists()).toBe(false)
  })
})
