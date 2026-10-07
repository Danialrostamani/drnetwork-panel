import HttpUtils from '@/plugins/httputil'
import Data from '@/store/modules/data'
import { push } from 'notivue'
import { i18n } from '@/locales'
import type { NodeAction, NodeActionResult } from '@/types/node'

// One request at a time: the HTTP client drops a request when an identical
// one starts, and every action posts to the same address.
let queue: Promise<unknown> = Promise.resolve()

// runNodeAction runs an action on some nodes, says how it went, and reloads
// the nodes. It answers null when the request itself failed, which the HTTP
// client has already reported.
export function runNodeAction(action: NodeAction, ids: number[]): Promise<NodeActionResult[] | null> {
  const next = queue.then(async () => {
    const msg = await HttpUtils.post<NodeActionResult[]>('api/nodeAction', { ids: JSON.stringify(ids), action })
    if (!msg.success) return null
    const results = msg.obj ?? []
    reportNodeAction(action, results)
    const store = Data()
    store.lastLoad = 0
    await store.loadData()
    return results
  })
  queue = next.catch(() => undefined)
  return next
}

export function reportNodeAction(action: NodeAction, results: NodeActionResult[]): void {
  const failed = results.filter(r => !r.ok)
  const title = i18n.global.t('node.action.' + action)
  if (failed.length === 0) {
    push.success({ title, message: i18n.global.t('node.action.done', { n: results.length }), duration: 5000 })
    return
  }
  push.error({
    title,
    message: [
      i18n.global.t('node.action.partial', { ok: results.length - failed.length, failed: failed.length }),
      ...failed.map(f => `${f.name}: ${f.error ?? ''}`),
    ].join(' · '),
    duration: 12000,
  })
}
