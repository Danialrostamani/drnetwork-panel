import { describe, it, expect, vi } from 'vitest'

vi.mock('./httputil', () => ({ logout: vi.fn() }))
vi.mock('notivue', () => ({ push: { error: vi.fn(), success: vi.fn() } }))

import { filenameFromDisposition } from './download'

describe('the name of a download', () => {
  it('prefers the UTF-8 name', () => {
    expect(filenameFromDisposition(`attachment; filename="s-ui_n_.db"; filename*=UTF-8''s-ui_%D9%86%D9%88%D8%AF.db`, 'x'))
      .toBe('s-ui_نود.db')
  })

  it('falls back to the plain name, quoted or not', () => {
    expect(filenameFromDisposition('attachment; filename="a b.zip"', 'x')).toBe('a b.zip')
    expect(filenameFromDisposition('attachment; filename=plain.db', 'x')).toBe('plain.db')
  })

  it('never leaves the downloads folder', () => {
    expect(filenameFromDisposition(`attachment; filename*=UTF-8''..%2F..%2Fetc%2Fpasswd`, 'x')).toBe('_.._etc_passwd')
    expect(filenameFromDisposition('attachment; filename="..\\evil"', 'x')).toBe('_evil')
  })

  it('uses the fallback without a usable name', () => {
    expect(filenameFromDisposition(undefined, 'node.db')).toBe('node.db')
    expect(filenameFromDisposition('attachment', 'node.db')).toBe('node.db')
    expect(filenameFromDisposition(`attachment; filename*=UTF-8''%E0%A4%A`, 'node.db')).toBe('node.db')
  })
})
