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
  it('writes a CSV that quotes what needs it', () => {
    const csv = siteCsv('ali,1', 0, 3600, [{ site: 'x"y.com', up: 1048576, down: 2097152, total: 3145728 }])
    const lines = csv.trimEnd().split('\r\n')
    expect(lines[0]).toBe('# user,"ali,1"')
    expect(lines[1]).toBe('# from,1970-01-01 00:00 UTC')
    expect(lines[3]).toBe('site,download_bytes,upload_bytes,total_bytes,download_MiB,upload_MiB,total_MiB')
    expect(lines[4]).toBe('"x""y.com",2097152,1048576,3145728,2.00,1.00,3.00')
  })
})
