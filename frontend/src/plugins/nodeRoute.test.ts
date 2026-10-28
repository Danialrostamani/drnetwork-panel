import { describe, expect, it } from 'vitest'
import { parseNodeRoute, serializeNodeRoute, normalizeRouteEntry } from './nodeRoute'

describe('smart route settings', () => {
  it('keeps existing percentage settings opt-out', () => {
    expect(parseNodeRoute('{"enable":true,"entries":[{"id":1,"weight":70}]}')).toMatchObject({ enable: true, smart: false, entries: [{ id: 1, weight: 70 }] })
  })
  it('round trips every capacity control and drain', () => {
    const r = parseNodeRoute('{"enable":true,"smart":true,"entries":[{"id":1,"weight":50,"capacity":2.5,"bandwidthMbps":500,"maxUsers":90,"drain":true}]}')
    expect(parseNodeRoute(serializeNodeRoute(r))).toEqual(r)
    expect(r.entries[0].capacity).toBe(2.5)
    expect(r.entries[0].drain).toBe(true)
  })
  it('normalizes invalid and oversized inputs', () => {
    expect(normalizeRouteEntry({ id: 1, capacity: -5, weight: Infinity, bandwidthMbps: 1000001, maxUsers: NaN })).toMatchObject({ capacity: 0, weight: 0, bandwidthMbps: 1000000, maxUsers: 0, drain: false })
  })
  it('handles null, malformed entries and duplicate IDs', () => {
    expect(parseNodeRoute('null').enable).toBe(false)
    expect(parseNodeRoute('{').entries).toEqual([])
    expect(parseNodeRoute('{"entries":[null,{"id":1},{"id":1}]}').entries).toHaveLength(1)
  })
})
