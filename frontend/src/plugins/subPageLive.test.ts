import { afterEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { runInNewContext } from 'node:vm'
const html = readFileSync(resolve(process.cwd(), '../sub/subPage.html'), 'utf8')
const code = html.slice(html.indexOf('let refreshBusy ='), html.indexOf('let deferredInstall = null;'))
const snapshot = { ok: true, enabled: true, upload: 12, download: 34, total: 100, expire: 0 }
const response = (data = snapshot) => ({ ok: true, headers: { get: () => null }, json: async () => data })
function env() {
  const state = { url: 'https://panel.test/sub/alice', demo: false, offline: false, info: {}, headerMissing: true, serverDate: '', lastFetch: 0 }
  const document = { hidden: false }, navigator = { onLine: true }
  const dash = { hidden: false }, ended = { hidden: true }
  const fetch = vi.fn().mockResolvedValue(response()), renderDash = vi.fn()
  const ctx = { state, document, navigator, fetch, URL, AbortController, location: { origin: 'https://panel.test', href: state.url },
    $: (s: string) => s === '#dash' ? dash : ended, setTimeout, clearTimeout, refreshTimer: null,
    dailyTrack: vi.fn(), renderDash, renderCountdown: vi.fn(), setOffline: vi.fn(), showDash: vi.fn(), parseUserInfo: vi.fn(),
  }
  const api = runInNewContext(code + '\n;({refreshFast,startAutoRefresh,stopAutoRefresh})', ctx) as {
    refreshFast: () => Promise<void>, startAutoRefresh: () => void, stopAutoRefresh: () => void,
  }
  return { ...api, state, document, navigator, fetch, renderDash }
}
afterEach(() => vi.useRealTimers())
describe('actual subscription page live script', () => {
  it('uses a no-store live snapshot without reloading', async () => {
    const e = env(); await e.refreshFast()
    expect(e.fetch.mock.calls[0]?.[0]).toBe('https://panel.test/sub/alice?format=live')
    expect(e.fetch.mock.calls[0]?.[1]).toMatchObject({ method: 'GET', cache: 'no-store' })
    expect(e.state.info).toMatchObject({ upload: 12, download: 34, enabled: true })
    expect(e.renderDash).toHaveBeenCalledWith(true)
  })
  it('does not fetch while hidden, offline or in demo', async () => {
    const e = env(); e.document.hidden = true; await e.refreshFast()
    e.document.hidden = false; e.navigator.onLine = false; await e.refreshFast()
    e.navigator.onLine = true; e.state.demo = true; await e.refreshFast()
    expect(e.fetch).not.toHaveBeenCalled()
  })
  it('never overlaps slow requests', async () => {
    const e = env(); let finish!: (v: unknown) => void
    e.fetch.mockImplementation(() => new Promise(r => { finish = r }))
    const first = e.refreshFast(); await e.refreshFast(); expect(e.fetch).toHaveBeenCalledTimes(1)
    finish(response()); await first
  })
  it('polls each second, pauses and resumes', async () => {
    vi.useFakeTimers(); const e = env(); e.startAutoRefresh()
    await vi.advanceTimersByTimeAsync(1); expect(e.fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000); expect(e.fetch).toHaveBeenCalledTimes(2)
    e.document.hidden = true; e.stopAutoRefresh()
    await vi.advanceTimersByTimeAsync(3000); expect(e.fetch).toHaveBeenCalledTimes(2)
    e.document.hidden = false; e.startAutoRefresh()
    await vi.advanceTimersByTimeAsync(1); expect(e.fetch).toHaveBeenCalledTimes(3)
    e.stopAutoRefresh()
  })
  it('backs off instead of flooding a failed server', async () => {
    vi.useFakeTimers(); const e = env(); e.fetch.mockRejectedValue(new Error('offline')); e.startAutoRefresh()
    await vi.advanceTimersByTimeAsync(1000); expect(e.fetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000); expect(e.fetch).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(3999); expect(e.fetch).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1); expect(e.fetch).toHaveBeenCalledTimes(3); e.stopAutoRefresh()
  })
  it('ignores a late response for a different subscription', async () => {
    const e = env(); let finish!: (v: unknown) => void
    e.fetch.mockImplementation(() => new Promise(r => { finish = r }))
    const first = e.refreshFast(); e.state.url = 'https://panel.test/sub/bob'
    finish(response()); await first; expect(e.state.info).toEqual({}); expect(e.renderDash).not.toHaveBeenCalled()
  })
  it('aborts at eight seconds and can retry', async () => {
    vi.useFakeTimers(); const e = env()
    e.fetch.mockImplementation((_url, opts) => new Promise((_r, reject) => opts.signal.addEventListener('abort', () => reject(new Error('aborted')))))
    const pending = e.refreshFast(); await vi.advanceTimersByTimeAsync(8000); await pending
    e.fetch.mockResolvedValue(response()); await e.refreshFast(); expect(e.state.info).toMatchObject({ upload: 12 })
  })
  it('shows reset counters and changed limits without inventing usage', async () => {
    const e = env(); await e.refreshFast()
    e.fetch.mockResolvedValue(response({ ...snapshot, upload: 0, download: 0, total: 200, enabled: false }))
    await e.refreshFast(); expect(e.state.info).toMatchObject({ upload: 0, download: 0, total: 200, enabled: false })
  })
})
