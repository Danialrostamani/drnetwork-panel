// Node route shares are percentages: they should add up to 100. Older
// settings kept free weights; those still split in proportion, and can be
// scaled to percentages here.
export type ShareEntry = { id: number, weight: number }

export const sumShares = (list: ShareEntry[]): number =>
  list.reduce((s, e) => s + Math.max(0, Math.trunc(Number(e.weight) || 0)), 0)

// splitShares scales the weights to whole percentages adding up to exactly
// 100 (largest remainder); entries at 0 stay backup only. With no positive
// weight every entry gets an equal share.
export const splitShares = <T extends ShareEntry>(list: T[]): T[] => {
  if (list.length === 0) return []
  let base = list.map(e => Math.max(0, Math.trunc(Number(e.weight) || 0)))
  if (base.every(w => w === 0)) base = base.map(() => 1)
  const total = base.reduce((s, w) => s + w, 0)
  const exact = base.map(w => (w * 100) / total)
  const out = exact.map(x => Math.floor(x))
  let left = 100 - out.reduce((s, w) => s + w, 0)
  const order = exact
    .map((x, i) => ({ i, r: x - Math.floor(x) }))
    .filter(x => base[x.i] > 0)
    .sort((a, b) => b.r - a.r || a.i - b.i)
  for (let k = 0; left > 0 && order.length > 0; k = (k + 1) % order.length, left--) out[order[k].i]++
  return list.map((e, i) => ({ ...e, weight: out[i] }))
}
