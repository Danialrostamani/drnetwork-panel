import { describe, expect, it } from 'vitest'
import { parseSubPageConf, subPageConfJSON } from './subPageConf'

describe('subPageConf', () => {
  it('reads a broken or empty value as the defaults', () => {
    for (const raw of ['', '{', '[]', 'null', undefined]) {
      const c = parseSubPageConf(raw)
      expect(c.name).toBe('')
      expect(c.hideNet).toBe(false)
    }
  })
  it('round-trips and leaves empty fields out', () => {
    const c = parseSubPageConf('{"name":"Dr","hideQr":true,"accent":7,"noBot":"yes"}')
    expect(c.name).toBe('Dr')
    expect(c.hideQr).toBe(true)
    expect(c.accent).toBe('')
    expect(c.noBot).toBe(false)
    expect(subPageConfJSON({ ...c, notice: 'Hi', name: '  ' })).toBe('{"notice":"Hi","hideQr":true}')
  })
})
