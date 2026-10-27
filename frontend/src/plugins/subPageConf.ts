// The subscription page settings (the subPageConf setting): how the page a
// browser gets on a subscription link looks and what it shows.

export const subPageTextKeys = ['name', 'nameEn', 'logo', 'accent', 'support', 'channel'] as const
export const subPageFlagKeys = [
  'hideNet', 'hideApps', 'hideQr', 'hideLive', 'hidePlans', 'hideTips',
  'hideVpn', 'hideSites', 'hideServers', 'hideReport', 'noBot',
] as const
export const subPageNoticeKinds = ['info', 'warn', 'promo'] as const

export type SubPageText = typeof subPageTextKeys[number]
export type SubPageFlag = typeof subPageFlagKeys[number]
export type SubPageNotice = { text: string, textEn: string, kind: typeof subPageNoticeKinds[number], until: string }
export type SubPageSite = { name: string, url: string }
export type SubPageConf = Record<SubPageText, string> & Record<SubPageFlag, boolean> & {
  notices: SubPageNotice[]
  sites: SubPageSite[]
}

const str = (v: unknown): string => typeof v === 'string' ? v : ''

export const parseSubPageConf = (raw: string | undefined): SubPageConf => {
  let o: Record<string, unknown> = {}
  try {
    const v = JSON.parse(raw || '{}')
    if (v && typeof v === 'object' && !Array.isArray(v)) o = v
  } catch { /* a broken value reads as empty */ }
  const out = { notices: [], sites: [] } as unknown as SubPageConf
  for (const k of subPageTextKeys) out[k] = str(o[k])
  for (const k of subPageFlagKeys) out[k] = o[k] === true
  // The single notice of older versions becomes the first of the list.
  if (str(o.notice).trim() || str(o.noticeEn).trim()) {
    out.notices.push({ text: str(o.notice), textEn: str(o.noticeEn), kind: 'info', until: '' })
  }
  if (Array.isArray(o.notices)) {
    for (const n of o.notices) {
      if (!n || typeof n !== 'object') continue
      const r = n as Record<string, unknown>
      const kind = (subPageNoticeKinds as readonly string[]).includes(str(r.kind)) ? str(r.kind) as SubPageNotice['kind'] : 'info'
      out.notices.push({ text: str(r.text), textEn: str(r.textEn), kind, until: /^\d{4}-\d{2}-\d{2}$/.test(str(r.until)) ? str(r.until) : '' })
    }
  }
  if (Array.isArray(o.sites)) {
    for (const x of o.sites) {
      if (x && typeof x === 'object') out.sites.push({ name: str((x as SubPageSite).name), url: str((x as SubPageSite).url) })
    }
  }
  return out
}

/** The setting with c written over it: empty texts, false flags, empty notices and sites are left out. */
export const subPageConfJSON = (c: SubPageConf): string => {
  const out: Record<string, unknown> = {}
  for (const k of subPageTextKeys) if (c[k].trim() !== '') out[k] = c[k]
  for (const k of subPageFlagKeys) if (c[k]) out[k] = true
  const notices = (c.notices || [])
    .filter(n => n.text.trim() !== '' || n.textEn.trim() !== '')
    .map(n => {
      const r: Record<string, string> = {}
      if (n.text.trim()) r.text = n.text
      if (n.textEn.trim()) r.textEn = n.textEn
      if (n.kind && n.kind !== 'info') r.kind = n.kind
      if (n.until) r.until = n.until
      return r
    })
  if (notices.length) out.notices = notices
  const sites = (c.sites || [])
    .map(x => ({ name: x.name.trim(), url: x.url.trim() }))
    .filter(x => x.name !== '' && x.url !== '')
  if (sites.length) out.sites = sites
  return JSON.stringify(out)
}

