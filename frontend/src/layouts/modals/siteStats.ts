// What api/siteStats answers: a client's traffic per site in a window.
export interface SiteReport {
  user: string
  start: number
  end: number
  sites: { site: string; up: number; down: number }[] | null
  missing?: string[]
}

export interface SiteRow {
  site: string
  up: number
  down: number
  total: number
}

// siteRows is the table of a report, biggest first.
export function siteRows(r: SiteReport): SiteRow[] {
  return (r.sites ?? [])
    .map(s => ({ site: s.site, up: s.up, down: s.down, total: s.up + s.down }))
    .sort((a, b) => b.total - a.total || a.site.localeCompare(b.site))
}

function field(v: string): string {
  return /[",\n\r]/.test(v) ? '"' + v.replaceAll('"', '""') + '"' : v
}

function mb(v: number): string {
  return (v / 1048576).toFixed(2)
}

export interface CsvLabels {
  rank: string
  site: string
  download: string
  upload: string
  total: string
  share: string
  sum: string
}

// siteCsv is the report as one plain table, one row a site plus a totals
// row, in MB so it reads well in any spreadsheet. The user and window go in
// the file name, not into the table.
export function siteCsv(l: CsvLabels, rows: SiteRow[]): string {
  const grand = rows.reduce((a, r) => a + r.total, 0)
  const pct = (v: number) => (grand ? (v * 100) / grand : 0).toFixed(2)
  const lines = [[l.rank, l.site, l.download + ' (MB)', l.upload + ' (MB)', l.total + ' (MB)', l.share + ' (%)'].map(field).join(',')]
  rows.forEach((r, i) => {
    lines.push([String(i + 1), field(r.site), mb(r.down), mb(r.up), mb(r.total), pct(r.total)].join(','))
  })
  const up = rows.reduce((a, r) => a + r.up, 0)
  const down = rows.reduce((a, r) => a + r.down, 0)
  lines.push(['', field(l.sum), mb(down), mb(up), mb(grand), grand ? '100.00' : '0.00'].join(','))
  return lines.join('\r\n') + '\r\n'
}
