// The subscription page settings (the subPageConf setting): how the page a
// browser gets on a subscription link looks and what it shows.

export const subPageTextKeys = ['name', 'nameEn', 'logo', 'accent', 'support', 'channel', 'notice', 'noticeEn'] as const
export const subPageFlagKeys = ['hideNet', 'hideApps', 'hideQr', 'hideLive', 'hidePlans', 'hideTips', 'noBot'] as const

export type SubPageText = typeof subPageTextKeys[number]
export type SubPageFlag = typeof subPageFlagKeys[number]
export type SubPageConf = Record<SubPageText, string> & Record<SubPageFlag, boolean>

export const parseSubPageConf = (raw: string | undefined): SubPageConf => {
  let o: Record<string, unknown> = {}
  try {
    const v = JSON.parse(raw || '{}')
    if (v && typeof v === 'object' && !Array.isArray(v)) o = v
  } catch { /* a broken value reads as empty */ }
  const out = {} as SubPageConf
  for (const k of subPageTextKeys) out[k] = typeof o[k] === 'string' ? o[k] as string : ''
  for (const k of subPageFlagKeys) out[k] = o[k] === true
  return out
}

/** The setting with c written over it: empty texts and false flags are left out. */
export const subPageConfJSON = (c: SubPageConf): string => {
  const out: Record<string, string | boolean> = {}
  for (const k of subPageTextKeys) if (c[k].trim() !== '') out[k] = c[k]
  for (const k of subPageFlagKeys) if (c[k]) out[k] = true
  return JSON.stringify(out)
}
