import { describe, expect, it } from 'vitest'
import { crc32, siteSheet, siteXlsx, xmlText, zip } from './siteXlsx'

const labels = { title: 'Report', user: 'User', from: 'From', to: 'To', sites: 'Sites', rank: '#', site: 'Site', download: 'Down', upload: 'Up', total: 'Total', share: 'Share', readable: 'Readable', sum: 'Sum' }

describe('site xlsx', () => {
  it('computes the standard CRC-32', () => {
    expect(crc32(new TextEncoder().encode('123456789'))).toBe(0xcbf43926)
  })
  it('writes a zip with every part listed', () => {
    const z = zip([{ name: 'a.txt', data: 'hi' }, { name: 'b/c.xml', data: '<x/>' }])
    const v = new DataView(z.buffer)
    expect(v.getUint32(0, true)).toBe(0x04034b50)
    const end = z.length - 22
    expect(v.getUint32(end, true)).toBe(0x06054b50)
    expect(v.getUint16(end + 10, true)).toBe(2)
    expect(new TextDecoder().decode(z)).toContain('b/c.xml')
  })
  it('escapes what XML cannot carry', () => {
    expect(xmlText('a<b>&"c"\u0001')).toBe('a&lt;b&gt;&amp;&quot;c&quot;')
  })
  it('lays out the table with a header, the sites and a totals row', () => {
    const rows = [
      { site: 'google.com', up: 1048576, down: 3 * 1048576, total: 4 * 1048576 },
      { site: '1.2.3.4', up: 0, down: 1048576, total: 1048576 },
    ]
    const xml = siteSheet(labels, 'ali', 'f', 't', rows, v => `${v}B`, true)
    expect(xml).toContain('rightToLeft="1"')
    expect(xml).toContain('<pane ySplit="7" topLeftCell="A8"')
    expect(xml).toContain('<autoFilter ref="A7:G9"/>')
    expect(xml).toContain('<c r="B8" s="4" t="inlineStr"><is><t xml:space="preserve">google.com</t></is></c>')
    expect(xml).toContain('<c r="C8" s="6"><v>3</v></c>')
    expect(xml).toContain('<c r="F8" s="7"><v>0.8</v></c>')
    expect(xml).toContain('<c r="E10" s="9"><v>5</v></c>')
    const book = siteXlsx('ali/x', xml)
    expect(new TextDecoder().decode(book)).toContain('<sheet name="ali x"')
  })
})
