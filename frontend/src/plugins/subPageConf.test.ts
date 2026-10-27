import { describe, expect, it } from 'vitest'
import { parseSubPageConf, subPageConfJSON } from './subPageConf'

describe('subPageConf', () => {
  it('reads a broken or empty value as the defaults', () => {
    for (const raw of ['', '{', '[]', 'null', undefined]) {
      const c = parseSubPageConf(raw)
      expect(c.name).toBe('')
      expect(c.hideNet).toBe(false)
      expect(c.notices).toEqual([])
      expect(c.sites).toEqual([])
    }
  })
  it('round-trips and leaves empty fields out', () => {
    const c = parseSubPageConf('{"name":"Dr","hideQr":true,"accent":7,"noBot":"yes","hideVpn":true}')
    expect(c.name).toBe('Dr')
    expect(c.hideQr).toBe(true)
    expect(c.hideVpn).toBe(true)
    expect(c.accent).toBe('')
    expect(c.noBot).toBe(false)
    expect(subPageConfJSON({ ...c, name: '  ', hideVpn: false })).toBe('{"hideQr":true}')
  })
  it('moves the old notice into the list and keeps notices and sites', () => {
    const c = parseSubPageConf('{"notice":"Hi","notices":[{"text":"Off","kind":"warn","until":"2030-01-02"},{"text":"x","kind":"bad","until":"soon"}],"sites":[{"name":"G","url":"https://g.co"}]}')
    expect(c.notices).toEqual([
      { text: 'Hi', textEn: '', kind: 'info', until: '' },
      { text: 'Off', textEn: '', kind: 'warn', until: '2030-01-02' },
      { text: 'x', textEn: '', kind: 'info', until: '' },
    ])
    expect(JSON.parse(subPageConfJSON({ ...c, sites: [...c.sites, { name: ' ', url: '' }] }))).toEqual({
      notices: [{ text: 'Hi' }, { text: 'Off', kind: 'warn', until: '2030-01-02' }, { text: 'x' }],
      sites: [{ name: 'G', url: 'https://g.co' }],
    })
  })
})
