// One person who used the bot's shop, as api/botMembers answers it. Times are
// Unix seconds, 0 when it never happened.
export interface BotMember {
  tgId: number
  name: string
  username: string
  blocked: boolean
  adminId: number
  // The seller their introducer word belongs to, when the owner set none.
  refAdmin?: number
  note: string
  firstSeen: number
  lastSeen: number
  ref: string
  refGroup: string
  trialAt: number
  buys: number
  renews: number
  spent: number
  lastBuy: number
  pending: number
  balance: number
  clients: string[]
  clientIds?: number[]
  trialClients?: number[]
}

export const purchases = (m: BotMember): number => m.buys + m.renews

// What one introducer brought: everybody with that word, and how many of them
// took the test and bought. Members without an introducer come under ''.
export interface RefStat {
  ref: string
  members: number
  trial: number
  buyers: number
  purchases: number
  spent: number
}

export function refStats(members: BotMember[]): RefStat[] {
  const by = new Map<string, RefStat>()
  for (const m of members) {
    const key = m.ref ?? ''
    let s = by.get(key)
    if (!s) {
      s = { ref: key, members: 0, trial: 0, buyers: 0, purchases: 0, spent: 0 }
      by.set(key, s)
    }
    s.members++
    if (m.trialAt > 0) s.trial++
    if (purchases(m) > 0) s.buyers++
    s.purchases += purchases(m)
    s.spent += m.spent
  }
  return [...by.values()].sort((a, b) => b.spent - a.spent || b.members - a.members || a.ref.localeCompare(b.ref))
}

const day = (sec: number): string => sec ? new Date(sec * 1000).toISOString().slice(0, 10) : ''

// A cell as a spreadsheet reads it: quoted when it has to be, and with a
// leading quote mark when it would otherwise start a formula.
export function csvCell(v: string | number | boolean): string {
  let s = String(v)
  if (/^[=+\-@\t\r]/.test(s) && typeof v === 'string') s = "'" + s
  return /[",\r\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s
}

// membersCSV is the list as a CSV file Excel opens with the right encoding.
export function membersCSV(members: BotMember[], sellerName: (id: number) => string): string {
  const head = ['telegram_id', 'name', 'username', 'introducer', 'introducer_group', 'seller', 'joined', 'last_seen',
    'test_account', 'buys', 'renews', 'paid', 'wallet', 'last_purchase', 'pending', 'blocked', 'clients', 'note']
  const rows = members.map(m => [
    m.tgId, m.name, m.username, m.ref, m.refGroup, m.adminId ? sellerName(m.adminId) : m.refAdmin ? sellerName(m.refAdmin) : '', day(m.firstSeen), day(m.lastSeen),
    day(m.trialAt), m.buys, m.renews, m.spent, m.balance, day(m.lastBuy), m.pending, m.blocked ? 'yes' : '', m.clients.join(' '), m.note,
  ])
  return '\uFEFF' + [head, ...rows].map(r => r.map(csvCell).join(',')).join('\r\n') + '\r\n'
}

export function saveText(name: string, text: string, type = 'text/csv;charset=utf-8') {
  const href = URL.createObjectURL(new Blob([text], { type }))
  const a = document.createElement('a')
  a.href = href
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(href), 1000)
}
