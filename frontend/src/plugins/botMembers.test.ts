import { describe, expect, it } from 'vitest'
import { BotMember, csvCell, membersCSV, refStats } from './botMembers'

const member = (o: Partial<BotMember>): BotMember => ({
  tgId: 1, name: '', username: '', blocked: false, adminId: 0, note: '', firstSeen: 0, lastSeen: 0, ref: '', refGroup: '',
  trialAt: 0, buys: 0, renews: 0, spent: 0, lastBuy: 0, pending: 0, balance: 0, clients: [], ...o,
})

describe('bot members', () => {
  it('sums what each introducer brought', () => {
    const stats = refStats([
      member({ tgId: 1, ref: 'a', trialAt: 5, buys: 1, spent: 100 }),
      member({ tgId: 2, ref: 'a', trialAt: 5 }),
      member({ tgId: 3, ref: 'b', buys: 2, renews: 1, spent: 300 }),
      member({ tgId: 4 }),
    ])
    expect(stats).toEqual([
      { ref: 'b', members: 1, trial: 0, buyers: 1, purchases: 3, spent: 300 },
      { ref: 'a', members: 2, trial: 2, buyers: 1, purchases: 1, spent: 100 },
      { ref: '', members: 1, trial: 0, buyers: 0, purchases: 0, spent: 0 },
    ])
  })

  it('quotes CSV cells and defuses formulas', () => {
    expect(csvCell('plain')).toBe('plain')
    expect(csvCell('a,b')).toBe('"a,b"')
    expect(csvCell('say "hi"')).toBe('"say ""hi"""')
    expect(csvCell('=HYPERLINK("x")')).toBe('"\'=HYPERLINK(""x"")"')
    expect(csvCell(-5)).toBe('-5')
  })

  it('writes one CSV row per member, with a BOM for Excel', () => {
    const csv = membersCSV([member({ tgId: 42, name: 'علی', username: 'ali', adminId: 9, firstSeen: 86400, buys: 2, clients: ['c1', 'c2'] })], id => 'seller' + id)
    expect(csv.startsWith('\uFEFFtelegram_id,')).toBe(true)
    const row = csv.trim().split('\r\n')[1]
    expect(row).toBe('42,علی,ali,,,seller9,1970-01-02,,,2,0,0,0,,0,,c1 c2,')
  })
})
