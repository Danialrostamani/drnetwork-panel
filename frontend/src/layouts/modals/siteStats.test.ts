import { describe, expect, it } from 'vitest'
import { siteCsv, siteRows } from './siteStats'

describe('site stats', () => {
  it('sorts the sites biggest first and adds the totals', () => {
    const rows = siteRows({ user: 'a', start: 0, end: 1, sites: [
      { site: 'b.com', up: 1, down: 1 },
      { site: 'a.com', up: 5, down: 10 },
      { site: 'c.com', up: 1, down: 1 },
    ] })
    expect(rows.map(r => r.site)).toEqual(['a.com', 'b.com', 'c.com'])
    expect(rows[0].total).toBe(15)
    expect(siteRows({ user: 'a', start: 0, end: 1, sites: null })).toEqual([])
  })
  it('writes the CSV as one plain table with a totals row', () => {
    const l = { rank: '#', site: 'Site', download: 'Down', upload: 'Up', total: 'Total', share: 'Share', sum: 'Sum' }
    const csv = siteCsv(l, [
      { site: 'x"y.com', up: 1048576, down: 2097152, total: 3145728 },
      { site: 'a,b', up: 0, down: 1048576, total: 1048576 },
    ])
    const lines = csv.trimEnd().split('\r\n')
    expect(lines[0]).toBe('#,Site,Down (MB),Up (MB),Total (MB),Share (%)')
    expect(lines[1]).toBe('1,"x""y.com",2.00,1.00,3.00,75.00')
    expect(lines[2]).toBe('2,"a,b",1.00,0.00,1.00,25.00')
    expect(lines[3]).toBe(',Sum,3.00,1.00,4.00,100.00')
    expect(lines.every(x => !x.startsWith('#,') || x === lines[0])).toBe(true)
  })
})
