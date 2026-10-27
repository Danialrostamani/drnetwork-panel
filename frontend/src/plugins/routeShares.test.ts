import { describe, expect, it } from 'vitest'
import { splitShares, sumShares } from './routeShares'

describe('route shares', () => {
  it('splits evenly to 100', () => {
    const got = splitShares([{ id: 1, weight: 1 }, { id: 2, weight: 1 }, { id: 3, weight: 1 }])
    expect(got.map(e => e.weight)).toEqual([34, 33, 33])
    expect(sumShares(got)).toBe(100)
  })
  it('scales old weights and keeps backups', () => {
    const got = splitShares([{ id: 1, weight: 300 }, { id: 2, weight: 100 }, { id: 3, weight: 0 }])
    expect(got.map(e => e.weight)).toEqual([75, 25, 0])
  })
  it('gives all an equal share when none has one', () => {
    expect(splitShares([{ id: 1, weight: 0 }, { id: 2, weight: 0 }]).map(e => e.weight)).toEqual([50, 50])
  })
})
