export type NodeRouteEntry = {
  id: number
  weight: number
  capacity?: number
  bandwidthMbps?: number
  maxUsers?: number
  drain?: boolean
}
export type NodeRoute = { enable: boolean, smart: boolean, entries: NodeRouteEntry[] }
const limit = (v: unknown, max: number, integer = true): number => {
  const n = Number(v)
  if (!Number.isFinite(n)) return 0
  return Math.max(0, Math.min(max, integer ? Math.trunc(n) : n))
}
export const normalizeRouteEntry = (v: Partial<NodeRouteEntry>): NodeRouteEntry => ({
  id: limit(v?.id, Number.MAX_SAFE_INTEGER),
  weight: limit(v?.weight, 10000),
  capacity: limit(v?.capacity, 10000, false),
  bandwidthMbps: limit(v?.bandwidthMbps, 1000000),
  maxUsers: limit(v?.maxUsers, 1000000),
  drain: v?.drain === true,
})
export const parseNodeRoute = (raw: string): NodeRoute => {
  try {
    const o = JSON.parse(raw || '{}')
    const seen = new Set<number>()
    return {
      enable: o?.enable === true,
      smart: o?.smart === true,
      entries: (Array.isArray(o?.entries) ? o.entries : []).map((e: Partial<NodeRouteEntry>) => normalizeRouteEntry(e)).filter((e: NodeRouteEntry) => {
        if (!e.id || seen.has(e.id)) return false
        seen.add(e.id)
        return true
      }),
    }
  } catch { return { enable: false, smart: false, entries: [] } }
}
export const serializeNodeRoute = (route: NodeRoute): string => JSON.stringify({
  enable: route.enable, smart: route.smart,
  entries: route.entries.map(normalizeRouteEntry),
})
