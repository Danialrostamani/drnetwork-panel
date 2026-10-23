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

// siteCsv is the report as a file to hand on: the window first, then one line
// a site with the bytes and the same in MiB.
export function siteCsv(user: string, start: number, end: number, rows: SiteRow[]): string {
  const iso = (t: number) => new Date(t * 1000).toISOString().replace('T', ' ').slice(0, 16) + ' UTC'
  const lines = [
    `# user,${field(user)}`,
    `# from,${iso(start)}`,
    `# to,${iso(end)}`,
    'site,download_bytes,upload_bytes,total_bytes,download_MiB,upload_MiB,total_MiB',
  ]
  for (const r of rows) {
    lines.push([field(r.site), r.down, r.up, r.total, mb(r.down), mb(r.up), mb(r.total)].join(','))
  }
  return lines.join('\r\n') + '\r\n'
}
