import { describe, it, expect, vi, beforeEach } from 'vitest'
import axios from 'axios'

const push = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
const api = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('notivue', () => ({ push }))
vi.mock('./api', () => ({ default: api }))
vi.mock('@/router', () => ({ default: { push: vi.fn() } }))
vi.mock('./auth', () => ({ clearAuthenticated: vi.fn() }))
vi.mock('@/store/modules/data', () => ({ default: () => ({ $reset: vi.fn() }) }))
vi.mock('@/locales', () => ({ i18n: { global: { t: (k: string) => k } } }))

import HttpUtils from './httputil'

describe('HttpUtils', () => {
  beforeEach(() => {
    push.error.mockClear()
    api.get.mockReset()
    api.post.mockReset()
  })

  it('stays quiet about a request dropped for a newer one', async () => {
    api.get.mockRejectedValue(new axios.Cancel('Duplicate request cancelled'))
    api.post.mockRejectedValue(new axios.Cancel('Duplicate request cancelled'))
    expect(await HttpUtils.get('api/x')).toEqual({ success: false, msg: '', obj: null })
    expect((await HttpUtils.post('api/x', {})).success).toBe(false)
    expect(push.error).not.toHaveBeenCalled()
  })

  it('still reports a request that failed', async () => {
    api.get.mockRejectedValue(new Error('Network Error'))
    const msg = await HttpUtils.get('api/x')
    expect(msg.success).toBe(false)
    expect(push.error).toHaveBeenCalledOnce()
  })
})
