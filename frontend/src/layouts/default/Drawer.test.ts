import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createVuetify } from 'vuetify'
import * as vuetifyComponents from 'vuetify/components'
import * as directives from 'vuetify/directives'
import { VApp } from 'vuetify/components'
import { createRouter, createMemoryHistory } from 'vue-router'
import { h } from 'vue'
import { i18n } from '@/locales'
import Drawer from './Drawer.vue'

vi.mock('@/router', () => ({ default: { currentRoute: { value: { path: '/' } } } }))
vi.mock('@/plugins/httputil', () => ({ logout: vi.fn(), default: { get: vi.fn(), post: vi.fn() } }))

const vuetify = createVuetify({ components: vuetifyComponents, directives })
const empty = { render: () => null }

async function mountDrawer(isMobile = false) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: ['/', '/inbounds', '/clients', '/nodes', '/outbounds', '/endpoints', '/services', '/tls', '/basics', '/rules', '/dns', '/admins', '/settings']
      .map(path => ({ path, component: empty })),
  })
  await router.push('/')
  await router.isReady()
  return mount({ render: () => h(VApp, () => h(Drawer, { isMobile, displayDrawer: true })) }, {
    global: { plugins: [vuetify, router, i18n] },
  })
}

describe('the side menu', () => {
  it.each([false, true])('only leads to the panel\'s own pages and Logout (mobile: %s)', async (isMobile) => {
    const wrapper = await mountDrawer(isMobile)
    const links = wrapper.findAll('a')
    expect(links.length).toBeGreaterThan(10)
    for (const link of links) {
      expect(link.attributes('href'), link.text()).toMatch(/^\//)
    }
    expect(wrapper.find('a[target="_blank"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Logout')
  })

  it('carries no credit to S-UI or its author: that is for the GitHub page', async () => {
    const wrapper = await mountDrawer()
    expect(wrapper.html()).not.toMatch(/alireza|s-ui/i)
  })
})

describe('the translations', () => {
  it('thank nobody inside the panel', () => {
    for (const lang of ['en', 'fa', 'vi', 'zhHans', 'zhHant', 'ru']) {
      const messages = JSON.stringify(i18n.global.getLocaleMessage(lang))
      expect(messages.length, lang).toBeGreaterThan(1000)
      expect(messages, lang).not.toMatch(/alireza/i)
      expect(messages, lang).not.toContain('"credits"')
    }
  })
})
