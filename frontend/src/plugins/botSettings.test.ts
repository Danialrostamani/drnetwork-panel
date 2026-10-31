import { describe, expect, it, vi } from 'vitest'

vi.mock('@/plugins/httputil', () => ({ default: { get: vi.fn(), post: vi.fn() } }))
vi.mock('@/store/modules/data', () => ({ default: () => ({ outbounds: [] }) }))
vi.mock('@/locales', () => ({ i18n: { global: { t: (k: string) => k } } }))

import { changedSettings, parseShop, shopJSON } from './botSettings'

describe('bot settings', () => {
  it('sends only what changed', () => {
    expect(changedSettings({ tgBotLang: 'en', tgBotToken: 'x', tgBotAdmins: '1' }, { tgBotLang: 'fa', tgBotToken: 'x', tgBotAdmins: '1' }))
      .toEqual({ tgBotLang: 'en' })
    expect(changedSettings({ a: '1' }, { a: '1' })).toEqual({})
  })

  it('keeps shop keys the page does not edit', () => {
    const raw = JSON.stringify({ menu: [{ data: 'p:buy', fa: 'x' }], welcomeFa: 'hi', desks: { '42': { payCard: '1234', plans: [{ id: '1', name: 'a', days: 30, gb: 10, price: '5' }] } } })
    const shop = parseShop(raw)
    expect(shop.desks['42'].payCard).toBe('1234')
    expect(shop.desks['42'].plans[0].gb).toBe(10)
    const out = JSON.parse(shopJSON(raw, { ...shop, welcomeFa: 'salam', desks: {} }))
    expect(out.menu).toEqual([{ data: 'p:buy', fa: 'x' }])
    expect(out.welcomeFa).toBe('salam')
    expect(out.desks).toEqual({})
  })

  it('reads a broken shop as empty', () => {
    expect(parseShop('{nope').plans).toEqual([])
  })
  it('round-trips a personal sales group independently of access settings', () => {
    const raw = '{"desks":{"42":{"payCard":"1234","salesGroup":"Partners","trialGB":5}}}'
    const s = parseShop(raw)
    expect(s.desks['42'].salesGroup).toBe('Partners')
    s.desks['42'].salesGroup = ' Team A '
    const out = JSON.parse(shopJSON(raw, s))
    expect(out.desks['42'].salesGroup).toBe('Team A')
    expect(out.desks['42'].trialGB).toBe(5)
    expect(out).not.toHaveProperty('tgBotScopes')
    s.desks['42'].salesGroup = ''
    expect(JSON.parse(shopJSON(raw, s)).desks['42'].salesGroup).toBeUndefined()
  })
  it('keeps legacy desks compatible without a personal sales group', () => {
    expect(parseShop('{"desks":{"77":{"payCard":"1234"}}}').desks['77'].salesGroup).toBe('')
  })
  it('keeps what a desk has beyond this page and writes its page links', () => {
    const raw = '{"desks":{"77":{"payCard":"1","trialGB":5,"support":"https://t.me/old"}}}'
    const s = parseShop(raw)
    expect(s.desks['77'].support).toBe('https://t.me/old')
    s.desks['77'] = { ...s.desks['77'], support: '', channel: ' https://t.me/news ' }
    const out = JSON.parse(shopJSON(raw, s))
    expect(out.desks['77'].trialGB).toBe(5)
    expect(out.desks['77'].support).toBeUndefined()
    expect(out.desks['77'].channel).toBe('https://t.me/news')
  })
})
